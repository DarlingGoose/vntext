package gameConfig

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DarlingGoose/vntext/pkg/app"
)

func TestInstallGameAcceptsExeInputAndWritesDefaultConfig(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "js", "plugins"), 0o755); err != nil {
		t.Fatalf("create project dirs: %v", err)
	}
	writeTestFile(t, filepath.Join(root, "js", "rpg_core.js"), "")
	writeTestFile(t, filepath.Join(root, "js", "plugins.js"), "var $plugins = [];\n")

	exe := filepath.Join(root, "Direct_Game.exe")
	writeTestFile(t, exe, "")

	g, eng, err := InstallGame(context.Background(), exe, false, "")
	if err != nil {
		t.Fatalf("InstallGame(%q) returned error: %v", exe, err)
	}

	if eng.Name() != "rpgmaker" {
		t.Fatalf("expected rpgmaker engine, got %q", eng.Name())
	}
	if g.Executable != exe {
		t.Fatalf("expected direct exe %q, got %q", exe, g.Executable)
	}

	configPath := filepath.Join(configHome, app.Name(), "games", "direct-game.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected default config at %s: %v", configPath, err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
