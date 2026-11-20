package credentials

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
)

func TestOSKeyringOwnedLifecycle(t *testing.T) {
	if os.Getenv("ODM_TEST_OS_KEYRING") != "1" {
		t.Skip("explicit isolated OS keyring test not enabled")
	}
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	id := hex.EncodeToString(key)
	service := "org.opendownloadmanager.qa." + id
	backend := OSBackend{}
	created := false
	t.Cleanup(func() {
		if created {
			if err := backend.Delete(service, id); err != nil && !backend.Missing(err) {
				t.Error("could not remove exact owned keyring fixture", service, id)
			}
		}
	})
	fixture := "disposable public QA fixture; never an owner credential"
	if err := backend.Set(service, id, fixture); err != nil {
		t.Fatal("OS keyring could not store owned fixture")
	}
	created = true
	value, err := backend.Get(service, id)
	if err != nil || value != fixture {
		t.Fatal("OS keyring round trip failed")
	}
	if err = backend.Delete(service, id); err != nil {
		t.Fatal("OS keyring removal failed")
	}
	created = false
	if _, err = backend.Get(service, id); !backend.Missing(err) {
		t.Fatal("owned fixture remained after removal")
	}
}
