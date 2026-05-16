package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/app"
	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/engine/auto"
	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/gameConfig"
	"github.com/DarlingGoose/vntext/pkg/util"
	"github.com/spf13/cobra"
)

type InstallHookOptions struct {
	ConfigDir       string
	Engine          string
	NoSave          bool
	HookFilter      []string
	ClearHookFilter bool
}

func NewInstallHookCommand() *cobra.Command {
	var opts InstallHookOptions
	cmd := &cobra.Command{
		Use:   "install-hook [game-name]",
		Short: "Install or refresh the text hook for an installed game",
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

			var selected *game.Game
			if len(args) > 0 {
				selected, err = gameConfig.FindInstalledGame(games, args[0])
				if err != nil {
					return err
				}
			} else {
				selected, err = PickGameTUI(games)
				if err != nil {
					return err
				}
				if selected == nil {
					return nil
				}
			}

			eng, err := selectHookEngine(selected, opts.Engine)
			if err != nil {
				return err
			}

			if err := eng.InstallHook(cmd.Context(), selected); err != nil {
				return fmt.Errorf("install %s text hook: %w", eng.Name(), err)
			}

			if strings.TrimSpace(selected.EngineName) == "" {
				selected.EngineName = eng.Name()
			}
			if opts.ClearHookFilter {
				selected.TextHookFilter = nil
			}
			if len(opts.HookFilter) > 0 {
				selected.TextHookFilter = normalizeHookFilters(opts.HookFilter)
			}

			if !opts.NoSave {
				if err := gameConfig.WriteGameConfig(installedHookConfigPath(opts.ConfigDir, selected), selected); err != nil {
					return err
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "installed text hook\n")
			fmt.Fprintf(cmd.OutOrStdout(), "  engine: %s\n", eng.Name())
			fmt.Fprintf(cmd.OutOrStdout(), "  game:   %s\n", selected.Name)
			if strings.TrimSpace(selected.TextHookLogFile) != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  log:    %s\n", selected.TextHookLogFile)
			}
			if len(selected.TextHookFilter) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "  filter: %s\n", strings.Join(selected.TextHookFilter, ", "))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(
		&opts.ConfigDir,
		"config-dir",
		"",
		fmt.Sprintf("installed game config directory; defaults to ~/.config/%s/games", app.Name()),
	)
	cmd.Flags().StringVar(
		&opts.Engine,
		"engine",
		"",
		"engine override, such as kirikiri2 or rpgmaker",
	)
	cmd.Flags().BoolVar(
		&opts.NoSave,
		"no-save",
		false,
		"do not write updated hook metadata back to the game config",
	)
	cmd.Flags().StringArrayVar(
		&opts.HookFilter,
		"hook-filter",
		nil,
		"default Textractor hook group filter; repeat for multiple groups",
	)
	cmd.Flags().BoolVar(
		&opts.ClearHookFilter,
		"clear-hook-filter",
		false,
		"clear default Textractor hook group filters",
	)

	return cmd
}

func init() {
	rootCmd.AddCommand(NewInstallHookCommand())
}

func selectHookEngine(g *game.Game, override string) (engine.EngineV2, error) {
	if g == nil {
		return nil, errors.New("game is nil")
	}

	selector := auto.DefaultEngineSelectorV2()

	if eng := hookEngineByName(selector, override); eng != nil {
		return eng, nil
	}

	if eng := hookEngineByName(selector, g.EngineName); eng != nil {
		return eng, nil
	}

	for _, candidate := range []string{g.GamePath, g.WorkingDir, g.Executable} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		eng, err := selector.Select(candidate)
		if err == nil {
			return eng, nil
		}
	}

	return nil, fmt.Errorf("could not determine engine for %q; pass --engine", g.Name)
}

func hookEngineByName(selector *auto.EngineSelectorV2, name string) engine.EngineV2 {
	if selector == nil {
		selector = auto.DefaultEngineSelectorV2()
	}
	return selector.ByName(name)
}

func installedHookConfigPath(configDir string, g *game.Game) string {
	configDir = strings.TrimSpace(configDir)
	if configDir == "" || configDir == DefaultGameConfigDir() {
		return gameConfig.DefaultGameConfigPath(g)
	}

	name := "game"
	if g != nil && strings.TrimSpace(g.Name) != "" {
		name = util.SanitizeName(g.Name)
	}

	return filepath.Join(configDir, name+".json")
}

func normalizeHookFilters(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if _, ok := seen[part]; ok {
				continue
			}
			out = append(out, part)
			seen[part] = struct{}{}
		}
	}
	return out
}
