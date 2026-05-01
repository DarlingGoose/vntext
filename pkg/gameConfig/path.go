package gameConfig

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/util"
)

func DefaultGameConfigPath(g *game.Game) string {
	name := strings.TrimSpace(g.Name)
	if name == "" {
		name = "game"
	}

	return filepath.Join(
		ConfigBaseDir(),
		"games",
		util.SanitizeName(name)+".json",
	)
}

func ConfigBaseDir() string {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "vntext")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".vntext")
	}

	return filepath.Join(home, ".config", "vntext")
}
