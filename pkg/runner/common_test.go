package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DarlingGoose/vntext/pkg/game"
)

func TestBaseEnvDoesNotSetWineArchForEveryRun(t *testing.T) {
	g := &game.Game{
		Architecture: game.ArchitectureX86,
	}

	got := strings.Join(baseEnv(g), "\n")
	if strings.Contains(got, "WINEARCH=") {
		t.Fatalf("baseEnv() included WINEARCH:\n%s", got)
	}
}

func TestWineArchitectureEnvForNewPrefix(t *testing.T) {
	g := &game.Game{
		Architecture: game.ArchitectureX86,
		PrefixPath:   t.TempDir(),
		Runner:       game.RunnerWine,
	}

	got := strings.Join(wineArchitectureEnvForNewPrefix(g), "\n")
	if got != "WINEARCH=win32" {
		t.Fatalf("wineArchitectureEnvForNewPrefix() = %q", got)
	}
}

func TestWineArchitectureEnvSkipsInitializedPrefix(t *testing.T) {
	prefix := t.TempDir()
	if err := os.WriteFile(filepath.Join(prefix, "system.reg"), []byte("WINE REGISTRY\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	g := &game.Game{
		Architecture: game.ArchitectureX86,
		PrefixPath:   prefix,
		Runner:       game.RunnerWine,
	}

	if got := wineArchitectureEnvForNewPrefix(g); len(got) != 0 {
		t.Fatalf("wineArchitectureEnvForNewPrefix() = %v", got)
	}
}

func TestWineCommandUsesVirtualDesktopByDefault(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "Game.exe")
	if err := os.WriteFile(exe, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd, err := (&WineRunner{}).command(&game.Game{
		Name:       "game",
		Executable: exe,
		Runner:     game.RunnerWine,
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"wine", "explorer", "/desktop=vntext,1280x720", exe}
	if strings.Join(cmd.Args, "\n") != strings.Join(want, "\n") {
		t.Fatalf("cmd.Args = %#v, want %#v", cmd.Args, want)
	}
}

func TestWineCommandCanDisableVirtualDesktop(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "Game.exe")
	if err := os.WriteFile(exe, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd, err := (&WineRunner{}).command(&game.Game{
		Name:           "game",
		Executable:     exe,
		Runner:         game.RunnerWine,
		VirtualDesktop: "off",
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"wine", exe}
	if strings.Join(cmd.Args, "\n") != strings.Join(want, "\n") {
		t.Fatalf("cmd.Args = %#v, want %#v", cmd.Args, want)
	}
}
