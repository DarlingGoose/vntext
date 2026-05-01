package rpgmaker

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/util"
)

const (
	engineName = "rpgmaker"

	pluginName = "WGLClipboardText"

	defaultLocale = "ja_JP.UTF-8"
)

//go:embed rpgmakerPlugin.js
var pluginSource string
var _ engine.Engine = &Engine{}

type Engine struct {
	PrefixRoot string
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
	projectRoot, detectedEngine, err := resolveProjectRoot(dir)
	if err != nil {
		return nil, err
	}

	exe, err := findExecutable(projectRoot)
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
	name := util.DeriveGameName(projectRoot, exe, info.IsDir())
	prefixPath := e.prefixPathFor(name)
	locale := DetectGameLocale(projectRoot)
	g := &game.Game{
		Name:          name,
		GamePath:      projectRoot,
		Executable:    exe,
		WorkingDir:    filepath.Dir(exe),
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

	if err := e.InstallTextHook(g); err != nil {
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

	jsDir := filepath.Join(projectRoot, "js")
	pluginsDir := filepath.Join(jsDir, "plugins")
	pluginsConfigPath := filepath.Join(jsDir, "plugins.js")
	pluginPath := filepath.Join(pluginsDir, pluginName+".js")

	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return fmt.Errorf("create plugins dir: %w", err)
	}

	if err := os.WriteFile(pluginPath, []byte(pluginSource), 0o644); err != nil {
		return fmt.Errorf("write RPG Maker plugin: %w", err)
	}

	if err := ensurePluginEnabled(pluginsConfigPath, pluginName); err != nil {
		return err
	}

	g.EngineName = detectedEngine
	g.TextHookLogFile = filepath.Join(projectRoot, "wgl-dialogue.log")

	return nil
}

func (e *Engine) IsEngine(dir string) bool {
	root := strings.TrimSpace(dir)
	if root == "" {
		return false
	}

	_, _, err := resolveProjectRoot(root)
	return err == nil
}

func (e *Engine) prefixPathFor(name string) string {
	if strings.TrimSpace(e.PrefixRoot) == "" {
		return ""
	}
	return filepath.Join(e.PrefixRoot, safePathName(name))
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
