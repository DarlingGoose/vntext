package auto

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"

	"github.com/DarlingGoose/vntext/pkg/app"
	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/engine/artemis"
	"github.com/DarlingGoose/vntext/pkg/engine/kirikiri2"
	"github.com/DarlingGoose/vntext/pkg/engine/rpgmaker"
	"github.com/DarlingGoose/vntext/pkg/engine/textreactor"
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
	return DefaultEngineSelectorV2().Select(dir)
}

var (
	defaultSelectorMu sync.Mutex
	defaultSelector   *EngineSelectorV2
	defaultProgram    string
)

func DefaultEngineSelectorV2() *EngineSelectorV2 {
	programName := app.Name()

	defaultSelectorMu.Lock()
	defer defaultSelectorMu.Unlock()

	if defaultSelector == nil || defaultProgram != programName {
		defaultSelector = NewEngineSelectorV2(programName)
		defaultProgram = programName
	}

	return defaultSelector
}

type EngineSelectorV2 struct {
	programName string
	engines     []engine.EngineV2
}

func NewEngineSelectorV2(programName string) *EngineSelectorV2 {
	programName = strings.TrimSpace(programName)
	if programName == "" {
		programName = app.DefaultProgramName
	}
	engines := []engine.EngineV2{
		rpgmaker.New(),
		kirikiri2.New(),
	}

	art, err := artemis.New()
	if err != nil {
		slog.Error("unable to setup artemis game engine", "err", err)
	} else {
		slog.Info("using artimis")
		engines = append(engines, art)
	}

	//should always be last to be checked
	engines = append(engines, &textreactor.Client{ProgramName: programName})
	return &EngineSelectorV2{
		programName: programName,
		engines:     engines,
	}
}

func NewEngineSelector(programName string) *EngineSelectorV2 {
	return NewEngineSelectorV2(programName)
}

func (e *EngineSelectorV2) Select(dir string) (engine.EngineV2, error) {
	for _, eng := range e.engines {
		if eng.IsEngine(dir) {
			return eng, nil
		}
	}
	return nil, engine.ErrNoEngineFound
}

func (e *EngineSelectorV2) ByName(name string) engine.EngineV2 {
	name = normalizeEngineName(name)
	if name == "" {
		return nil
	}

	for _, eng := range e.engines {
		if normalizeEngineName(eng.Name()) == name {
			return eng
		}
	}

	return nil
}

func SelectEngineV2ByName(name string) engine.EngineV2 {
	return DefaultEngineSelectorV2().ByName(name)
}

func normalizeEngineName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, " ", "-")

	switch {
	case name == "kirikiri", name == "krkr", name == "krkr2":
		return "kirikiri2"
	case name == "rpg-maker", strings.HasPrefix(name, "rpgmaker-"), strings.HasPrefix(name, "rpg-maker"):
		return "rpgmaker"
	default:
		return name
	}
}
