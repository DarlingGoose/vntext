package textreactor

import (
	"testing"

	"github.com/DarlingGoose/gr/gamescope"
	"github.com/DarlingGoose/gr/wine"
	"github.com/DarlingGoose/vntext/pkg/game"
)

func TestTextReactorRunGameDefaultsToWine(t *testing.T) {
	g := &game.Game{
		Runner: game.RunnerGamescope,
		WineConfig: &wine.Options{
			WineBin: "/usr/bin/wine-custom",
		},
	}

	runGame := textReactorRunGame(g)
	if runGame.Runner != game.RunnerWine {
		t.Fatalf("Runner = %q, want %q", runGame.Runner, game.RunnerWine)
	}
	if runGame.RunnerPath != "/usr/bin/wine-custom" {
		t.Fatalf("RunnerPath = %q, want wine bin", runGame.RunnerPath)
	}
	if g.Runner != game.RunnerGamescope {
		t.Fatalf("original runner was mutated: %q", g.Runner)
	}
}

func TestTextReactorRunGameKeepsExplicitGamescope(t *testing.T) {
	g := &game.Game{
		Runner: game.RunnerGamescope,
		GamescopeConfig: &gamescope.Options{
			GamescopeBin: "/usr/bin/gamescope",
		},
	}

	runGame := textReactorRunGame(g)
	if runGame.Runner != game.RunnerGamescope {
		t.Fatalf("Runner = %q, want explicit gamescope", runGame.Runner)
	}
}
