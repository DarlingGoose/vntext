// pkg/engines/kirikiri2/setup.go
package kirikiri2

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/util"
)

type PreparedGame struct {
	Game         *game.Game
	SourceRoot   string
	WasInstalled bool
	WasStaged    bool
	Profile      KiriKiriProfile
}

// EnsureReady is the thing to call before doing anything else.
// It normalizes installer folders, loose game folders, and already-installed dirs
// into a runnable/staged game.Game.
func (e *Engine) EnsureReady(ctx context.Context, inputPath string) (*PreparedGame, error) {
	logger := e.logger()

	resolvedPath, err := filepath.Abs(strings.TrimSpace(inputPath))
	if err != nil {
		return nil, fmt.Errorf("resolve input path: %w", err)
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("stat input path: %w", err)
	}

	sourceRoot := resolvedPath
	if !info.IsDir() {
		sourceRoot = filepath.Dir(resolvedPath)
	}

	var profile KiriKiriProfile
	if !info.IsDir() {
		profile = DetectKiriKiriProfileShallow(sourceRoot)
	} else {
		profile = DetectKiriKiriProfile(sourceRoot)
	}

	if !profile.IsKiriKiri {
		return nil, fmt.Errorf("not a KiriKiri2/KAG game: %s", resolvedPath)
	}

	exe, workingDir, exeErr := resolveKiriKiriExecutable(resolvedPath, info)
	arch := detectExecutableArchitecture(exe)

	name := util.DeriveGameName(resolvedPath, exe, info.IsDir())
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(sourceRoot)
	}

	g := &game.Game{
		Name:            name,
		GamePath:        sourceRoot,
		Executable:      exe,
		Architecture:    arch,
		WorkingDir:      workingDir,
		Runner:          e.runner(),
		RunnerPath:      e.RunnerPath,
		PrefixPath:      e.prefixPathFor(name),
		RequiresSteam:   false,
		CreatedAt:       time.Now().UTC(),
		Locale:          defaultLocale,
		StageToPrefix:   true,
		TextHookLogFile: filepath.Join(sourceRoot, "vntext.log"),
		EngineName:      e.Name(),
		EnvVars: []game.EnvVar{
			{Key: "LANG", Value: defaultLocale},
			{Key: "LC_ALL", Value: defaultLocale},
		},
	}

	if profile.HasMojibakeName || profile.HasJapaneseName || profile.HasTJS || profile.HasXP3 {
		g.Locale = defaultLocale
	}

	scan := scanInstallerArtifacts(sourceRoot)

	shouldInstall := e.AutoInstall &&
		info.IsDir() &&
		len(scan.Installers) > 0 &&
		(exeErr != nil || scan.LooksInstallerOnly())

	if shouldInstall {
		installer, ok := scan.BestInstaller()
		if !ok {
			return nil, errors.New("installer artifacts found, but no usable setup/msi installer was detected")
		}

		if strings.TrimSpace(g.PrefixPath) == "" {
			return nil, errors.New("prefix path is required to install KiriKiri game")
		}
		if strings.TrimSpace(string(g.Architecture)) == "" {
			g.Architecture = detectExecutableArchitecture(installer.Path)
		}

		if err := os.MkdirAll(g.PrefixPath, 0o755); err != nil {
			return nil, fmt.Errorf("create prefix: %w", err)
		}

		logger.InfoContext(ctx,
			"running KiriKiri installer",
			"installer", installer.Path,
			"type", installer.Type,
			"prefix", g.PrefixPath,
		)

		if err := e.runInstaller(ctx, g, installer); err != nil {
			return nil, fmt.Errorf("run installer: %w", err)
		}

		installedExe, err := findInstalledKiriKiriExecutable(g.PrefixPath)
		if err != nil {
			return nil, err
		}

		g.Executable = installedExe
		g.Architecture = detectExecutableArchitecture(installedExe)
		g.WorkingDir = filepath.Dir(installedExe)
		g.GamePath = g.WorkingDir
		g.StageToPrefix = false
		g.StagedPath = ""
		g.TextHookLogFile = filepath.Join(g.WorkingDir, "vntext.log")

		return &PreparedGame{
			Game:         g,
			SourceRoot:   sourceRoot,
			WasInstalled: true,
			WasStaged:    false,
			Profile:      profile,
		}, nil
	}

	if exeErr != nil {
		return nil, exeErr
	}

	if e.AutoStage && g.StageToPrefix {
		logger.InfoContext(ctx,
			"staging KiriKiri game into prefix",
			"source", g.WorkingDir,
			"prefix", g.PrefixPath,
		)

		if err := stageGameIntoPrefix(g); err != nil {
			return nil, err
		}

		g.TextHookLogFile = filepath.Join(g.WorkingDir, "vntext.log")

		return &PreparedGame{
			Game:         g,
			SourceRoot:   sourceRoot,
			WasInstalled: false,
			WasStaged:    true,
			Profile:      profile,
		}, nil
	}

	return &PreparedGame{
		Game:         g,
		SourceRoot:   sourceRoot,
		WasInstalled: false,
		WasStaged:    false,
		Profile:      profile,
	}, nil
}

func (e *Engine) logger() *slog.Logger {
	if e.Logger != nil {
		return e.Logger
	}
	return slog.Default()
}

func (e *Engine) runner() game.RunnerType {
	if e.Runner == "" {
		return game.RunnerWine
	}
	return e.Runner
}

func detectExecutableArchitecture(path string) game.Architecture {
	if strings.TrimSpace(path) == "" {
		return ""
	}

	arch, err := game.DetectExecutableArchitecture(path)
	if err != nil {
		return ""
	}
	return arch
}

func (e *Engine) ensureGameReadyEnough(g *game.Game) error {
	if g == nil {
		return errors.New("game is nil")
	}

	if strings.TrimSpace(g.Executable) == "" {
		return errors.New("game executable is empty; call EnsureReady or InstallGame first")
	}

	if strings.TrimSpace(g.WorkingDir) == "" {
		g.WorkingDir = filepath.Dir(g.Executable)
	}

	if strings.TrimSpace(g.GamePath) == "" {
		g.GamePath = g.WorkingDir
	}

	if strings.TrimSpace(g.EngineName) == "" {
		g.EngineName = e.Name()
	}

	if strings.TrimSpace(g.Locale) == "" {
		g.Locale = defaultLocale
	}

	if strings.TrimSpace(string(g.Architecture)) == "" {
		g.Architecture = detectExecutableArchitecture(g.Executable)
	}

	if strings.TrimSpace(g.TextHookLogFile) == "" {
		g.TextHookLogFile = filepath.Join(g.WorkingDir, "vntext.log")
	}

	if _, err := os.Stat(g.Executable); err != nil {
		return fmt.Errorf("game executable is not accessible: %s: %w", g.Executable, err)
	}

	if info, err := os.Stat(g.WorkingDir); err != nil {
		return fmt.Errorf("game working dir is not accessible: %s: %w", g.WorkingDir, err)
	} else if !info.IsDir() {
		return fmt.Errorf("game working dir is not a directory: %s", g.WorkingDir)
	}

	return nil
}
