package textreactor

import (
	"fmt"

	"github.com/DarlingGoose/vntext/pkg/engine"
)

func (c *Client) GetPlugins() []*engine.Plugin {
	return []*engine.Plugin{}
}

func (c *Client) InstallPlugin(plugin *engine.Plugin) error {
	if plugin == nil {
		return fmt.Errorf("plugin is nil")
	}
	return fmt.Errorf("%w: %s", engine.ErrUnsupportedPlugin, plugin.Name)
}

func (c *Client) UnInstallPlugin(plugin *engine.Plugin) error {
	if plugin == nil {
		return fmt.Errorf("plugin is nil")
	}
	return fmt.Errorf("%w: %s", engine.ErrUnsupportedPlugin, plugin.Name)
}
