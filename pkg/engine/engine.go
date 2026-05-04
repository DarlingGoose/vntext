package engine

import (
	"errors"

	"github.com/DarlingGoose/vntext/pkg/game"
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
}

//todo add an option to get/find media files
// like audio and use it in
//https://github.com/Artikash/Textractor/blob/master/texthook/engine/engine.h todo use this to support more engines
//
