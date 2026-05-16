package artemis

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DarlingGoose/vntext/pkg/game"
)

func TestAddGameConfiguresWinePrefix(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "PRISON_ACADEMIA.exe")
	writeFile(t, exe, "not a real PE")
	writeFile(t, filepath.Join(root, "root.pfs"), "")

	eng := &Engine{
		Runner:     game.RunnerWine,
		PrefixRoot: filepath.Join(root, "prefixes"),
		managed:    make(map[string]*game.Game),
	}

	g, err := eng.AddGame(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}

	if got := g.Name; got != "PRISON_ACADEMIA" {
		t.Fatalf("Name = %q, want PRISON_ACADEMIA", got)
	}
	if got := g.Runner; got != game.RunnerWine {
		t.Fatalf("Runner = %q, want %q", got, game.RunnerWine)
	}
	if g.PrefixPath == "" {
		t.Fatal("PrefixPath is empty")
	}
	if got := g.RunnerConfig.WinePrefix; got != g.PrefixPath {
		t.Fatalf("RunnerConfig.WinePrefix = %q, want %q", got, g.PrefixPath)
	}
	if got := g.WorkingDir; got != root {
		t.Fatalf("WorkingDir = %q, want %q", got, root)
	}
	if got := g.TextHookLogFile; got != filepath.Join(root, dialogueJSONLName) {
		t.Fatalf("TextHookLogFile = %q, want %q", got, filepath.Join(root, dialogueJSONLName))
	}
}

func TestPrepareGameForRunBackfillsOlderConfig(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "PRISON_ACADEMIA.exe")
	writeFile(t, exe, "not a real PE")

	eng := &Engine{PrefixRoot: filepath.Join(root, "prefixes")}
	g := &game.Game{
		Name:       "PRISON_ACADEMIA",
		GamePath:   root,
		Executable: exe,
		EngineName: engineName,
	}

	if err := eng.prepareGameForRun(g); err != nil {
		t.Fatal(err)
	}

	if g.PrefixPath == "" {
		t.Fatal("PrefixPath is empty")
	}
	if got := g.Runner; got != game.RunnerWine {
		t.Fatalf("Runner = %q, want %q", got, game.RunnerWine)
	}
	if got := g.RunnerConfig.WinePrefix; got != g.PrefixPath {
		t.Fatalf("RunnerConfig.WinePrefix = %q, want %q", got, g.PrefixPath)
	}
	if got := g.WorkingDir; got != root {
		t.Fatalf("WorkingDir = %q, want %q", got, root)
	}
}

func TestRemoveExtractedRootDirOnlyRemovesTopLevelArtemisRoot(t *testing.T) {
	gameDir := t.TempDir()
	root001 := filepath.Join(gameDir, "root001")
	writeFile(t, filepath.Join(root001, "system", "adv", "init.lua"), "")
	archive := filepath.Join(gameDir, "root.pfs.001")
	writeFile(t, archive, "")

	if err := removeExtractedRootDir(gameDir, root001, archive); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root001); !os.IsNotExist(err) {
		t.Fatalf("root001 still exists or stat failed with non-not-exist error: %v", err)
	}

	other := filepath.Join(gameDir, "not-root")
	writeFile(t, filepath.Join(other, "file.txt"), "")
	if err := removeExtractedRootDir(gameDir, other, archive); err == nil {
		t.Fatal("expected non-Artemis root removal to be rejected")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("non-Artemis dir should remain: %v", err)
	}
}

func TestFindExtractedRootsHandlesNestedRootCreatedByPFSCreate(t *testing.T) {
	gameDir := t.TempDir()
	nested := filepath.Join(gameDir, "root002", "root002")
	writeFile(t, filepath.Join(nested, "system", "adv", "init.lua"), "")

	roots, err := findExtractedRoots(gameDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0] != nested {
		t.Fatalf("roots = %#v, want [%s]", roots, nested)
	}
}

func TestFindPFSArchivesSkipsYomunaBackups(t *testing.T) {
	gameDir := t.TempDir()
	writeFile(t, filepath.Join(gameDir, "root.pfs"), "")
	writeFile(t, filepath.Join(gameDir, "root.pfs.000"), "")
	writeFile(t, filepath.Join(gameDir, "root.pfs.001.yomuna.bak"), "")
	writeFile(t, filepath.Join(gameDir, "root.pfs.002.yomuna.new"), "")

	archives, err := findPFSArchives(gameDir)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		filepath.Join(gameDir, "root.pfs"),
		filepath.Join(gameDir, "root.pfs.000"),
	}
	if len(archives) != len(want) {
		t.Fatalf("archives = %#v, want %#v", archives, want)
	}
	for i := range want {
		if archives[i] != want[i] {
			t.Fatalf("archives = %#v, want %#v", archives, want)
		}
	}
}

func writeFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
