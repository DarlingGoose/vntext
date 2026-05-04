package kirikiri2

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
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

func (e *Engine) GetFile(g *game.Game, file string) (*engine.EngineFileInfo, error) {
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

	dataDir, err := cachedKiriKiriArchiveDataDir(ctx, archive)
	if err != nil {
		return nil, err
	}

	path, data, err := util.FindFileAndRead(dataDir, file)
	if err != nil {
		return nil, err
	}
	return engine.NewEngineFileInfo(path, data), nil
}

func cachedKiriKiriArchiveDataDir(ctx context.Context, archive string) (string, error) {
	cacheDir, err := kirikiriArchiveCacheDir(archive)
	if err != nil {
		return "", err
	}

	marker := filepath.Join(cacheDir, ".vntext-complete")
	plan := xp3PatchPlan{
		SourceArchive: archive,
		OutputArchive: filepath.Join(filepath.Dir(archive), wglPatchXP3Name),
		WorkDir:       cacheDir,
	}

	if util.IsFile(marker) {
		if err := detectExtractedLayout(&plan); err == nil {
			return plan.DataDir, nil
		}
		_ = os.RemoveAll(cacheDir)
	}

	if err := os.RemoveAll(cacheDir); err != nil {
		return "", fmt.Errorf("clear stale Kirikiri cache: %w", err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create Kirikiri cache dir: %w", err)
	}

	if err := extractXP3ForPatch(ctx, &plan); err != nil {
		_ = os.RemoveAll(cacheDir)
		return "", err
	}
	if err := detectExtractedLayout(&plan); err != nil {
		_ = os.RemoveAll(cacheDir)
		return "", err
	}
	if err := os.WriteFile(marker, []byte("ok\n"), 0o644); err != nil {
		_ = os.RemoveAll(cacheDir)
		return "", fmt.Errorf("mark Kirikiri cache complete: %w", err)
	}

	return plan.DataDir, nil
}

func kirikiriArchiveCacheDir(archive string) (string, error) {
	abs, err := filepath.Abs(archive)
	if err != nil {
		return "", fmt.Errorf("resolve Kirikiri archive path: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat Kirikiri archive: %w", err)
	}

	sum := sha256.Sum256([]byte(fmt.Sprintf(
		"%s\x00%d\x00%d",
		abs,
		info.Size(),
		info.ModTime().UnixNano(),
	)))
	key := hex.EncodeToString(sum[:16])

	return filepath.Join(os.TempDir(), "vntext", "kirikiri2", "xp3-cache", key), nil
}
