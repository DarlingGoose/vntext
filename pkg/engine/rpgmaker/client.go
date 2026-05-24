package rpgmaker

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/engine/enginerun"
	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/util"
)

const (
	engineName = "rpgmaker"

	pluginName                = "WGLClipboardText"
	ollamaTranslatePluginName = "WGLOllamaTranslate"
)

//go:embed rpgmakerPlugin.js
var pluginSource string

//go:embed rpgmakerOllamaTranslatePlugin.js
var ollamaTranslatePluginSource string

var _ engine.Engine = &Engine{}

type Engine struct {
	PrefixRoot    string
	CurrentPlugin string
}

func (e *Engine) GetFile(g *game.Game, file string) (*engine.EngineFileInfo, error) {
	if g == nil {
		return nil, errors.New("game is nil")
	}

	projectRoot, _, err := resolveProjectRoot(g.GamePath)
	if err != nil {
		for _, candidate := range []string{g.WorkingDir, filepath.Dir(g.Executable)} {
			if strings.TrimSpace(candidate) == "" {
				continue
			}
			projectRoot, _, err = resolveProjectRoot(candidate)
			if err == nil {
				break
			}
		}
		if err != nil {
			return nil, err
		}
	}

	path, data, err := util.FindFileAndRead(projectRoot, file)
	if err != nil {
		return nil, err
	}
	return engine.NewEngineFileInfo(path, data), nil
}

func (e *Engine) GetDefaultPlugin() string {
	if e != nil && strings.TrimSpace(e.CurrentPlugin) != "" {
		return e.CurrentPlugin
	}
	return pluginSource
}

func (e *Engine) SetCustomPlugin(data string) error {
	if e == nil {
		return errors.New("engine is nil")
	}
	e.CurrentPlugin = data
	return nil
}

func (e *Engine) GetOllamaTranslatePlugin() string {
	return ollamaTranslatePluginSource
}

func (e *Engine) GetPlugins() []*engine.Plugin {
	return []*engine.Plugin{
		{
			Name: pluginName,
			Data: e.GetDefaultPlugin(),
		},
		{
			Name: ollamaTranslatePluginName,
			Data: e.GetOllamaTranslatePlugin(),
		},
	}
}

func (e *Engine) GetPluginsForGame(g *game.Game) []*engine.Plugin {
	plugins := e.GetPlugins()
	installed := e.installedPluginNames(g)
	for _, plugin := range plugins {
		if plugin == nil {
			continue
		}
		plugin.Game = g
		plugin.Installed = installed[normalizePluginName(plugin.Name)]
	}
	return plugins
}

func (e *Engine) InstallPlugin(plugin *engine.Plugin) error {
	if plugin == nil {
		return errors.New("plugin is nil")
	}
	switch normalizePluginName(plugin.Name) {
	case normalizePluginName(pluginName):
		return e.installRPGMakerPlugin(plugin.Game, pluginName, plugin.Data, true, map[string]any{}, "WGL clipboard/dialogue text hook")
	case normalizePluginName(ollamaTranslatePluginName), "rpgmaker-ollama-translate", "rpgmaker-ollama-trasklate":
		return e.installRPGMakerPlugin(
			plugin.Game,
			ollamaTranslatePluginName,
			firstNonEmpty(plugin.Data, e.GetOllamaTranslatePlugin()),
			true,
			ollamaTranslatePluginParameters(),
			"Local Ollama RPG Maker text translator",
		)
	default:
		return fmt.Errorf("%w: %s", engine.ErrUnsupportedPlugin, plugin.Name)
	}
}

func (e *Engine) installedPluginNames(g *game.Game) map[string]bool {
	out := map[string]bool{}
	projectRoot, err := e.resolvePluginProjectRoot(g)
	if err != nil {
		return out
	}

	configs, _, _, err := readPluginConfigs(filepath.Join(projectRoot, "js", "plugins.js"))
	if err == nil {
		for _, config := range configs {
			if strings.TrimSpace(config.Name) == "" {
				continue
			}
			out[normalizePluginName(config.Name)] = true
		}
	}

	for _, name := range []string{pluginName, ollamaTranslatePluginName} {
		if _, err := os.Stat(filepath.Join(projectRoot, "js", "plugins", name+".js")); err == nil {
			out[normalizePluginName(name)] = true
		}
	}
	return out
}

func (e *Engine) UnInstallPlugin(plugin *engine.Plugin) error {
	if plugin == nil {
		return errors.New("plugin is nil")
	}

	name := plugin.Name
	switch normalizePluginName(plugin.Name) {
	case normalizePluginName(pluginName):
		name = pluginName
	case normalizePluginName(ollamaTranslatePluginName), "rpgmaker-ollama-translate", "rpgmaker-ollama-trasklate":
		name = ollamaTranslatePluginName
	default:
		return fmt.Errorf("%w: %s", engine.ErrUnsupportedPlugin, plugin.Name)
	}

	projectRoot, err := e.resolvePluginProjectRoot(plugin.Game)
	if err != nil {
		return err
	}
	return ensurePluginConfigured(filepath.Join(projectRoot, "js", "plugins.js"), pluginConfig{
		Name:       name,
		Status:     false,
		Parameters: map[string]any{},
	})
}

// New returns an RPG Maker engine installer with sane defaults.
// You can override fields directly if your caller already resolved runner/prefix.
func New() *Engine {
	return &Engine{}
}

func (e *Engine) Name() string {
	return engineName
}

func (e *Engine) IsDirEngine(dir string) bool {
	_, _, err := resolveProjectRoot(dir)
	return err == nil
}

func (e *Engine) InstallGame(dir string) (*game.Game, error) {
	g, err := e.AddGame(context.Background(), dir)
	if err != nil {
		return nil, err
	}
	if err := e.InstallTextHook(g); err != nil {
		return nil, err
	}
	return g, nil
}

func (e *Engine) AddGame(ctx context.Context, dir string) (*game.Game, error) {
	_ = ctx

	projectRoot, detectedEngine, err := resolveProjectRoot(dir)
	if err != nil {
		return nil, err
	}

	resolvedPath, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("stat path: %w", err)
	}

	exe, err := findExecutableForInput(resolvedPath, info, projectRoot)
	if err != nil {
		return nil, err
	}
	name := util.DeriveGameName(projectRoot, exe, info.IsDir())
	prefixPath, err := e.prefixPathFor(name)
	if err != nil {
		return nil, err
	}
	locale := DetectGameLocale(projectRoot)
	g := &game.Game{
		Name:          name,
		GamePath:      projectRoot,
		Executable:    exe,
		Architecture:  detectArchitecture(exe),
		WorkingDir:    filepath.Dir(exe),
		Runner:        game.RunnerWine,
		PrefixPath:    prefixPath,
		RequiresSteam: false,
		CreatedAt:     time.Now(),

		Locale:        locale,
		StageToPrefix: false,

		TextHookLogFile: filepath.Join(projectRoot, "wgl-dialogue.log"),
		EnvVars: []game.EnvVar{
			{Key: "LANG", Value: locale},
			{Key: "LC_ALL", Value: locale},
		},
		EngineName: detectedEngine,
	}

	if icon := findFirstExisting(projectRoot,
		"icon/icon.png",
		"www/icon/icon.png",
		"icon.png",
	); icon != "" {
		g.IconPath = icon
	}

	if image := findLikelyImage(projectRoot); image != "" {
		g.ImagePath = image
	}

	if err := enginerun.ConfigureRunner(g); err != nil {
		return nil, err
	}

	return g, nil
}

func (e *Engine) InstallTextHook(g *game.Game) error {
	if g == nil {
		return errors.New("game is nil")
	}

	projectRoot, detectedEngine, err := resolveProjectRoot(g.GamePath)
	if err != nil {
		// Fall back to executable/working dir because older configs may not have GamePath set cleanly.
		for _, candidate := range []string{g.WorkingDir, filepath.Dir(g.Executable)} {
			if strings.TrimSpace(candidate) == "" {
				continue
			}
			projectRoot, detectedEngine, err = resolveProjectRoot(candidate)
			if err == nil {
				break
			}
		}
		if err != nil {
			return err
		}
	}

	if err := e.installRPGMakerPlugin(g, pluginName, e.GetDefaultPlugin(), true, map[string]any{}, "WGL clipboard/dialogue text hook"); err != nil {
		return err
	}

	g.EngineName = detectedEngine
	g.TextHookLogFile = filepath.Join(projectRoot, "wgl-dialogue.log")

	return nil
}

func (e *Engine) InstallOllamaTranslateHook(g *game.Game, enabled bool) error {
	return e.installRPGMakerPlugin(
		g,
		ollamaTranslatePluginName,
		e.GetOllamaTranslatePlugin(),
		enabled,
		ollamaTranslatePluginParameters(),
		"Local Ollama RPG Maker text translator",
	)
}

func (e *Engine) installRPGMakerPlugin(g *game.Game, name, data string, enabled bool, parameters map[string]any, description string) error {
	projectRoot, err := e.resolvePluginProjectRoot(g)
	if err != nil {
		return err
	}

	jsDir := filepath.Join(projectRoot, "js")
	pluginsDir := filepath.Join(jsDir, "plugins")
	pluginPath := filepath.Join(pluginsDir, name+".js")

	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return fmt.Errorf("create plugins dir: %w", err)
	}

	if err := os.WriteFile(pluginPath, []byte(firstNonEmpty(data, e.pluginSourceForName(name))), 0o644); err != nil {
		return fmt.Errorf("write RPG Maker plugin: %w", err)
	}

	return ensurePluginConfigured(filepath.Join(jsDir, "plugins.js"), pluginConfig{
		Name:        name,
		Status:      enabled,
		Description: description,
		Parameters:  parameters,
	})
}

func (e *Engine) resolvePluginProjectRoot(g *game.Game) (string, error) {
	if g == nil {
		return "", errors.New("game is nil")
	}

	projectRoot, _, err := resolveProjectRoot(g.GamePath)
	if err == nil {
		return projectRoot, nil
	}
	for _, candidate := range []string{g.WorkingDir, filepath.Dir(g.Executable)} {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		projectRoot, _, err = resolveProjectRoot(candidate)
		if err == nil {
			return projectRoot, nil
		}
	}
	return "", err
}

func (e *Engine) pluginSourceForName(name string) string {
	switch normalizePluginName(name) {
	case normalizePluginName(pluginName):
		return e.GetDefaultPlugin()
	case normalizePluginName(ollamaTranslatePluginName):
		return e.GetOllamaTranslatePlugin()
	default:
		return ""
	}
}

func ollamaTranslatePluginParameters() map[string]any {
	return map[string]any{
		"Enabled":               "true",
		"Endpoint":              "http://127.0.0.1:11434/api/generate",
		"Model":                 "llama3.1",
		"Curl Path":             "curl",
		"Source Language":       "Japanese",
		"Target Language":       "English",
		"Translate Dialogue":    "true",
		"Translate Choices":     "true",
		"Translate Window Text": "true",
		"Show Menu Command":     "true",
		"Menu Command Name":     "Ollama Translation",
	}
}

func normalizePluginName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, " ", "-")
	return name
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (e *Engine) IsEngine(dir string) bool {
	root := strings.TrimSpace(dir)
	if root == "" {
		return false
	}

	_, _, err := resolveProjectRoot(root)
	return err == nil
}

func (e *Engine) prefixPathFor(name string) (string, error) {
	if strings.TrimSpace(e.PrefixRoot) == "" {
		return enginerun.PrefixPath(e.Name(), "", name)
	}
	return filepath.Join(e.PrefixRoot, safePathName(name)), nil
}

func detectEngine(projectRoot string) (string, bool) {
	jsDir := filepath.Join(projectRoot, "js")
	pluginsDir := filepath.Join(jsDir, "plugins")
	pluginsConfigPath := filepath.Join(jsDir, "plugins.js")

	if !util.IsDir(pluginsDir) || !util.IsFile(pluginsConfigPath) {
		return "", false
	}

	switch {
	case util.IsFile(filepath.Join(jsDir, "rmmz_core.js")):
		return "RPG Maker MZ", true
	case util.IsFile(filepath.Join(jsDir, "rpg_core.js")):
		return "RPG Maker MV", true
	default:
		return "RPG Maker MV/MZ", true
	}
}

func exeScore(path string) int {
	base := strings.ToLower(filepath.Base(path))

	score := 0
	switch base {
	case "game.exe":
		score += 100
	case "rpg_rt.exe":
		score += 90
	}

	badParts := []string{
		"unins",
		"setup",
		"install",
		"config",
		"crash",
		"nw.exe",
	}
	for _, part := range badParts {
		if strings.Contains(base, part) {
			score -= 50
		}
	}

	return score
}

func safePathName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false

	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "rpgmaker-game"
	}
	return out
}
