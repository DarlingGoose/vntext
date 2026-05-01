package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/runner"
	"github.com/DarlingGoose/vntext/pkg/util"
	"github.com/spf13/cobra"
)

type RunGameOptions struct {
	Background bool
	List       bool
	ConfigDir  string

	Sync    bool
	Follow  bool
	LogFile string
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

			if opts.Follow && !opts.Sync {
				return errors.New("--follow requires --sync")
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

			if opts.Sync {
				return SyncGameProcess(cmd.Context(), cmd.OutOrStdout(), selected, status, opts)
			}

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

	cmd.Flags().BoolVar(
		&opts.Sync,
		"sync",
		false,
		"keep vntext alive while the game is running and stop the game when vntext exits",
	)

	cmd.Flags().BoolVar(
		&opts.Follow,
		"follow",
		false,
		"tail the game log while running; requires --sync",
	)

	cmd.Flags().StringVar(
		&opts.LogFile,
		"log-file",
		"",
		"log file to follow when --follow is enabled",
	)

	return cmd
}
func init() {
	rootCmd.AddCommand(NewRunGameCommand())
}

func RunSelectedGame(g *game.Game, opts RunGameOptions) (*runner.ProcessStatus, error) {
	r := runner.New()

	if opts.Sync || opts.Background {
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

func SyncGameProcess(
	ctx context.Context,
	out io.Writer,
	g *game.Game,
	status *runner.ProcessStatus,
	opts RunGameOptions,
) error {
	if status == nil {
		return errors.New("cannot sync game process: nil process status")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	var wg sync.WaitGroup

	if opts.Follow {
		logFile := strings.TrimSpace(opts.LogFile)
		if logFile == "" {
			logFile = DefaultGameLogPath(g)
		}

		if logFile != "" {
			wg.Add(1)
			go func() {
				defer wg.Done()

				err := FollowFile(sigCtx, out, logFile, FollowFileOptions{
					FromEnd:       true,
					PollInterval:  250 * time.Millisecond,
					MissingIsOkay: true,
				})
				if err != nil && !errors.Is(err, context.Canceled) {
					fmt.Fprintf(out, "follow log error: %v\n", err)
				}
			}()
		}
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- status.Wait()
	}()

	var waitErr error

	select {
	case <-sigCtx.Done():
		fmt.Fprintf(out, "stopping %s\n", g.Name)

		if err := status.Kill(); err != nil {
			fmt.Fprintf(out, "failed to stop game pid=%d: %v\n", status.PID, err)
		}

		waitErr = <-waitCh

	case err := <-waitCh:
		waitErr = err
	}

	cancel()
	wg.Wait()

	if waitErr != nil {
		return fmt.Errorf("game exited with error: %w", waitErr)
	}

	fmt.Fprintf(out, "game exited: %s\n", g.Name)
	return nil
}

type FollowFileOptions struct {
	FromEnd       bool
	PollInterval  time.Duration
	MissingIsOkay bool
}

func FollowFile(ctx context.Context, out io.Writer, path string, opts FollowFileOptions) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("log file path is required")
	}

	if opts.PollInterval <= 0 {
		opts.PollInterval = 250 * time.Millisecond
	}

	var f *os.File

	for {
		file, err := os.Open(path)
		if err == nil {
			f = file
			break
		}

		if !opts.MissingIsOkay {
			return fmt.Errorf("open log file %s: %w", path, err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(opts.PollInterval):
		}
	}

	defer f.Close()

	if opts.FromEnd {
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			return fmt.Errorf("seek log file %s: %w", path, err)
		}
	}

	reader := bufio.NewReader(f)

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			fmt.Fprint(out, line)
		}

		if err == nil {
			continue
		}

		if !errors.Is(err, io.EOF) {
			return fmt.Errorf("read log file %s: %w", path, err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(opts.PollInterval):
		}
	}
}

func DefaultGameLogPath(g *game.Game) string {
	if g == nil {
		return ""
	}

	if strings.TrimSpace(g.TextHookLogFile) != "" {
		return g.TextHookLogFile
	}

	return ""
}
