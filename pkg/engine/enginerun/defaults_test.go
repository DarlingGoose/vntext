package enginerun

import (
	"testing"

	"github.com/DarlingGoose/gr/gamescope"
	"github.com/DarlingGoose/gr/wine"
	"github.com/DarlingGoose/vntext/pkg/game"
)

func TestDefaultConfigsForRunnerWine(t *testing.T) {
	defaults, err := DefaultConfigsForRunner(game.RunnerWine, "/tmp/prefix", []wine.Option{
		wine.WithWineBin("wine-custom"),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if defaults.Runner != game.RunnerWine {
		t.Fatalf("Runner = %q, want %q", defaults.Runner, game.RunnerWine)
	}
	if defaults.WineConfig == nil {
		t.Fatal("WineConfig is nil")
	}
	if got := defaults.WineConfig.DefaultPrefix; got != "/tmp/prefix" {
		t.Fatalf("DefaultPrefix = %q, want /tmp/prefix", got)
	}
	if got := defaults.WineConfig.WineBin; got != "wine-custom" {
		t.Fatalf("WineBin = %q, want wine-custom", got)
	}
	if defaults.GamescopeConfig != nil {
		t.Fatal("GamescopeConfig is not nil")
	}
}

func TestDefaultConfigsForRunnerGamescope(t *testing.T) {
	defaults, err := DefaultConfigsForRunner(game.RunnerGamescope, "/tmp/prefix", nil, []gamescope.Option{
		gamescope.WithResolution(800, 600),
		gamescope.WithFilter("nearest"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if defaults.Runner != game.RunnerGamescope {
		t.Fatalf("Runner = %q, want %q", defaults.Runner, game.RunnerGamescope)
	}
	if defaults.GamescopeConfig == nil {
		t.Fatal("GamescopeConfig is nil")
	}
	if got := defaults.GamescopeConfig.DefaultWinePrefix; got != "/tmp/prefix" {
		t.Fatalf("DefaultWinePrefix = %q, want /tmp/prefix", got)
	}
	if !defaults.GamescopeConfig.UseWine {
		t.Fatal("UseWine = false, want true")
	}
	if defaults.GamescopeConfig.Width != 800 || defaults.GamescopeConfig.Height != 600 {
		t.Fatalf("resolution = %dx%d, want 800x600", defaults.GamescopeConfig.Width, defaults.GamescopeConfig.Height)
	}
	if got := defaults.GamescopeConfig.Filter; got != "nearest" {
		t.Fatalf("Filter = %q, want nearest", got)
	}
	if defaults.WineConfig != nil {
		t.Fatal("WineConfig is not nil")
	}
}

func TestDefaultConfigsForRunnerRejectsUnsupportedRunner(t *testing.T) {
	if _, err := DefaultConfigsForRunner(game.RunnerSteam, "", nil, nil); err == nil {
		t.Fatal("expected unsupported runner error")
	}
}
