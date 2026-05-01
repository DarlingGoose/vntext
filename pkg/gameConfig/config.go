package gameConfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/engine/auto"
	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/util"
)

func InstallGame(inputPath string, installHook bool, configDir string) (*game.Game, engine.Engine, error) {
	inputPath = strings.TrimSpace(inputPath)
	if inputPath == "" {
		return nil, nil, errors.New("game path is required")
	}

	resolvedPath, err := filepath.Abs(inputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve game path: %w", err)
	}

	eng, err := auto.SelectEngine(resolvedPath)
	if err != nil {
		return nil, nil, err
	}

	g, err := eng.InstallGame(resolvedPath)
	if err != nil {
		return nil, nil, fmt.Errorf("install %s game: %w", eng.Name(), err)
	}

	if installHook {
		if err := eng.InstallTextHook(g); err != nil {
			return nil, nil, fmt.Errorf("install %s text hook: %w", eng.Name(), err)
		}
	}
	output := strings.TrimSpace(configDir)
	if output == "" {
		output = DefaultGameConfigPath(g)
	}
	
	if !util.IsDir(output) {
		return nil, nil, errors.New("invalid configDir, must be a directory")
	}

	if err := WriteGameConfig(output, g); err != nil {
		return nil, nil, err
	}

	return g, eng, nil
}

func WriteGameConfig(path string, g *game.Game) error {
	if g == nil {
		return errors.New("game is nil")
	}

	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("output path is required")
	}

	resolvedPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(resolvedPath), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	raw, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal game config: %w", err)
	}
	raw = append(raw, '\n')

	if err := os.WriteFile(resolvedPath, raw, 0o644); err != nil {
		return fmt.Errorf("write game config: %w", err)
	}

	return nil
}
