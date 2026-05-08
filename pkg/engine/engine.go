package engine

import (
	"context"
	"errors"

	"github.com/DarlingGoose/tr/pkg/textractor"
	"github.com/DarlingGoose/vntext/pkg/game"

	"github.com/DarlingGoose/gr"
)

var ErrNoEngineFound = errors.New("no supported engine found")

type Engine interface {
	Name() string
	InstallGame(dir string) (*game.Game, error)
	IsDirEngine(dir string) bool
	InstallTextHook(game *game.Game) error
	IsEngine(dir string) bool
	SetCustomPlugin(data string) error
	GetDefaultPlugin() string
	GetFile(g *game.Game, file string) (*EngineFileInfo, error)
	//todo
}

type EngineV2 interface {
	Name() string

	IsEngine(dir string) bool
	//AddGame Will only work on games that have already been installed/ not the launcher
	// WIll need to set game fields based off the gr runner so we know what to run when we call run game
	AddGame(ctx context.Context, filepath string) (*game.Game, error)
	InstallHook(ctx context.Context, game *game.Game) error
	RunGame(ctx context.Context, game *game.Game) (*gr.Process, error)

	StopGame(ctx context.Context, proc *gr.Process) (*gr.Process, error)
	// GetFile will not be supported on TR
	GetFile(g *game.Game, file string) (*EngineFileInfo, error)

	FollowGameText(ctx context.Context, game *game.Game, opts ...FollowGameOptions) (chan Line, error)

	Shutdown() error
	ManagedGames() []*game.Game
	GetTextractor(game *game.Game) *textractor.Client // or nil for other engines
}

type FollowGameOptions struct {
	MaxLines int
	History  bool
	Filters  []func(l *Line) *Line
}

type Line struct {
	Raw     string
	Hook    string
	Text    string
	Speaker string
}

//todo add an option to get/find media files
// like audio and use it in
//https://github.com/Artikash/Textractor/blob/master/texthook/engine/engine.h todo use this to support more engines
//
