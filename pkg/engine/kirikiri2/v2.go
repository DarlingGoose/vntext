package kirikiri2

import (
	"context"

	"github.com/DarlingGoose/gr"
	"github.com/DarlingGoose/tr/pkg/textractor"
	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/engine/enginerun"
	"github.com/DarlingGoose/vntext/pkg/game"
)

var _ engine.EngineV2 = (*Engine)(nil)

func (e *Engine) GetTextractor(game *game.Game) *textractor.Client {
	//TODO implement me
	panic("implement me")
}

func (e *Engine) ManagedGames() []*game.Game {
	//TODO implement me
	panic("implement me")
}

func (e *Engine) Shutdown() error {
	//TODO implement me
	panic("implement me")
}

func (e *Engine) AddGame(ctx context.Context, path string) (*game.Game, error) {
	prepared, err := e.EnsureReady(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := enginerun.ConfigureRunner(prepared.Game); err != nil {
		return nil, err
	}
	return prepared.Game, nil
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
