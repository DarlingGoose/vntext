package rpgmaker

import (
	"context"
	"errors"
	"strings"

	"github.com/DarlingGoose/gr"
	"github.com/DarlingGoose/tr/pkg/textractor"
	"github.com/DarlingGoose/vntext/pkg/app"
	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/engine/enginerun"
	"github.com/DarlingGoose/vntext/pkg/game"
)

var _ engine.EngineV2 = (*Engine)(nil)

func (e *Engine) GetTextractor(game *game.Game) *textractor.Client {
	return nil
}

func (e *Engine) ManagedGames() []*game.Game {
	return nil
}

func (e *Engine) Shutdown() error {
	return nil
}

func (e *Engine) InstallHook(ctx context.Context, g *game.Game) error {
	_ = ctx
	return e.InstallTextHook(g)
}

func (e *Engine) RunGame(ctx context.Context, g *game.Game) (*gr.Process, error) {
	if err := e.prepareGameForRun(g); err != nil {
		return nil, err
	}

	return enginerun.RunGame(ctx, g)
}

func (e *Engine) StopGame(ctx context.Context, proc *gr.Process) (*gr.Process, error) {
	return enginerun.StopGame(ctx, proc)
}

func (e *Engine) FollowGameText(ctx context.Context, g *game.Game, opts ...engine.FollowGameOptions) (chan engine.Line, error) {
	return enginerun.FollowGameText(ctx, g, opts...)
}

func detectArchitecture(path string) game.Architecture {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	arch, err := game.DetectExecutableArchitecture(path)
	if err != nil {
		return ""
	}
	return arch
}
func (e *Engine) prepareGameForRun(g *game.Game) error {
	if g == nil {
		return errors.New("game is nil")
	}

	if strings.TrimSpace(g.PrefixPath) == "" {
		prefix, err := enginerun.PrefixPath(app.Name(), e.PrefixRoot, g.Name)
		if err != nil {
			return err
		}
		g.PrefixPath = prefix
	}

	if strings.TrimSpace(g.WorkingDir) == "" {
		g.WorkingDir = enginerun.WorkingDir(g)
	}

	if g.Architecture == "" {
		g.Architecture = detectArchitecture(g.Executable)
	}

	// Keep runner configs in sync if they already exist.
	if g.WineConfig != nil && strings.TrimSpace(g.WineConfig.DefaultPrefix) == "" {
		g.WineConfig.DefaultPrefix = g.PrefixPath
	}

	if g.GamescopeConfig != nil && strings.TrimSpace(g.GamescopeConfig.DefaultWinePrefix) == "" {
		g.GamescopeConfig.DefaultWinePrefix = g.PrefixPath
	}

	if strings.TrimSpace(g.RunnerConfig.WinePrefix) == "" {
		g.RunnerConfig.WinePrefix = g.PrefixPath
	}

	return nil
}
