// Package addons implements signed, permission-reviewed WASI URL resolvers.
package addons

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"
)

const MaxPackage = 8 << 20

type Manifest struct {
	API          int      `json:"api"`
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description"`
	Capability   string   `json:"capability"`
	InputHosts   []string `json:"input_hosts"`
	OutputHosts  []string `json:"output_hosts"`
	NetworkHosts []string `json:"network_hosts,omitempty"`
	ModuleHash   string   `json:"module_sha256"`
	LicenseHash  string   `json:"license_sha256"`
}

type Info struct {
	Manifest  Manifest `json:"manifest"`
	Digest    string   `json:"digest"`
	Publisher string   `json:"publisher"`
	Enabled   bool     `json:"enabled"`
	// A signature proves key continuity, not that a publisher is trustworthy.
}

type Package struct {
	Info
	Module  []byte
	Key     []byte
	License []byte
}

var identifier = regexp.MustCompile(`^[a-z0-9]+([.-][a-z0-9]+)*$`)
var semver = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var hostname = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

func (m Manifest) Validate() error {
	if (m.API != 1 && m.API != 2) || len(m.ID) > 100 || !identifier.MatchString(m.ID) || len(m.Name) < 1 || len(m.Name) > 80 || len(m.Description) > 1000 || len(m.Version) > 32 || !semver.MatchString(m.Version) || m.Capability != "url-resolver" {
		return errors.New("unsupported add-on manifest, version or capability")
	}
	if len(m.ModuleHash) != 64 {
		return errors.New("invalid module digest")
	}
	if _, err := hex.DecodeString(m.ModuleHash); err != nil {
		return errors.New("invalid module digest")
	}
	if !validDigest(m.LicenseHash) {
		return errors.New("invalid license digest")
	}
	if m.API == 1 && len(m.NetworkHosts) > 0 {
		return errors.New("network permissions require API 2")
	}
	lists := [][]string{m.InputHosts, m.OutputHosts}
	if len(m.NetworkHosts) > 0 {
		lists = append(lists, m.NetworkHosts)
	}
	for _, hosts := range lists {
		if len(hosts) == 0 || len(hosts) > 16 {
			return errors.New("declare between 1 and 16 exact hosts")
		}
		seen := map[string]bool{}
		for _, host := range hosts {
			if len(host) > 253 || !hostname.MatchString(host) || strings.Contains(host, "..") || seen[host] {
				return errors.New("invalid or duplicate host permission; wildcards are forbidden")
			}
			seen[host] = true
		}
	}
	return nil
}

func permitted(raw string, hosts []string) bool {
	if len(raw) > 8192 || strings.ContainsAny(raw, "\r\n\x00") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	for _, host := range hosts {
		if strings.EqualFold(u.Hostname(), host) {
			return true
		}
	}
	return false
}

func Inspect(raw []byte) (*Package, error) {
	if len(raw) == 0 || len(raw) > MaxPackage {
		return nil, errors.New("add-on package exceeds 8 MiB")
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, errors.New("invalid add-on ZIP")
	}
	files := map[string][]byte{}
	total := uint64(0)
	for _, file := range archive.File {
		limit := uint64(MaxPackage)
		switch file.Name {
		case "manifest.json":
			limit = 16384
		case "module.wasm":
		case "LICENSE":
			limit = 1 << 20
		case "publisher.ed25519":
			limit = ed25519.PublicKeySize
		case "signature.ed25519":
			limit = ed25519.SignatureSize
		default:
			return nil, errors.New("unexpected add-on package entry")
		}
		total += file.UncompressedSize64
		if file.Mode()&os.ModeSymlink != 0 || file.FileInfo().IsDir() || file.UncompressedSize64 > limit || total > MaxPackage || files[file.Name] != nil {
			return nil, errors.New("unsafe, duplicate or oversized add-on entry")
		}
		r, e := file.Open()
		if e != nil {
			return nil, e
		}
		body, e := io.ReadAll(io.LimitReader(r, int64(limit)+1))
		_ = r.Close()
		if e != nil || uint64(len(body)) > limit {
			return nil, errors.New("corrupt add-on entry")
		}
		files[file.Name] = body
	}
	if len(files) != 5 {
		return nil, errors.New("add-on package must contain five required files")
	}
	var m Manifest
	d := json.NewDecoder(bytes.NewReader(files["manifest.json"]))
	d.DisallowUnknownFields()
	if d.Decode(&m) != nil || d.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid manifest JSON")
	}
	if err = m.Validate(); err != nil {
		return nil, err
	}
	key, signature, module := files["publisher.ed25519"], files["signature.ed25519"], files["module.wasm"]
	if len(key) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize || !ed25519.Verify(key, files["manifest.json"], signature) {
		return nil, errors.New("invalid publisher signature")
	}
	if len(module) < 8 || !bytes.Equal(module[:8], []byte{0, 97, 115, 109, 1, 0, 0, 0}) {
		return nil, errors.New("invalid WebAssembly module")
	}
	hash := sha256.Sum256(module)
	if hex.EncodeToString(hash[:]) != m.ModuleHash {
		return nil, errors.New("module digest mismatch")
	}
	license := files["LICENSE"]
	licenseHash := sha256.Sum256(license)
	if len(license) == 0 || hex.EncodeToString(licenseHash[:]) != m.LicenseHash {
		return nil, errors.New("license digest mismatch")
	}
	digest, publisher := sha256.Sum256(raw), sha256.Sum256(key)
	return &Package{Info: Info{Manifest: m, Digest: hex.EncodeToString(digest[:]), Publisher: hex.EncodeToString(publisher[:])}, Module: module, Key: key, License: license}, nil
}
