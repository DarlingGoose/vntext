package rpgmaker

import (
	"context"
	"strings"

	"github.com/DarlingGoose/gr"
	"github.com/DarlingGoose/tr/pkg/textractor"
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
