package kirikiri2

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/game"
)

const textLoggerPluginName = "kirikiri2-text-logger"

func (e *Engine) GetPlugins() []*engine.Plugin {
	return []*engine.Plugin{
		{
			Name: textLoggerPluginName,
			Data: e.GetDefaultPlugin(),
		},
	}
}

func (e *Engine) GetPluginsForGame(g *game.Game) []*engine.Plugin {
	plugins := e.GetPlugins()
	for _, plugin := range plugins {
		if plugin == nil {
			continue
		}
		plugin.Game = g
		plugin.Installed = kirikiriTextLoggerInstalled(g)
	}
	return plugins
}

func (e *Engine) InstallPlugin(plugin *engine.Plugin) error {
	if plugin == nil {
		return errors.New("plugin is nil")
	}
	if normalizeKiriKiriPluginName(plugin.Name) != textLoggerPluginName {
		return fmt.Errorf("%w: %s", engine.ErrUnsupportedPlugin, plugin.Name)
	}
	if strings.TrimSpace(plugin.Data) != "" {
		if err := e.SetCustomPlugin(plugin.Data); err != nil {
			return err
		}
	}
	return e.InstallHook(context.Background(), plugin.Game)
}

func kirikiriTextLoggerInstalled(g *game.Game) bool {
	if g == nil {
		return false
	}
	root := strings.TrimSpace(g.WorkingDir)
	if root == "" {
		root = filepath.Dir(g.Executable)
	}
	if root == "." || strings.TrimSpace(root) == "" {
		return false
	}
	for _, candidate := range []string{
		filepath.Join(root, "text_logger.tjs"),
		filepath.Join(root, "log.exe"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return true
		}
	}
	return false
}

func (e *Engine) UnInstallPlugin(plugin *engine.Plugin) error {
	if plugin == nil {
		return errors.New("plugin is nil")
	}
	return fmt.Errorf("%w: uninstall is not supported for %s", engine.ErrUnsupportedPlugin, plugin.Name)
}

func normalizeKiriKiriPluginName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, " ", "-")
	switch name {
	case "text-logger", "text-logger.tjs", "kirikiri-text-logger":
		return textLoggerPluginName
	default:
		return name
	}
}
