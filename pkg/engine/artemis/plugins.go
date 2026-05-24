package artemis

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/game"
)

const artemisLoggerPluginName = "artemis-logger"

func (e *Engine) GetPlugins() []*engine.Plugin {
	data, _ := fs.ReadFile(pluginFS, "plugin/artemis_logger.lua")
	return []*engine.Plugin{
		{
			Name: artemisLoggerPluginName,
			Data: string(data),
		},
	}
}

func (e *Engine) GetPluginsForGame(g *game.Game) []*engine.Plugin {
	plugins := e.GetPlugins()
	installed := artemisLoggerInstalled(g)
	for _, plugin := range plugins {
		if plugin == nil {
			continue
		}
		plugin.Game = g
		plugin.Installed = installed
	}
	return plugins
}

func (e *Engine) InstallPlugin(plugin *engine.Plugin) error {
	if plugin == nil {
		return errors.New("plugin is nil")
	}
	if normalizeArtemisPluginName(plugin.Name) != artemisLoggerPluginName {
		return fmt.Errorf("%w: %s", engine.ErrUnsupportedPlugin, plugin.Name)
	}
	return e.InstallHook(context.Background(), plugin.Game)
}

func artemisLoggerInstalled(g *game.Game) bool {
	if g == nil || strings.TrimSpace(g.GamePath) == "" {
		return false
	}
	roots, err := findExtractedRoots(g.GamePath)
	if err != nil {
		return false
	}
	for _, root := range roots {
		if exists(filepath.Join(root, loggerFileName)) {
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

func normalizeArtemisPluginName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, " ", "-")
	switch name {
	case "logger", "artemis_logger", "artemis-logger.lua":
		return artemisLoggerPluginName
	default:
		return name
	}
}
