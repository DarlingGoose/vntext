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
}
