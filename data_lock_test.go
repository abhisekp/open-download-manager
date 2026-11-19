package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataLockExcludesAnotherInstanceAndCanBeReacquired(t *testing.T) {
	dir := t.TempDir()
	first, err := acquireDataLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := acquireDataLock(dir); err == nil {
		second.Close()
		t.Fatal("second owner acquired live lock")
	}
	first.Close()
	second, err := acquireDataLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	second.Close()
	if _, err = os.Stat(filepath.Join(dir, ".instance.lock")); err != nil {
		t.Fatal("permanent lock file missing", err)
	}
	if _, err = os.Stat(filepath.Join(dir, "running.lock")); !os.IsNotExist(err) {
		t.Fatal("compatibility marker not cleaned")
	}
}

func TestLegacyLockIsNeverSilentlyRemoved(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "running.lock")
	if err := os.Mkdir(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	if lock, err := acquireDataLock(dir); err == nil {
		lock.Close()
		t.Fatal("unknown legacy owner ignored")
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatal("legacy lock removed without owner proof")
	}
}
