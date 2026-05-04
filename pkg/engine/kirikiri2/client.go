package kirikiri2

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/util"
)

const (
	engineName = "kirikiri2"

	defaultLocale = "ja_JP.UTF-8"
)

//go:embed text_logger.tjs
var textLoggerSource string
var _ engine.Engine = &Engine{}

type Engine struct {
	Runner        game.RunnerType
	RunnerPath    string
	PrefixRoot    string
	SteamRoot     string
	Logger        *slog.Logger
	AutoInstall   bool
	AutoStage     bool
	CurrentPlugin string
}

func (e *Engine) GetDefaultPlugin() string {
	return textLoggerSource
}

func (e *Engine) SetCustomPlugin(data string) error {
	e.CurrentPlugin = data
	return nil
}

func New() *Engine {
	return &Engine{
		Runner:      game.RunnerWine,
		PrefixRoot:  defaultPrefixRoot(),
		AutoInstall: true,
		AutoStage:   true,
		Logger:      slog.Default(),
	}
}
func (e *Engine) prefixPathFor(name string) string {
	root := strings.TrimSpace(e.PrefixRoot)
	if root == "" {
		root = defaultPrefixRoot()
	}

	return filepath.Join(root, util.SanitizeName(name))
}

func defaultPrefixRoot() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "vntext", "prefixes")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".vntext", "prefixes")
	}

	return filepath.Join(home, ".config", "vntext", "prefixes")
}

func (e *Engine) Name() string {
	return engineName
}

func (e *Engine) IsDirEngine(dir string) bool {
	root, err := filepath.Abs(strings.TrimSpace(dir))
	if err != nil {
		return false
	}

	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return false
	}

	profile := DetectKiriKiriProfile(root)
	return profile.IsKiriKiri
}

func (e *Engine) InstallGame(dir string) (*game.Game, error) {
	prepared, err := e.EnsureReady(context.Background(), dir)
	if err != nil {
		return nil, err
	}
	return prepared.Game, nil
}

func (e *Engine) InstallTextHook(g *game.Game) error {
	if err := e.ensureGameReadyEnough(g); err != nil {
		return err
	}

	root := g.WorkingDir
	if strings.TrimSpace(root) == "" {
		root = filepath.Dir(g.Executable)
	}

	profile := DetectKiriKiriProfile(root)
	if !profile.IsKiriKiri {
		return fmt.Errorf("game does not look like KiriKiri2/KAG: %s", root)
	}

	if err := e.installXP3TextHook(context.Background(), g, root); err != nil {
		return err
	}

	if err := e.installLogExe(context.Background(), root); err != nil {
		return err
	}

	return nil
}

func (e *Engine) IsEngine(dir string) bool {
	if e == nil {
		return false
	}

	root := strings.TrimSpace(dir)
	if root == "" {
		return false
	}

	info, err := os.Stat(root)
	if err != nil {
		return false
	}

	if !info.IsDir() {
		root = filepath.Dir(root)
		profile := DetectKiriKiriProfileShallow(root)
		return profile.IsKiriKiri
	}

	profile := DetectKiriKiriProfile(root)
	return profile.IsKiriKiri
}

func (e *Engine) GetFile(g *game.Game, file string) ([]byte, error) {
	if err := e.ensureGameReadyEnough(g); err != nil {
		return nil, err
	}
	root := g.WorkingDir
	if strings.TrimSpace(root) == "" {
		root = filepath.Dir(g.Executable)
	}
	ctx := context.Background()
	archive, err := findBestKiriKiriArchive(root)
	if err != nil {
		return nil, err
	}

	workDir, err := os.MkdirTemp("", "wgl-krkr-hook-*")
	if err != nil {
		return nil, fmt.Errorf("create temp hook dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	plan := xp3PatchPlan{
		SourceArchive: archive,
		OutputArchive: filepath.Join(root, wglPatchXP3Name),
		WorkDir:       workDir,
	}
	if err := extractXP3ForPatch(ctx, &plan); err != nil {
		return nil, err
	}

	if err := detectExtractedLayout(&plan); err != nil {
		return nil, err
	}

	_, data, err := util.FindFileAndRead(plan.DataDir, file)
	if strings.Contains(plan.DataDir, "/tmp") {
		_ = os.RemoveAll(plan.DataDir)
	}
	return data, err
}
