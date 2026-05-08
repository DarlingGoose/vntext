package enginerun

import (
	"testing"

	"github.com/DarlingGoose/gr/gamescope"
	"github.com/DarlingGoose/vntext/pkg/game"
)

func TestRunnerForGameSupportsGamescope(t *testing.T) {
	g := &game.Game{
		Runner:     game.RunnerGamescope,
		RunnerPath: "/usr/bin/gamescope",
		PrefixPath: "/tmp/prefix",
	}

	r, err := RunnerForGame(g)
	if err != nil {
		t.Fatalf("RunnerForGame returned error: %v", err)
	}

	gs, ok := r.(*gamescope.Runner)
	if !ok {
		t.Fatalf("RunnerForGame returned %T, want *gamescope.Runner", r)
	}
	if !gs.UseWine {
		t.Fatal("gamescope runner should run target through wine")
	}
	if gs.GamescopeBin != g.RunnerPath {
		t.Fatalf("GamescopeBin = %q, want %q", gs.GamescopeBin, g.RunnerPath)
	}
	if gs.DefaultWinePrefix != g.PrefixPath {
		t.Fatalf("DefaultWinePrefix = %q, want %q", gs.DefaultWinePrefix, g.PrefixPath)
	}
}

func TestWineBinIgnoresGamescopeRunnerPath(t *testing.T) {
	g := &game.Game{
		Runner:     game.RunnerGamescope,
		RunnerPath: "/usr/bin/gamescope",
	}

	if got := WineBin(g); got != "" {
		t.Fatalf("WineBin = %q, want empty for gamescope runner", got)
	}
}
