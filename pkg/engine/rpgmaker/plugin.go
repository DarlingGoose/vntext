package rpgmaker

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type pluginConfig struct {
	Name        string         `json:"name"`
	Status      bool           `json:"status"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

func readPluginConfigs(pluginsConfigPath string) ([]pluginConfig, string, string, error) {
	data, err := os.ReadFile(pluginsConfigPath)
	if err != nil {
		return nil, "", "", fmt.Errorf("read plugins.js: %w", err)
	}

	content := string(data)
	firstBracket := strings.Index(content, "[")
	lastBracket := strings.LastIndex(content, "]")
	if firstBracket == -1 || lastBracket == -1 || lastBracket < firstBracket {
		return nil, "", "", fmt.Errorf("plugins.js did not contain a recognizable plugin array: %s", pluginsConfigPath)
	}

	var configs []pluginConfig
	if err := json.Unmarshal([]byte(content[firstBracket:lastBracket+1]), &configs); err != nil {
		return nil, "", "", fmt.Errorf("decode plugins.js: %w", err)
	}

	return configs, content[:firstBracket], content[lastBracket+1:], nil
}

func ensurePluginEnabled(pluginsConfigPath, name string) error {
	return ensurePluginConfigured(pluginsConfigPath, pluginConfig{
		Name:        name,
		Status:      true,
		Description: "WGL clipboard/dialogue text hook",
		Parameters:  map[string]any{},
	})
}

func ensurePluginConfigured(pluginsConfigPath string, desired pluginConfig) error {
	configs, prefix, suffix, err := readPluginConfigs(pluginsConfigPath)
	if err != nil {
		return err
	}

	desired.Name = strings.TrimSpace(desired.Name)
	if desired.Name == "" {
		return fmt.Errorf("plugin name is required")
	}
	if desired.Parameters == nil {
		desired.Parameters = map[string]any{}
	}

	found := false
	for i := range configs {
		if strings.TrimSpace(configs[i].Name) == desired.Name {
			configs[i].Status = desired.Status
			if strings.TrimSpace(desired.Description) != "" {
				configs[i].Description = desired.Description
			}
			if configs[i].Parameters == nil {
				configs[i].Parameters = map[string]any{}
			}
			for key, value := range desired.Parameters {
				if _, ok := configs[i].Parameters[key]; !ok {
					configs[i].Parameters[key] = value
				}
			}
			found = true
			break
		}
	}

	if !found {
		configs = append(configs, desired)
	}

	encoded, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return fmt.Errorf("encode plugins.js: %w", err)
	}

	updated := prefix + string(encoded) + suffix
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}

	if err := os.WriteFile(pluginsConfigPath, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write plugins.js: %w", err)
	}

	return nil
}
