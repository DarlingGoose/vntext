package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DarlingGoose/gr"
	grwine "github.com/DarlingGoose/gr/wine"
	"github.com/DarlingGoose/vntext/pkg/app"
	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/gameConfig"

	"github.com/spf13/cobra"
)

var errGameAlreadyRunning = errors.New("game is already running")

const (
	processStatusUnknown = iota
	processStatusRunning
	processStatusExited
	processStatusStopped
)

type RunGameOptions struct {
	Background     bool
	List           bool
	ConfigDir      string
	VirtualDesktop string

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

			games, err := gameConfig.LoadInstalledGames(opts.ConfigDir, DefaultGameConfigDir())
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

			status, err := RunSelectedGame(cmd.Context(), selected, opts)
			if errors.Is(err, errGameAlreadyRunning) {
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
		fmt.Sprintf("installed game config directory; defaults to ~/.config/%s/games", app.Name()),
	)

	cmd.Flags().StringVar(
		&opts.VirtualDesktop,
		"virtual-desktop",
		"",
		"Wine virtual desktop size, such as 1280x720; use off to disable",
	)

	cmd.Flags().BoolVar(
		&opts.Sync,
		"sync",
		false,
		fmt.Sprintf("keep %s alive while the game is running and stop the game when %s exits", app.Name(), app.Name()),
	)

	cmd.Flags().BoolVar(
		&opts.Follow,
		"follow",
		false,
		"follow game text while running; requires --sync",
	)

	cmd.Flags().StringVar(
		&opts.LogFile,
		"log-file",
		"",
		"override text hook log file for engines that follow a log file",
	)

	return cmd
}
func init() {
	rootCmd.AddCommand(NewRunGameCommand())
}

func RunSelectedGame(ctx context.Context, g *game.Game, opts RunGameOptions) (*GameProcessStatus, error) {
	if g == nil {
		return nil, errors.New("game is nil")
	}

	if strings.TrimSpace(opts.VirtualDesktop) != "" {
		copy := *g
		copy.VirtualDesktop = opts.VirtualDesktop
		g = &copy
	}

	eng, err := selectHookEngine(g, "")
	if err != nil {
		return nil, err
	}

	copy := *g
	copy.RunnerConfig.Background = opts.Background || opts.Sync
	proc, err := eng.RunGame(ctx, &copy)
	if err != nil {
		return nil, err
	}
	status := processStatusFromGR(proc)
	if status == nil {
		status = &GameProcessStatus{Status: processStatusExited}
	}
	status.engine = eng
	status.stop = func() error {
		_, err := eng.StopGame(ctx, proc)
		return err
	}
	return status, nil
}

func processStatusFromGR(proc *gr.Process) *GameProcessStatus {
	if proc == nil {
		return nil
	}

	pid := proc.PID
	if pid == 0 && proc.Cmd != nil && proc.Cmd.Process != nil {
		pid = proc.Cmd.Process.Pid
	}

	status := processStatusUnknown
	switch proc.Status {
	case gr.StatusRunning:
		status = processStatusRunning
	case gr.StatusExited:
		status = processStatusExited
	case gr.StatusStopped:
		status = processStatusStopped
	}

	return &GameProcessStatus{
		PID:     pid,
		Message: proc.Status.String(),
		Status:  status,
		proc:    proc,
	}
}

type GameProcessStatus struct {
	PID     int
	Message string
	Status  int

	proc   *gr.Process
	engine engine.EngineV2
	cancel context.CancelFunc
	waitCh chan error
	stop   func() error
}

func (p *GameProcessStatus) Wait() error {
	if p != nil && p.proc != nil && p.proc.WinePID > 0 {
		return waitWinePID(context.Background(), p.proc)
	}
	if p != nil && p.proc != nil && p.proc.Cmd != nil {
		return p.proc.Cmd.Wait()
	}
	if p == nil || p.waitCh == nil {
		return nil
	}
	return <-p.waitCh
}

func (p *GameProcessStatus) Kill() error {
	if p == nil {
		return nil
	}

	if p.cancel != nil {
		p.cancel()
	}

	if p.stop != nil {
		return p.stop()
	}

	if p.proc != nil && p.proc.Cmd != nil && p.proc.Cmd.Process != nil {
		return p.proc.Cmd.Process.Kill()
	}

	return nil
}

func waitWinePID(ctx context.Context, proc *gr.Process) error {
	if proc == nil || proc.WinePID <= 0 {
		return nil
	}
	if strings.TrimSpace(processEnv(proc, "WINEPREFIX")) == "" {
		return errors.New("cannot wait for wine PID without WINEPREFIX")
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		running, err := winePIDRunning(ctx, proc)
		if err != nil {
			return err
		}
		if !running {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func winePIDRunning(ctx context.Context, proc *gr.Process) (bool, error) {
	cmd := exec.CommandContext(ctx, wineBinFromProcess(proc), "tasklist")
	cmd.Env = append([]string(nil), proc.Environ...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("wine tasklist failed while waiting for pid=%d: %w: %s", proc.WinePID, err, strings.TrimSpace(stderr.String()))
	}

	for _, p := range grwine.ParseTasklist(stdout.String()) {
		if p != nil && p.PID == proc.WinePID {
			return true, nil
		}
	}
	return false, nil
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

func DefaultGameConfigDir() string {
	return filepath.Join(configBaseDir(), "games")
}

func SyncGameProcess(
	ctx context.Context,
	out io.Writer,
	g *game.Game,
	status *GameProcessStatus,
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
		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := FollowGameText(sigCtx, out, g, status, opts); err != nil && !errors.Is(err, context.Canceled) {
				fmt.Fprintf(out, "follow text error: %v\n", err)
			}
		}()
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

func FollowGameText(ctx context.Context, out io.Writer, g *game.Game, status *GameProcessStatus, opts RunGameOptions) error {
	if g == nil {
		return errors.New("game is nil")
	}

	eng := statusEngine(status)
	if eng == nil {
		var err error
		eng, err = selectHookEngine(g, "")
		if err != nil {
			return err
		}
	}

	followGame := *g
	if logFile := strings.TrimSpace(opts.LogFile); logFile != "" {
		followGame.TextHookLogFile = logFile
	}

	lines, err := eng.FollowGameText(ctx, &followGame)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case line, ok := <-lines:
			if !ok {
				return nil
			}
			if text := formatFollowLine(line); text != "" {
				fmt.Fprintln(out, text)
			}
		}
	}
}

func statusEngine(status *GameProcessStatus) engine.EngineV2 {
	if status == nil {
		return nil
	}
	return status.engine
}

func formatFollowLine(line engine.Line) string {
	text := strings.TrimSpace(line.Text)
	if text == "" {
		text = strings.TrimSpace(line.Raw)
	}
	if text == "" {
		return ""
	}

	speaker := strings.TrimSpace(line.Speaker)
	if speaker == "" {
		return text
	}
	return speaker + ": " + text
}
