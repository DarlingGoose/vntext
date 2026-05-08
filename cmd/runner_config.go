package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/DarlingGoose/gr"
	"github.com/DarlingGoose/gr/gamescope"
	"github.com/DarlingGoose/gr/wine"
	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/gameConfig"
	"github.com/spf13/cobra"
)

type RunnerConfigOptions struct {
	ConfigDir             string
	Import                string
	Export                string
	ImportWineConfig      string
	ExportWineConfig      string
	ImportGamescopeConfig string
	ExportGamescopeConfig string
	NoSave                bool

	Runner       string
	RunnerPath   string
	WineBin      string
	GamescopeBin string
	WinePrefix   string
	WorkingDir   string
	Arch         string
	Background   bool

	ClearEnv     bool
	Env          []string
	ClearDeps    bool
	Dependencies []string

	Resolution       string
	OutputResolution string
	RefreshRate      int
	Fullscreen       bool
	Borderless       bool
	ForceGrab        bool
	SteamDeck        bool
	ExposeWayland    bool
	Scaler           string
	Filter           string
	ExtraArgs        []string
	ClearExtraArgs   bool
}

type runnerProfile struct {
	Runner          game.RunnerType `json:"runner"`
	RunnerPath      string          `json:"runner_path,omitempty"`
	PrefixPath      string          `json:"prefix_path,omitempty"`
	WorkingDir      string          `json:"working_dir,omitempty"`
	RunnerConfig    gr.Config       `json:"runner_config,omitempty"`
	WineConfig      any             `json:"wine_config,omitempty"`
	GamescopeConfig any             `json:"gamescope_config,omitempty"`
}

func NewRunnerConfigCommand() *cobra.Command {
	var opts RunnerConfigOptions

	cmd := &cobra.Command{
		Use:   "runner-config [game-name]",
		Short: "Show or update per-game runner settings",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(opts.ConfigDir) == "" {
				opts.ConfigDir = DefaultGameConfigDir()
			}

			games, err := gameConfig.LoadInstalledGames(opts.ConfigDir, DefaultGameConfigDir())
			if err != nil {
				return err
			}
			if len(games) == 0 {
				return fmt.Errorf("no installed games found in %s", opts.ConfigDir)
			}

			selected, err := selectInstalledGame(games, args)
			if err != nil || selected == nil {
				return err
			}

			changed, err := applyRunnerConfigFlags(cmd, selected, opts)
			if err != nil {
				return err
			}

			if strings.TrimSpace(opts.Export) != "" {
				if err := writeRunnerProfile(opts.Export, selected); err != nil {
					return err
				}
			}
			if strings.TrimSpace(opts.ExportWineConfig) != "" {
				if err := wineConfigForGame(selected).Save(opts.ExportWineConfig); err != nil {
					return err
				}
			}
			if strings.TrimSpace(opts.ExportGamescopeConfig) != "" {
				if err := gamescopeConfigForGame(selected).Save(opts.ExportGamescopeConfig); err != nil {
					return err
				}
			}

			if changed && !opts.NoSave {
				if err := gameConfig.WriteGameConfig(installedHookConfigPath(opts.ConfigDir, selected), selected); err != nil {
					return err
				}
			}

			return printRunnerProfile(cmd, selected)
		},
	}

	cmd.Flags().StringVar(&opts.ConfigDir, "config-dir", "", "installed game config directory; defaults to ~/.config/vntext/games")
	cmd.Flags().StringVar(&opts.Import, "import", "", "load runner profile JSON from this path")
	cmd.Flags().StringVar(&opts.Export, "export", "", "write runner profile JSON to this path")
	cmd.Flags().StringVar(&opts.ImportWineConfig, "import-wine-config", "", "load native GR wine options JSON from this path")
	cmd.Flags().StringVar(&opts.ExportWineConfig, "export-wine-config", "", "write native GR wine options JSON to this path")
	cmd.Flags().StringVar(&opts.ImportGamescopeConfig, "import-gamescope-config", "", "load native GR gamescope options JSON from this path")
	cmd.Flags().StringVar(&opts.ExportGamescopeConfig, "export-gamescope-config", "", "write native GR gamescope options JSON to this path")
	cmd.Flags().BoolVar(&opts.NoSave, "no-save", false, "do not write changes back to the installed game config")

	cmd.Flags().StringVar(&opts.Runner, "runner", "", "runner to use: wine or gamescope")
	cmd.Flags().StringVar(&opts.RunnerPath, "runner-path", "", "primary runner binary path; wine for wine runner, gamescope for gamescope runner")
	cmd.Flags().StringVar(&opts.WineBin, "wine-bin", "", "Wine binary path")
	cmd.Flags().StringVar(&opts.GamescopeBin, "gamescope-bin", "", "gamescope binary path")
	cmd.Flags().StringVar(&opts.WinePrefix, "wine-prefix", "", "Wine prefix path")
	cmd.Flags().StringVar(&opts.WorkingDir, "working-dir", "", "working directory used when launching")
	cmd.Flags().StringVar(&opts.Arch, "arch", "", "Wine architecture/system arch, such as win32 or win64")
	cmd.Flags().BoolVar(&opts.Background, "background", false, "run in background")

	cmd.Flags().BoolVar(&opts.ClearEnv, "clear-env", false, "clear stored runner environment variables before applying --env")
	cmd.Flags().StringArrayVar(&opts.Env, "env", nil, "append runner env var KEY=VALUE")
	cmd.Flags().BoolVar(&opts.ClearDeps, "clear-deps", false, "clear stored runner dependencies before applying --dependency")
	cmd.Flags().StringArrayVar(&opts.Dependencies, "dependency", nil, "append runner dependency")

	cmd.Flags().StringVar(&opts.Resolution, "resolution", "", "gamescope internal resolution WIDTHxHEIGHT")
	cmd.Flags().StringVar(&opts.OutputResolution, "output-resolution", "", "gamescope output resolution WIDTHxHEIGHT")
	cmd.Flags().IntVar(&opts.RefreshRate, "refresh-rate", 0, "gamescope refresh rate")
	cmd.Flags().BoolVar(&opts.Fullscreen, "fullscreen", false, "enable gamescope fullscreen")
	cmd.Flags().BoolVar(&opts.Borderless, "borderless", false, "enable gamescope borderless mode")
	cmd.Flags().BoolVar(&opts.ForceGrab, "force-grab", false, "force gamescope cursor grab")
	cmd.Flags().BoolVar(&opts.SteamDeck, "steam-deck", false, "enable gamescope Steam Deck mode")
	cmd.Flags().BoolVar(&opts.ExposeWayland, "expose-wayland", false, "expose nested Wayland display")
	cmd.Flags().StringVar(&opts.Scaler, "scaler", "", "gamescope scaler")
	cmd.Flags().StringVar(&opts.Filter, "filter", "", "gamescope filter")
	cmd.Flags().StringArrayVar(&opts.ExtraArgs, "gamescope-arg", nil, "append raw gamescope argument")
	cmd.Flags().BoolVar(&opts.ClearExtraArgs, "clear-gamescope-args", false, "clear raw gamescope args before applying --gamescope-arg")

	return cmd
}

func init() {
	rootCmd.AddCommand(NewRunnerConfigCommand())
}

func selectInstalledGame(games []*game.Game, args []string) (*game.Game, error) {
	if len(args) > 0 {
		return gameConfig.FindInstalledGame(games, args[0])
	}
	return PickGameTUI(games)
}

func applyRunnerConfigFlags(cmd *cobra.Command, g *game.Game, opts RunnerConfigOptions) (bool, error) {
	changed := false

	if strings.TrimSpace(opts.Import) != "" {
		if err := readRunnerProfile(opts.Import, g); err != nil {
			return false, err
		}
		changed = true
	}
	if strings.TrimSpace(opts.ImportWineConfig) != "" {
		cfg, err := wine.LoadOptions(opts.ImportWineConfig)
		if err != nil {
			return false, err
		}
		g.WineConfig = &cfg
		if g.Runner == "" {
			g.Runner = game.RunnerWine
		}
		changed = true
	}
	if strings.TrimSpace(opts.ImportGamescopeConfig) != "" {
		cfg, err := gamescope.LoadOptions(opts.ImportGamescopeConfig)
		if err != nil {
			return false, err
		}
		g.GamescopeConfig = &cfg
		g.Runner = game.RunnerGamescope
		changed = true
	}

	if cmd.Flags().Changed("runner") {
		runner, err := parseRunnerType(opts.Runner)
		if err != nil {
			return false, err
		}
		g.Runner = runner
		changed = true
	}
	if cmd.Flags().Changed("runner-path") {
		g.RunnerPath = strings.TrimSpace(opts.RunnerPath)
		changed = true
	}
	if cmd.Flags().Changed("wine-bin") {
		applyWineBin(g, strings.TrimSpace(opts.WineBin))
		changed = true
	}
	if cmd.Flags().Changed("gamescope-bin") {
		gs := ensureGamescopeConfig(g)
		gs.GamescopeBin = strings.TrimSpace(opts.GamescopeBin)
		if g.Runner == game.RunnerGamescope {
			g.RunnerPath = gs.GamescopeBin
		}
		changed = true
	}
	if cmd.Flags().Changed("wine-prefix") {
		g.PrefixPath = strings.TrimSpace(opts.WinePrefix)
		ensureWineConfig(g).DefaultPrefix = g.PrefixPath
		ensureGamescopeConfig(g).DefaultWinePrefix = g.PrefixPath
		g.RunnerConfig.WinePrefix = g.PrefixPath
		changed = true
	}
	if cmd.Flags().Changed("working-dir") {
		g.WorkingDir = strings.TrimSpace(opts.WorkingDir)
		g.RunnerConfig.WorkingDir = g.WorkingDir
		changed = true
	}
	if cmd.Flags().Changed("arch") {
		g.RunnerConfig.SystemArch = strings.TrimSpace(opts.Arch)
		changed = true
	}
	if cmd.Flags().Changed("background") {
		g.RunnerConfig.Background = opts.Background
		changed = true
	}
	if opts.ClearEnv {
		g.RunnerConfig.Envs = nil
		changed = true
	}
	if len(opts.Env) > 0 {
		for _, env := range opts.Env {
			if !strings.Contains(env, "=") {
				return false, fmt.Errorf("env must be KEY=VALUE: %q", env)
			}
			g.RunnerConfig.Envs = append(g.RunnerConfig.Envs, env)
		}
		changed = true
	}
	if opts.ClearDeps {
		g.RunnerConfig.Dependencies = nil
		changed = true
	}
	if len(opts.Dependencies) > 0 {
		g.RunnerConfig.Dependencies = append(g.RunnerConfig.Dependencies, opts.Dependencies...)
		changed = true
	}

	if cmd.Flags().Changed("resolution") {
		width, height, err := parseSize(opts.Resolution)
		if err != nil {
			return false, fmt.Errorf("resolution: %w", err)
		}
		gs := ensureGamescopeConfig(g)
		gs.Width = width
		gs.Height = height
		changed = true
	}
	if cmd.Flags().Changed("output-resolution") {
		width, height, err := parseSize(opts.OutputResolution)
		if err != nil {
			return false, fmt.Errorf("output-resolution: %w", err)
		}
		gs := ensureGamescopeConfig(g)
		gs.OutputWidth = width
		gs.OutputHeight = height
		changed = true
	}
	if cmd.Flags().Changed("refresh-rate") {
		ensureGamescopeConfig(g).RefreshRate = opts.RefreshRate
		changed = true
	}
	if cmd.Flags().Changed("fullscreen") {
		ensureGamescopeConfig(g).Fullscreen = opts.Fullscreen
		changed = true
	}
	if cmd.Flags().Changed("borderless") {
		ensureGamescopeConfig(g).Borderless = opts.Borderless
		changed = true
	}
	if cmd.Flags().Changed("force-grab") {
		ensureGamescopeConfig(g).ForceGrab = opts.ForceGrab
		changed = true
	}
	if cmd.Flags().Changed("steam-deck") {
		ensureGamescopeConfig(g).SteamDeckMode = opts.SteamDeck
		changed = true
	}
	if cmd.Flags().Changed("expose-wayland") {
		ensureGamescopeConfig(g).ExposeWayland = opts.ExposeWayland
		changed = true
	}
	if cmd.Flags().Changed("scaler") {
		ensureGamescopeConfig(g).Scaler = strings.TrimSpace(opts.Scaler)
		changed = true
	}
	if cmd.Flags().Changed("filter") {
		ensureGamescopeConfig(g).Filter = strings.TrimSpace(opts.Filter)
		changed = true
	}
	if opts.ClearExtraArgs {
		ensureGamescopeConfig(g).ExtraArgs = nil
		changed = true
	}
	if len(opts.ExtraArgs) > 0 {
		gs := ensureGamescopeConfig(g)
		gs.ExtraArgs = append(gs.ExtraArgs, opts.ExtraArgs...)
		changed = true
	}

	return changed, nil
}

func applyWineBin(g *game.Game, bin string) {
	ensureWineConfig(g).WineBin = bin
	if g.Runner == "" || g.Runner == game.RunnerWine {
		g.RunnerPath = bin
		return
	}
	ensureGamescopeConfig(g).WineBin = bin
}

func ensureWineConfig(g *game.Game) *wine.Options {
	if g.WineConfig == nil {
		cfg := wine.ApplyOptions()
		g.WineConfig = &cfg
	}
	return g.WineConfig
}

func ensureGamescopeConfig(g *game.Game) *gamescope.Options {
	if g.GamescopeConfig == nil {
		cfg := gamescope.ApplyOptions()
		g.GamescopeConfig = &cfg
	}
	return g.GamescopeConfig
}

func wineConfigForGame(g *game.Game) wine.Options {
	cfg := wine.ApplyOptions()
	if g.WineConfig != nil {
		cfg = *g.WineConfig
	}
	if strings.TrimSpace(g.RunnerPath) != "" && (g.Runner == "" || g.Runner == game.RunnerWine) {
		cfg.WineBin = g.RunnerPath
	}
	if strings.TrimSpace(cfg.DefaultPrefix) == "" {
		cfg.DefaultPrefix = g.PrefixPath
	}
	return cfg
}

func gamescopeConfigForGame(g *game.Game) gamescope.Options {
	cfg := gamescope.ApplyOptions()
	if g.GamescopeConfig != nil {
		cfg = *g.GamescopeConfig
	}
	if strings.TrimSpace(g.RunnerPath) != "" && g.Runner == game.RunnerGamescope {
		cfg.GamescopeBin = g.RunnerPath
	}
	if strings.TrimSpace(cfg.DefaultWinePrefix) == "" {
		cfg.DefaultWinePrefix = g.PrefixPath
	}
	cfg.UseWine = true
	return cfg
}

func parseRunnerType(value string) (game.RunnerType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "wine", "":
		return game.RunnerWine, nil
	case "gamescope", "game-scope":
		return game.RunnerGamescope, nil
	default:
		return "", fmt.Errorf("unsupported runner %q", value)
	}
}

func parseSize(value string) (int, int, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	left, right, ok := strings.Cut(value, "x")
	if !ok {
		return 0, 0, errors.New("expected WIDTHxHEIGHT")
	}
	width, err := strconv.Atoi(strings.TrimSpace(left))
	if err != nil || width <= 0 {
		return 0, 0, fmt.Errorf("invalid width %q", left)
	}
	height, err := strconv.Atoi(strings.TrimSpace(right))
	if err != nil || height <= 0 {
		return 0, 0, fmt.Errorf("invalid height %q", right)
	}
	return width, height, nil
}

func printRunnerProfile(cmd *cobra.Command, g *game.Game) error {
	raw, err := json.MarshalIndent(runnerProfileFromGame(g), "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(raw))
	return nil
}

func writeRunnerProfile(path string, g *game.Game) error {
	raw, err := json.MarshalIndent(runnerProfileFromGame(g), "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write runner profile: %w", err)
	}
	return nil
}

func readRunnerProfile(path string, g *game.Game) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read runner profile: %w", err)
	}

	var profile struct {
		Runner          game.RunnerType `json:"runner"`
		RunnerPath      string          `json:"runner_path"`
		PrefixPath      string          `json:"prefix_path"`
		WorkingDir      string          `json:"working_dir"`
		RunnerConfig    gr.Config       `json:"runner_config"`
		WineConfig      json.RawMessage `json:"wine_config"`
		GamescopeConfig json.RawMessage `json:"gamescope_config"`
	}
	if err := json.Unmarshal(raw, &profile); err != nil {
		return fmt.Errorf("parse runner profile: %w", err)
	}

	if profile.Runner != "" {
		g.Runner = profile.Runner
	}
	g.RunnerPath = profile.RunnerPath
	g.PrefixPath = profile.PrefixPath
	g.WorkingDir = profile.WorkingDir
	g.RunnerConfig = profile.RunnerConfig
	if len(profile.WineConfig) > 0 && string(profile.WineConfig) != "null" {
		if err := json.Unmarshal(profile.WineConfig, &g.WineConfig); err != nil {
			return fmt.Errorf("parse wine_config: %w", err)
		}
	}
	if len(profile.GamescopeConfig) > 0 && string(profile.GamescopeConfig) != "null" {
		if err := json.Unmarshal(profile.GamescopeConfig, &g.GamescopeConfig); err != nil {
			return fmt.Errorf("parse gamescope_config: %w", err)
		}
	}
	return nil
}

func runnerProfileFromGame(g *game.Game) runnerProfile {
	return runnerProfile{
		Runner:          g.Runner,
		RunnerPath:      g.RunnerPath,
		PrefixPath:      g.PrefixPath,
		WorkingDir:      g.WorkingDir,
		RunnerConfig:    g.RunnerConfig,
		WineConfig:      g.WineConfig,
		GamescopeConfig: g.GamescopeConfig,
	}
}
