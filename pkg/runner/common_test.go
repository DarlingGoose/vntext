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
