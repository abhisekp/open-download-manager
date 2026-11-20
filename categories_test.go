package main

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func categoryRevision(value int64) *int64 { return &value }

func TestCategoriesLifecyclePersistenceAndFailedSave(t *testing.T) {
	root := ownedTestDir(t)
	path := filepath.Join(root, "categories.json")
	store, err := openCategories(path)
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.change(categoryChange{Action: "create", Name: "Books", Revision: categoryRevision(0)})
	if err != nil {
		t.Fatal(err)
	}
	id := state.Categories[0].ID
	state, err = store.change(categoryChange{Action: "assign", ID: id, GID: "original-download", Revision: categoryRevision(state.Revision)})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := store.snapshot()
	snapshot.Assignments["original-download"] = "tamper"
	snapshot.Categories[0].Name = "tamper"
	if store.snapshot().Categories[0].Name != "Books" || store.snapshot().Assignments["original-download"] != id {
		t.Fatal("snapshot aliases mutable metadata")
	}
	restored, err := openCategories(path)
	if err != nil || !reflect.DeepEqual(restored.snapshot(), state) {
		t.Fatal("category assignment not durable", err)
	}
	for _, input := range []categoryChange{{Action: "rename", ID: id, Name: "Lost update", Revision: categoryRevision(0)}, {Action: "create", Name: "books", Revision: categoryRevision(state.Revision)}, {Action: "assign", ID: "missing", GID: "original-download", Revision: categoryRevision(state.Revision)}} {
		if _, err = store.change(input); err == nil {
			t.Fatal("invalid or stale category write accepted")
		}
		if !reflect.DeepEqual(store.snapshot(), state) {
			t.Fatal("refused category write changed state")
		}
	}
	oldBytes, _ := os.ReadFile(path)
	store.path = filepath.Join(root, "absent", "categories.json")
	if _, err = store.change(categoryChange{Action: "rename", ID: id, Name: "New name", Revision: categoryRevision(state.Revision)}); err == nil {
		t.Fatal("failed category write accepted")
	}
	if !reflect.DeepEqual(store.snapshot(), state) {
		t.Fatal("failed category write changed active state")
	}
	actual, _ := os.ReadFile(path)
	if string(actual) != string(oldBytes) {
		t.Fatal("failed write changed original metadata")
	}
	store.path = path
	state, err = store.change(categoryChange{Action: "rename", ID: id, Name: "Reading", Revision: categoryRevision(state.Revision)})
	if err != nil {
		t.Fatal(err)
	}
	state, err = store.change(categoryChange{Action: "delete", ID: id, Revision: categoryRevision(state.Revision)})
	if err != nil || len(state.Categories) != 0 || len(state.Assignments) != 0 {
		t.Fatal("category removal did not clear only labels", err)
	}
}

func TestCategoriesConcurrentWritersRefuseLostUpdate(t *testing.T) {
	store, err := openCategories(filepath.Join(ownedTestDir(t), "categories.json"))
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	results := make(chan error, 2)
	for _, name := range []string{"Books", "Software"} {
		wait.Add(1)
		go func(name string) {
			defer wait.Done()
			_, err := store.change(categoryChange{Action: "create", Name: name, Revision: categoryRevision(0)})
			results <- err
		}(name)
	}
	wait.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, errCategoryConflict) || errors.Is(err, errCategoryBusy) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 || len(store.snapshot().Categories) != 1 {
		t.Fatal("concurrent category edit lost an update")
	}
}

func TestCategoryReadersRemainAvailableDuringPendingSave(t *testing.T) {
	store, err := openCategories(filepath.Join(ownedTestDir(t), "categories.json"))
	if err != nil {
		t.Fatal(err)
	}
	store.writeMu.Lock()
	defer store.writeMu.Unlock()
	if _, err = store.change(categoryChange{Action: "create", Name: "Books", Revision: categoryRevision(0)}); !errors.Is(err, errCategoryBusy) {
		t.Fatal("pending category save did not reject another writer")
	}
	state := store.snapshot()
	if state.Revision != 0 || len(state.Categories) != 0 {
		t.Fatal("pending writer changed readable snapshot")
	}
}

func TestCategoriesCorruptionIsRetained(t *testing.T) {
	for _, body := range []string{`null`, `{"version":1,"revision":-1,"categories":[],"assignments":{}}`, `{"version":1,"revision":0,"categories":[],"assignments":{"unknown":"missing"}}`, `{"version":1,"revision":0,"categories":[],"assignments":{},"hidden":true}`, strings.Repeat("x", (256<<10)+1)} {
		path := filepath.Join(ownedTestDir(t), "categories.json")
		os.WriteFile(path, []byte(body), 0600)
		if _, err := openCategories(path); err == nil {
			t.Fatal("invalid category state accepted")
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != body {
			t.Fatal("corrupt original changed", err)
		}
	}
}

func TestCategoryOwnerAPIAndExistingTransfer(t *testing.T) {
	root := ownedTestDir(t)
	store, _ := openCategories(filepath.Join(root, "categories.json"))
	native, err := newNativeTorrents(root, filepath.Join(root, "files"), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	defer native.Close()
	gid := addTrackerFixtureTorrent(t, native, false)
	app := &App{categories: store, engine: &Engine{Native: native}, token: "category-owner", host: "127.0.0.1:9944"}
	post := func(body, token, origin string) int {
		req := httptest.NewRequest("POST", "http://"+app.host+"/api/categories", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Origin", origin)
		out := httptest.NewRecorder()
		app.handler().ServeHTTP(out, req)
		return out.Code
	}
	create := `{"action":"create","name":"Books","revision":0}`
	if post(create, "", "") != 401 || post(create, app.token, "https://other.invalid") != 403 || post(create, app.token, "") != 200 {
		t.Fatal("category ownership boundary")
	}
	state := store.snapshot()
	id := state.Categories[0].ID
	for _, name := range []string{"", " leading", "line\nname", strings.Repeat("x", 49)} {
		data, _ := json.Marshal(categoryChange{Action: "rename", ID: id, Name: name, Revision: categoryRevision(state.Revision)})
		if post(string(data), app.token, "") != 400 {
			t.Fatal("invalid category name accepted")
		}
	}
	body, _ := json.Marshal(categoryChange{Action: "assign", ID: id, GID: "missing-download", Revision: categoryRevision(state.Revision)})
	if post(string(body), app.token, "") != 400 {
		t.Fatal("assignment to missing transfer accepted")
	}
	body, _ = json.Marshal(categoryChange{Action: "assign", ID: id, GID: gid, Revision: categoryRevision(state.Revision)})
	if post(string(body), app.token, "") != 200 {
		t.Fatal("existing native transfer assignment refused")
	}
	if post(string(body), app.token, "") != 409 {
		t.Fatal("stale assignment overwrote current state")
	}
	if post(`{"action":"delete","revision":2,"hidden":true}`, app.token, "") != 400 {
		t.Fatal("unknown input field accepted")
	}
	if native.session.GetTorrent(gid) == nil {
		t.Fatal("category API modified transfer lifecycle")
	}
}
