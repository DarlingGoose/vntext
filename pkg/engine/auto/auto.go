package auto

import (
	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/engine/kirikiri2"
	"github.com/DarlingGoose/vntext/pkg/engine/rpgmaker"
)

func SelectEngine(dir string) (engine.Engine, error) {
	engineList := []engine.Engine{
		rpgmaker.New(),
		kirikiri2.New(),
	}
	for _, e := range engineList {
		if e.IsEngine(dir) {
			return e, nil
		}
	}
	return nil, engine.ErrNoEngineFound
}
