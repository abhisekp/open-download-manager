package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type CategoryState struct {
	Version     int               `json:"version"`
	Revision    int64             `json:"revision"`
	Categories  []Category        `json:"categories"`
	Assignments map[string]string `json:"assignments"`
}
type Categories struct {
	mu      sync.Mutex
	writeMu sync.Mutex
	path    string
	state   CategoryState
}
type categoryChange struct {
	Action   string `json:"action"`
	Revision *int64 `json:"revision"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	GID      string `json:"gid"`
}

var errCategoryConflict = errors.New("categories changed in another window; refresh and review your change again")
var errCategoryBusy = errors.New("another category change is still being saved; try again shortly")

func categoryName(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && utf8.ValidString(value) && utf8.RuneCountInString(value) <= 48 && strings.IndexFunc(value, func(r rune) bool { return !unicode.IsPrint(r) || unicode.IsControl(r) }) < 0
}
func categoryGID(value string) bool {
	return value != "" && len(value) <= 64 && strings.IndexFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	}) < 0
}
func categoryID(value string) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == 8 && strings.ToLower(value) == value
}

func openCategories(path string) (*Categories, error) {
	c := &Categories{path: path, state: CategoryState{Version: 1, Categories: []Category{}, Assignments: map[string]string{}}}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<10 {
		return nil, errors.New("category metadata requires recovery; original retained")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("category metadata unavailable")
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, errors.New("category metadata changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(f, (256<<10)+1))
	if err != nil || len(data) > 256<<10 || !utf8.Valid(data) {
		return nil, errors.New("invalid category metadata; original retained")
	}
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	var stored CategoryState
	if d.Decode(&stored) != nil || d.Decode(new(any)) != io.EOF || !stored.valid() {
		return nil, errors.New("invalid category metadata; original retained")
	}
	c.state = stored
	return c, nil
}
func (s CategoryState) valid() bool {
	if s.Version != 1 || s.Revision < 0 || s.Revision >= 1<<53 || s.Categories == nil || s.Assignments == nil || len(s.Categories) > 32 || len(s.Assignments) > 2048 {
		return false
	}
	ids := map[string]bool{}
	names := []string{}
	for _, cat := range s.Categories {
		if !categoryID(cat.ID) || ids[cat.ID] || !categoryName(cat.Name) {
			return false
		}
		for _, name := range names {
			if strings.EqualFold(name, cat.Name) {
				return false
			}
		}
		ids[cat.ID] = true
		names = append(names, cat.Name)
	}
	for gid, id := range s.Assignments {
		if !categoryGID(gid) || !ids[id] {
			return false
		}
	}
	return true
}
func (s CategoryState) clone() CategoryState {
	s.Categories = slices.Clone(s.Categories)
	s.Assignments = maps.Clone(s.Assignments)
	return s
}
func (c *Categories) snapshot() CategoryState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state.clone()
}
func (c *Categories) change(in categoryChange) (CategoryState, error) {
	if !c.writeMu.TryLock() {
		return CategoryState{}, errCategoryBusy
	}
	defer c.writeMu.Unlock()
	c.mu.Lock()
	next := c.state.clone()
	path := c.path
	c.mu.Unlock()
	if in.Revision == nil || *in.Revision != next.Revision {
		return CategoryState{}, errCategoryConflict
	}
	index := slices.IndexFunc(next.Categories, func(v Category) bool { return v.ID == in.ID })
	switch in.Action {
	case "create":
		if in.ID != "" || in.GID != "" || !categoryName(in.Name) || len(next.Categories) >= 32 {
			return CategoryState{}, errors.New("choose a visible category name of 1–48 characters; at most 32 categories")
		}
		var bytes [8]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			return CategoryState{}, errors.New("cannot create category identifier")
		}
		next.Categories = append(next.Categories, Category{ID: hex.EncodeToString(bytes[:]), Name: in.Name})
	case "rename":
		if index < 0 || in.GID != "" || !categoryName(in.Name) {
			return CategoryState{}, errors.New("choose an existing category and a visible name of 1–48 characters")
		}
		next.Categories[index].Name = in.Name
	case "delete":
		if index < 0 || in.GID != "" || in.Name != "" {
			return CategoryState{}, errors.New("choose an existing category")
		}
		next.Categories = slices.Delete(next.Categories, index, index+1)
		for gid, id := range next.Assignments {
			if id == in.ID {
				delete(next.Assignments, gid)
			}
		}
	case "assign":
		if !categoryGID(in.GID) || in.Name != "" || (in.ID != "" && index < 0) {
			return CategoryState{}, errors.New("choose an existing download and category, or clear the category")
		}
		if in.ID == "" {
			delete(next.Assignments, in.GID)
		} else {
			next.Assignments[in.GID] = in.ID
		}
	default:
		return CategoryState{}, errors.New("unsupported category action")
	}
	next.Revision++
	for i, category := range next.Categories {
		for _, other := range next.Categories[:i] {
			if strings.EqualFold(category.Name, other.Name) {
				return CategoryState{}, errors.New("a category with this name already exists")
			}
		}
	}
	if len(next.Assignments) > 2048 {
		return CategoryState{}, errors.New("category assignment limit reached (2048); remove unused categories or clear assignments before trying again")
	}
	if !next.valid() {
		return CategoryState{}, errors.New("category settings could not be saved")
	}
	body, err := json.Marshal(next)
	if err != nil || len(body) > 256<<10 {
		return CategoryState{}, errors.New("category metadata limit reached")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".categories-*")
	if err != nil {
		return CategoryState{}, errors.New("cannot save categories; previous categories retained")
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(body)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil || os.Rename(f.Name(), path) != nil {
		return CategoryState{}, errors.New("cannot save categories; previous categories retained")
	}
	c.mu.Lock()
	c.state = next
	c.mu.Unlock()
	return next.clone(), nil
}

func (a *App) categoriesAPI(w http.ResponseWriter, r *http.Request) {
	if a.categories == nil {
		http.Error(w, "Saved categories are unavailable. Your original settings were kept; downloads remain usable", 503)
		return
	}
	if r.Method == http.MethodGet {
		respond(w, a.categories.snapshot())
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", 405)
		return
	}
	var input categoryChange
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	d.DisallowUnknownFields()
	if d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid category change", 400)
		return
	}
	if input.Action == "assign" {
		if a.engine == nil {
			http.Error(w, "transfer status unavailable; refresh before assigning a category", 503)
			return
		}
		if !categoryGID(input.GID) {
			http.Error(w, "invalid download identifier", 400)
			return
		}
		_, err := a.engine.Call(r.Context(), "tellStatus", input.GID)
		if err != nil && len(a.history.get(input.GID)) == 0 {
			http.Error(w, "download not found; refresh before assigning a category", 400)
			return
		}
	}
	state, err := a.categories.change(input)
	if err != nil {
		code := 400
		if errors.Is(err, errCategoryConflict) {
			code = 409
		}
		if errors.Is(err, errCategoryBusy) {
			code = 503
		}
		http.Error(w, err.Error(), code)
		return
	}
	respond(w, state)
}
