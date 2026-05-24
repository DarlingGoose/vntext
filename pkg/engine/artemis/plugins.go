package artemis

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/engine"
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

func (e *Engine) InstallPlugin(plugin *engine.Plugin) error {
	if plugin == nil {
		return errors.New("plugin is nil")
	}
	if normalizeArtemisPluginName(plugin.Name) != artemisLoggerPluginName {
		return fmt.Errorf("%w: %s", engine.ErrUnsupportedPlugin, plugin.Name)
	}
	return e.InstallHook(context.Background(), plugin.Game)
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
