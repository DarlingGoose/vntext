package enginerun

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DarlingGoose/gr"
	"github.com/DarlingGoose/gr/autorunner"
	"github.com/DarlingGoose/gr/gamescope"
	"github.com/DarlingGoose/gr/wine"
	"github.com/DarlingGoose/vntext/pkg/app"
	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/util"
)

const DefaultVirtualDesktop = "1280x720"

func PrefixPath(programName, prefixRoot, gameName string) (string, error) {
	if strings.TrimSpace(prefixRoot) != "" {
		return filepath.Join(prefixRoot, util.SanitizeName(gameName)), nil
	}
	return util.GetWinePrefix(programName, gameName)
}

func RunGame(ctx context.Context, g *game.Game) (*gr.Process, error) {
	if err := ValidateGame(g); err != nil {
		return nil, err
	}

	r, err := RunnerForGame(g)
	if err != nil {
		return nil, err
	}

	target, args := WineTarget(g)

	opts, err := WineOptions(g, args...)
	if err != nil {
		return nil, err
	}

	// Games are long-running GUI processes. Always launch them async.
	opts = append(opts, gr.WithBackground(true))

	return r.Run(ctx, target, opts...)
}

func StopGame(ctx context.Context, proc *gr.Process) (*gr.Process, error) {
	if proc == nil {
		return nil, errors.New("process is nil")
	}

	// Prefer killing the host process first.
	// For gamescope this should terminate gamescope + children.
	if proc.Cmd != nil {
		if proc.Cmd.Process != nil {
			if err := proc.Cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				slog.Error("failed to kill process", "err", err)
			}
		}

		time.Sleep(time.Second)

		if proc.Cmd.Cancel != nil {
			if err := proc.Cmd.Cancel(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return proc, err
			}

			proc.Status = gr.StatusStopped
			return proc, nil
		}

		proc.Status = gr.StatusStopped
		return proc, nil
	}

	// Fallback for wine-only process handles.
	if proc.WinePID > 0 {
		if err := stopWineProcess(ctx, proc); err != nil {
			return proc, err
		}

		proc.Status = gr.StatusStopped
		return proc, nil
	}

	if proc.PID <= 0 {
		return proc, errors.New("process PID is empty")
	}

	if err := stopWineProcess(ctx, proc); err != nil {
		return proc, err
	}

	proc.Status = gr.StatusStopped
	return proc, nil
}

func stopWineProcess(ctx context.Context, proc *gr.Process) error {
	prefix := processEnv(proc, "WINEPREFIX")
	if strings.TrimSpace(prefix) == "" {
		return errors.New("cannot stop process without host command or wine prefix")
	}

	pid := proc.WinePID
	if pid <= 0 {
		pid = proc.PID
	}

	cmd := exec.CommandContext(ctx, wineBinFromProcess(proc), "taskkill", "/PID", strconv.Itoa(pid), "/F")
	cmd.Env = append([]string(nil), proc.Environ...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wine taskkill pid=%d: %w", pid, err)
	}
	return nil
}

func processEnv(proc *gr.Process, key string) string {
	if proc == nil {
		return ""
	}
	prefix := key + "="
	for _, env := range proc.Environ {
		if strings.HasPrefix(env, prefix) {
			return env[len(prefix):]
		}
	}
	return ""
}

func wineBinFromProcess(proc *gr.Process) string {
	if proc != nil && len(proc.Cmdline) > 0 {
		cmd := strings.TrimSpace(proc.Cmdline[0])
		if cmd != "" && strings.Contains(strings.ToLower(filepath.Base(cmd)), "wine") {
			return cmd
		}
	}
	return "wine"
}

func RunnerForGame(g *game.Game) (gr.Runner, error) {
	if g.Runner == game.RunnerGamescope {
		return gamescope.NewFromOptions(gamescopeOptionsForGame(g)), nil
	}

	if g.Runner == "" || g.Runner == game.RunnerWine {
		return wine.NewFromOptions(wineOptionsForGame(g)), nil
	}

	return nil, fmt.Errorf("%s runner is not supported by EngineV2 GR launcher", g.Runner)
}

func wineOptionsForGame(g *game.Game) wine.Options {
	cfg := wine.ApplyOptions()
	if hasWineConfig(g.WineConfig) {
		cfg = *g.WineConfig
	}
	if strings.TrimSpace(g.RunnerPath) != "" {
		cfg.WineBin = g.RunnerPath
	}
	if strings.TrimSpace(cfg.DefaultPrefix) == "" {
		cfg.DefaultPrefix = g.PrefixPath
	}
	if strings.TrimSpace(cfg.WineBin) == "" {
		cfg.WineBin = "wine"
	}
	if strings.TrimSpace(cfg.WineTricksBin) == "" {
		cfg.WineTricksBin = "winetricks"
	}
	if strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = "wine"
	}
	return cfg
}

func gamescopeOptionsForGame(g *game.Game) gamescope.Options {
	cfg := gamescope.ApplyOptions()
	if hasGamescopeConfig(g.GamescopeConfig) {
		cfg = *g.GamescopeConfig
	}
	cfg.UseWine = true
	cfg.Fullscreen = true
	if strings.TrimSpace(g.RunnerPath) != "" {
		cfg.GamescopeBin = g.RunnerPath
	}
	if strings.TrimSpace(cfg.DefaultWinePrefix) == "" {
		cfg.DefaultWinePrefix = g.PrefixPath
	}
	if strings.TrimSpace(cfg.GamescopeBin) == "" {
		cfg.GamescopeBin = "gamescope"
	}
	if strings.TrimSpace(cfg.WineBin) == "" {
		cfg.WineBin = "wine"
	}
	if strings.TrimSpace(cfg.WineServerBin) == "" {
		cfg.WineServerBin = "wineserver"
	}
	if strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = "gamescope"
	}
	return cfg
}

func hasWineConfig(c *wine.Options) bool {
	return c != nil && (c.Name != "" ||
		c.WineBin != "" ||
		c.WineTricksBin != "" ||
		c.DefaultPrefix != "")
}

func hasGamescopeConfig(c *gamescope.Options) bool {
	return c != nil && (c.Name != "" ||
		c.GamescopeBin != "" ||
		c.WineBin != "" ||
		c.WineServerBin != "" ||
		c.DefaultWinePrefix != "" ||
		c.UseWine ||
		c.WineStartWait ||
		c.KillWineOnExit ||
		c.Width != 0 ||
		c.Height != 0 ||
		c.RefreshRate != 0 ||
		c.OutputWidth != 0 ||
		c.OutputHeight != 0 ||
		c.Fullscreen ||
		c.Borderless ||
		c.ForceGrab ||
		c.SteamDeckMode ||
		c.ExposeWayland ||
		c.Scaler != "" ||
		c.Filter != "" ||
		len(c.ExtraArgs) > 0)
}

func WineOptions(g *game.Game, args ...string) ([]gr.Option, error) {
	background := g.RunnerConfig.Background

	if hasRunnerConfig(g.RunnerConfig) {
		cfg := g.RunnerConfig
		applyGameLocaleConfig(g, &cfg)
		opts := cfg.Options()
		if len(args) > 0 {
			opts = append(opts, gr.WithArgs(args...))
		}
		return opts, nil
	}

	defaults, err := autorunner.AutoOptionsForExe(g.Executable, autorunner.DefaultOptionsConfig{
		WinePrefix: g.PrefixPath,
		WorkingDir: WorkingDir(g),
		WineBin:    WineBin(g),
		Args:       args,
		Env:        WineEnv(g),
	})
	if err != nil {
		return nil, err
	}
	opts := defaults.Options
	if background {
		opts = append(opts, gr.WithBackground(true))
	}
	return opts, nil
}

func ConfigureRunner(g *game.Game) error {
	if g == nil {
		return errors.New("game is nil")
	}
	defaults, err := autorunner.AutoOptionsForExe(g.Executable, autorunner.DefaultOptionsConfig{
		WinePrefix: g.PrefixPath,
		WorkingDir: WorkingDir(g),
		WineBin:    WineBin(g),
		Env:        WineEnv(g),
	})
	if err != nil {
		g.RunnerConfig = fallbackConfig(g)
		return nil
	}
	g.RunnerConfig = gr.NewConfig(defaults.Options...)
	applyGameLocaleConfig(g, &g.RunnerConfig)
	return nil
}

func fallbackConfig(g *game.Game) gr.Config {
	opts := make([]gr.Option, 0, 3)
	if strings.TrimSpace(g.PrefixPath) != "" {
		opts = append(opts, gr.WithWinePrefix(g.PrefixPath))
	}
	if dir := WorkingDir(g); strings.TrimSpace(dir) != "" {
		opts = append(opts, gr.WithWorkingDir(dir))
	}
	if env := autorunner.RecommendedWineEnv(WineEnv(g)); len(env) > 0 {
		opts = append(opts, gr.WithEnv(env...))
	}
	cfg := gr.NewConfig(opts...)
	applyGameLocaleConfig(g, &cfg)
	return cfg
}

func WineBin(g *game.Game) string {
	if g != nil && g.Runner == game.RunnerGamescope {
		return ""
	}
	if g == nil {
		return ""
	}
	return g.RunnerPath
}

func hasRunnerConfig(c gr.Config) bool {
	return c.WorkingDir != "" ||
		len(c.Args) > 0 ||
		len(c.Envs) > 0 ||
		c.SystemArch != "" ||
		c.WinePrefix != "" ||
		len(c.Dependencies) > 0 ||
		c.Name != "" ||
		c.PID != 0 ||
		c.Session != "" ||
		c.SessionID != ""
}

func WineEnv(g *game.Game) autorunner.WineEnvConfig {
	cfg := autorunner.DefaultWineEnvConfig()
	cfg.Lang = ""

	for _, kv := range g.EnvVars {
		key := strings.TrimSpace(kv.Key)
		if key != "" {
			cfg.Extra = append(cfg.Extra, key+"="+kv.Value)
		}
	}

	if locale := GameWineLocale(g); locale != "" {
		cfg.Lang = locale
		cfg.Extra = append(cfg.Extra,
			"LANG="+locale,
			"LC_ALL="+locale,
			"LC_CTYPE="+locale,
			"LC_MESSAGES="+locale,
		)
	}

	return cfg
}

func applyGameLocaleConfig(g *game.Game, cfg *gr.Config) {
	if g == nil || cfg == nil {
		return
	}
	locale := GameWineLocale(g)
	if locale == "" {
		return
	}
	cfg.Envs = upsertEnvSpec(cfg.Envs, "LANG="+locale)
	cfg.Envs = upsertEnvSpec(cfg.Envs, "LC_ALL="+locale)
	cfg.Envs = upsertEnvSpec(cfg.Envs, "LC_CTYPE="+locale)
	cfg.Envs = upsertEnvSpec(cfg.Envs, "LC_MESSAGES="+locale)
}

func GameWineLocale(g *game.Game) string {
	if g == nil {
		return ""
	}
	if locale := strings.TrimSpace(g.Locale); locale != "" {
		return locale
	}
	if exe := strings.TrimSpace(g.Executable); exe != "" {
		if locale, err := autorunner.DetectWineLang(exe); err == nil && strings.TrimSpace(locale) != "" {
			return strings.TrimSpace(locale)
		}
	}
	return ""
}

func upsertEnvSpec(envs []string, spec string) []string {
	key, _, ok := strings.Cut(spec, "=")
	if !ok || strings.TrimSpace(key) == "" {
		return append(envs, spec)
	}

	prefix := key + "="
	for i, existing := range envs {
		if strings.HasPrefix(existing, prefix) {
			envs[i] = spec
			return envs
		}
	}
	return append(envs, spec)
}

func WineTarget(g *game.Game) (string, []string) {
	if desktop := WineVirtualDesktop(g); desktop != "" {
		return "explorer", []string{"/desktop=" + app.Name() + "," + desktop, WindowsPathForWine(g)}
	}
	return g.Executable, nil
}

func WineVirtualDesktop(g *game.Game) string {
	if g == nil {
		return ""
	}

	switch desktop := strings.TrimSpace(g.VirtualDesktop); strings.ToLower(desktop) {
	case "off", "false", "none", "disabled", "disable", "0":
		return ""
	case "":
		return DefaultVirtualDesktop
	default:
		return desktop
	}
}

func WindowsPathForWine(g *game.Game) string {
	exe := g.Executable
	if strings.TrimSpace(g.PrefixPath) == "" {
		return exe
	}

	rel, err := filepath.Rel(filepath.Join(g.PrefixPath, "drive_c"), exe)
	if err != nil || strings.HasPrefix(rel, "..") {
		return exe
	}

	return `C:\` + strings.ReplaceAll(filepath.ToSlash(rel), "/", `\`)
}

func ValidateGame(g *game.Game) error {
	if g == nil {
		return errors.New("game is nil")
	}
	if strings.TrimSpace(g.Executable) == "" {
		return errors.New("game executable is required")
	}
	if strings.TrimSpace(g.PrefixPath) == "" {
		return errors.New("wine prefix path is required")
	}
	if _, err := os.Stat(g.Executable); err != nil {
		return fmt.Errorf("game executable is not accessible: %s: %w", g.Executable, err)
	}
	return nil
}

func WorkingDir(g *game.Game) string {
	if strings.TrimSpace(g.WorkingDir) != "" {
		return g.WorkingDir
	}
	if strings.TrimSpace(g.Executable) != "" {
		return filepath.Dir(g.Executable)
	}
	if strings.TrimSpace(g.GamePath) != "" {
		return g.GamePath
	}
	return "."
}
