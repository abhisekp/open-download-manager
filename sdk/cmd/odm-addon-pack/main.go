// odm-addon-pack builds signed local packages. Private keys must stay outside source.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/abhisekp/open-download-manager/internal/addons"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	generate := flag.String("generate-key", "", "create a new private signing key at this path; never commit it")
	manifest := flag.String("manifest", "", "manifest JSON path")
	module := flag.String("module", "", "WASI Preview 1 module path")
	keyPath := flag.String("key", "", "private signing key path")
	licensePath := flag.String("license", "", "license text including compiled dependency notices")
	output := flag.String("out", "", "new .odmx package path")
	flag.Parse()
	if *generate != "" {
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(*generate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = f.Write(key)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Println("Created private key. Keep it outside source and distribution archives.")
		return nil
	}
	if *manifest == "" || *module == "" || *keyPath == "" || *output == "" || *licensePath == "" {
		return fmt.Errorf("manifest, module, key, license and out are required")
	}
	key, err := os.ReadFile(*keyPath)
	if err != nil {
		return err
	}
	if len(key) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid signing key")
	}
	body, err := os.ReadFile(*module)
	if err != nil {
		return err
	}
	if len(body) > addons.MaxPackage {
		return fmt.Errorf("module too large")
	}
	raw, err := os.ReadFile(*manifest)
	if err != nil {
		return err
	}
	var m addons.Manifest
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&m); err != nil {
		return err
	}
	hash := sha256.Sum256(body)
	m.ModuleHash = hex.EncodeToString(hash[:])
	license, err := os.ReadFile(*licensePath)
	if err != nil {
		return err
	}
	if len(license) == 0 || len(license) > 1<<20 {
		return fmt.Errorf("license text must be between 1 byte and 1 MiB")
	}
	licenseHash := sha256.Sum256(license)
	m.LicenseHash = hex.EncodeToString(licenseHash[:])
	if err = m.Validate(); err != nil {
		return err
	}
	raw, err = json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	private := ed25519.PrivateKey(key)
	signature := ed25519.Sign(private, raw)
	var result bytes.Buffer
	archive := zip.NewWriter(&result)
	for _, file := range []struct {
		name string
		data []byte
	}{{"manifest.json", raw}, {"module.wasm", body}, {"publisher.ed25519", private.Public().(ed25519.PublicKey)}, {"signature.ed25519", signature}, {"LICENSE", license}} {
		w, e := archive.Create(file.name)
		if e != nil {
			return e
		}
		if _, e = w.Write(file.data); e != nil {
			return e
		}
	}
	if err = archive.Close(); err != nil {
		return err
	}
	pkg, err := addons.Inspect(result.Bytes())
	if err != nil {
		return err
	}
	f, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(result.Bytes())
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Printf("Created package %s\nPublisher fingerprint: %s\nPackage SHA-256: %s\n", pkg.Manifest.ID, pkg.Publisher, pkg.Digest)
	return nil
}
