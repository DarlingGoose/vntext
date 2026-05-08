package auto

import (
	"context"
	"encoding/json"

	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/engine/kirikiri2"
	"github.com/DarlingGoose/vntext/pkg/engine/rpgmaker"

	"github.com/DarlingGoose/vntext/pkg/util"
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
	if util.IsDir(dir) {
		ed, err := DetectEngineFromPath(context.Background(), dir)
		if err == nil && ed.Engine != "unknown" {
			d, _ := json.MarshalIndent(ed, "", "  ")
			println(string(d))
			return nil, engine.ErrNoEngineFound
		}

	}

	return nil, engine.ErrNoEngineFound
}

func SelectEngineV2(dir string) (engine.EngineV2, error) {
	engineList := []engine.EngineV2{
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
