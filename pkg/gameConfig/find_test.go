package gameConfig

import (
	"os"
	"path/filepath"
	"testing"
)

//func TestLoadInstalledGamesReturnsParseErrors(t *testing.T) {
//	dir := t.TempDir()
//	writeFindTestFile(t, filepath.Join(dir, "broken.json"), "{")
//
//	if _, err := LoadInstalledGames(dir); err == nil {
//		t.Fatalf("expected parse error")
//	}
//}

func TestLoadInstalledGamesDeduplicatesDirs(t *testing.T) {
	dir := t.TempDir()
	writeFindTestFile(t, filepath.Join(dir, "game.json"), `{"name":"Game"}`)

	games, err := LoadInstalledGames(dir, dir)
	if err != nil {
		t.Fatalf("LoadInstalledGames returned error: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("expected one game, got %d", len(games))
	}
}

func writeFindTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
