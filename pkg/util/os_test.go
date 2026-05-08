package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindExeFindsSingleTopLevelExe(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "Game.exe")
	if err := os.WriteFile(exe, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := FindExe(root)
	if err != nil {
		t.Fatalf("FindExe() error = %v", err)
	}
	if got != exe {
		t.Fatalf("FindExe() = %q, want %q", got, exe)
	}
}

func TestFindExeIgnoresNestedDirs(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "Nested.exe"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := FindExe(root); err == nil {
		t.Fatal("FindExe() returned nil error for nested-only exe")
	}
}
