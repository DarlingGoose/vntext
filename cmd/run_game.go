package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/runner"
	"github.com/DarlingGoose/vntext/pkg/util"
	"github.com/spf13/cobra"
)

type RunGameOptions struct {
	Background bool
	List       bool
	ConfigDir  string
}

func NewRunGameCommand() *cobra.Command {
	var opts RunGameOptions

	cmd := &cobra.Command{
		Use:   "run-game [game-name]",
		Short: "Run an installed visual novel game",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(opts.ConfigDir) == "" {
				opts.ConfigDir = DefaultGameConfigDir()
			}

			games, err := LoadInstalledGames(opts.ConfigDir)
			if err != nil {
				return err
			}

			if len(games) == 0 {
				return fmt.Errorf("no installed games found in %s", opts.ConfigDir)
			}

			if opts.List {
				for _, g := range games {
					fmt.Fprintf(cmd.OutOrStdout(), "%s\n", g.Name)
				}
				return nil
			}

			var selected *game.Game

			if len(args) > 0 {
				selected, err = FindInstalledGame(games, args[0])
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

			status, err := RunSelectedGame(selected, opts)
			if errors.Is(err, runner.IsAlreadyRunning) {
				fmt.Fprintf(cmd.OutOrStdout(), "game already running: %s pid=%d\n", selected.Name, status.PID)
				return nil
			}
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "started %s", selected.Name)
			if status != nil && status.PID > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), " pid=%d", status.PID)
			}
			fmt.Fprintln(cmd.OutOrStdout())

			return nil
		},
	}

	cmd.Flags().BoolVarP(
		&opts.Background,
		"background",
		"b",
		true,
		"run game in background",
	)

	cmd.Flags().BoolVarP(
		&opts.List,
		"list",
		"l",
		false,
		"list installed games",
	)

	cmd.Flags().StringVar(
		&opts.ConfigDir,
		"config-dir",
		"",
		"installed game config directory; defaults to ~/.config/vntext/games",
	)

	return cmd
}

func init() {
	rootCmd.AddCommand(NewRunGameCommand())
}

func RunSelectedGame(g *game.Game, opts RunGameOptions) (*runner.ProcessStatus, error) {
	r := runner.New()

	if opts.Background {
		return r.RunBackground(g)
	}

	return r.Run(g)
}

func LoadInstalledGames(configDir string) ([]*game.Game, error) {
	configDir = strings.TrimSpace(configDir)
	if configDir == "" {
		configDir = DefaultGameConfigDir()
	}

	entries, err := os.ReadDir(configDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read game config dir: %w", err)
	}

	var games []*game.Game

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}

		path := filepath.Join(configDir, entry.Name())

		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read game config %s: %w", path, err)
		}

		var g game.Game
		if err := json.Unmarshal(raw, &g); err != nil {
			return nil, fmt.Errorf("parse game config %s: %w", path, err)
		}

		if strings.TrimSpace(g.Name) == "" {
			g.Name = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}

		games = append(games, &g)
	}

	sort.Slice(games, func(i, j int) bool {
		return strings.ToLower(games[i].Name) < strings.ToLower(games[j].Name)
	})

	return games, nil
}

func FindInstalledGame(games []*game.Game, query string) (*game.Game, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("game name is required")
	}

	wanted := util.SanitizeName(query)

	var matches []*game.Game

	for _, g := range games {
		if g == nil {
			continue
		}

		name := strings.TrimSpace(g.Name)
		cleanName := util.SanitizeName(name)

		switch {
		case strings.EqualFold(name, query):
			matches = append(matches, g)
		case strings.EqualFold(cleanName, wanted):
			matches = append(matches, g)
		case strings.Contains(strings.ToLower(name), strings.ToLower(query)):
			matches = append(matches, g)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("game %q not found", query)
	}

	if len(matches) > 1 {
		names := make([]string, 0, len(matches))
		for _, match := range matches {
			names = append(names, match.Name)
		}
		sort.Strings(names)

		return nil, fmt.Errorf("game %q is ambiguous; matched: %s", query, strings.Join(names, ", "))
	}

	return matches[0], nil
}

func DefaultGameConfigDir() string {
	return filepath.Join(configBaseDir(), "games")
}
