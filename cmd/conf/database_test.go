// Copyright 2026, Offchain Labs, Inc.
// For license information, see https://github.com/OffchainLabs/nitro/blob/master/LICENSE.md

package conf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistentConfigValidateAllowsTreeDB(t *testing.T) {
	cfg := PersistentConfigDefault
	cfg.DBEngine = "treedb"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate treedb: %v", err)
	}
}

func TestPersistentConfigValidateRejectsUnknownEngine(t *testing.T) {
	cfg := PersistentConfigDefault
	cfg.DBEngine = "mssql"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected invalid db engine error")
	}
	want := `allowed "leveldb", "pebble", "treedb" or ""`
	if got := err.Error(); !strings.Contains(got, want) {
		t.Fatalf("Validate error = %q, want substring %q", got, want)
	}
}

func TestDatabaseInDirectoryDetectsTreeDBLayouts(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "maindb"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "maindb", "index.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if !DatabaseInDirectory(root) {
		t.Fatal("DatabaseInDirectory did not detect TreeDB root layout")
	}

	flat := t.TempDir()
	if err := os.WriteFile(filepath.Join(flat, "index.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if !DatabaseInDirectory(flat) {
		t.Fatal("DatabaseInDirectory did not detect TreeDB flat layout")
	}
}
