package enginerun

import (
	"fmt"

	"github.com/DarlingGoose/gr/gamescope"
	"github.com/DarlingGoose/gr/monitors"
	"github.com/DarlingGoose/gr/wine"
	"github.com/DarlingGoose/vntext/pkg/game"
)

// RunnerDefaultConfigs contains the native runner configs that autorunner
// would use when constructing a runner for a Wine prefix.
type RunnerDefaultConfigs struct {
	Runner          game.RunnerType
	WineConfig      *wine.Options
	GamescopeConfig *gamescope.Options
}

// DefaultConfigsForRunner returns the default native GR runner configs for the
// requested runner, including any caller-supplied options. The defaults mirror
// autorunner.NewRunner/NewRunnerWithOptions without checking installed
// dependencies or launching a runner.
func DefaultConfigsForRunner(runner game.RunnerType, winePrefix string, wineOpts []wine.Option, gamescopeOpts []gamescope.Option) (RunnerDefaultConfigs, error) {
	switch runner {
	case "", game.RunnerWine:
		cfg := defaultWineConfig(winePrefix, wineOpts...)
		return RunnerDefaultConfigs{
			Runner:     game.RunnerWine,
			WineConfig: &cfg,
		}, nil
	case game.RunnerGamescope:
		cfg := defaultGamescopeConfig(winePrefix, gamescopeOpts...)
		return RunnerDefaultConfigs{
			Runner:          game.RunnerGamescope,
			GamescopeConfig: &cfg,
		}, nil
	default:
		return RunnerDefaultConfigs{}, fmt.Errorf("%s runner is not supported by EngineV2 GR launcher", runner)
	}
}

func defaultWineConfig(winePrefix string, opts ...wine.Option) wine.Options {
	defaultOptions := append([]wine.Option{wine.WithDefaultPrefix(winePrefix)}, opts...)
	return wine.ApplyOptions(defaultOptions...)
}

func defaultGamescopeConfig(winePrefix string, opts ...gamescope.Option) gamescope.Options {
	outW, outH := 1280, 720
	inW, inH := 1280, 720
	if m, err := monitors.GetMonitors(); err == nil && len(m) > 0 {
		outW = m[0].CurrentMode.Width
		outH = m[0].CurrentMode.Height
		inW = 1920
		inH = 1080
	}

	defaultOptions := []gamescope.Option{
		gamescope.WithWine(true),
		gamescope.WithDefaultWinePrefix(winePrefix),
		gamescope.WithResolution(inW, inH),
		gamescope.WithOutputResolution(outW, outH),
		gamescope.WithFullscreen(true),
		gamescope.WithScaler("fit"),
		gamescope.WithFilter("linear"),
		gamescope.WithExposeWayland(monitors.IsWayland()),
	}
	defaultOptions = append(defaultOptions, opts...)
	return gamescope.ApplyOptions(defaultOptions...)
}
