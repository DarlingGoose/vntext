package runner

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/util"
)

type ProtonRunner struct {
	SteamRoot string
}

func (r *ProtonRunner) Run(g *game.Game) (*ProcessStatus, error) {
	cmd, err := r.command(g)
	if err != nil {
		return nil, err
	}

	if status, err := r.IsRunning(g); err == nil && status.Status == StatusRunning {
		return status, IsAlreadyRunning
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	_ = writePIDFile(g, cmd.Process.Pid)

	err = cmd.Wait()
	status := processStatusFromCmd(cmd, "game exited")
	status.Status = StatusExited
	_ = os.Remove(pidFilePath(g))

	return status, err
}

func (r *ProtonRunner) RunBackground(g *game.Game) (*ProcessStatus, error) {
	cmd, err := r.command(g)
	if err != nil {
		return nil, err
	}

	if status, err := r.IsRunning(g); err == nil && status.Status == StatusRunning {
		return status, IsAlreadyRunning
	}

	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	_ = writePIDFile(g, cmd.Process.Pid)

	go func() {
		_ = cmd.Wait()
		_ = os.Remove(pidFilePath(g))
	}()

	return processStatusFromCmd(cmd, "game started in background"), nil
}

func (r *ProtonRunner) IsRunning(g *game.Game) (*ProcessStatus, error) {
	if g == nil {
		return nil, IsNotRunning
	}

	return alreadyRunningByPIDFile(g)
}

func (r *ProtonRunner) Stop(p *ProcessStatus) {
	if p == nil || p.PID <= 0 {
		return
	}

	proc, err := os.FindProcess(p.PID)
	if err != nil {
		return
	}

	_ = proc.Signal(syscall.SIGTERM)
	p.Status = StatusStopped
	p.Message = "stopped"
}

func (r *ProtonRunner) command(g *game.Game) (*exec.Cmd, error) {
	if err := validateGame(g); err != nil {
		return nil, err
	}

	proton := util.FirstNonEmpty(g.RunnerPath)
	if proton == "" {
		return nil, errors.New("proton runner path is required")
	}

	prefix := g.PrefixPath
	if prefix == "" {
		return nil, errors.New("prefix path is required for proton")
	}

	if err := os.MkdirAll(prefix, 0o755); err != nil {
		return nil, err
	}

	cmd := exec.Command(proton, "run", windowsPathForWine(g))
	cmd.Dir = workingDir(g)
	cmd.Env = baseEnv(g)
	cmd.Env = append(cmd.Env, wineArchitectureEnvForNewPrefix(g)...)

	cmd.Env = append(cmd.Env,
		"STEAM_COMPAT_DATA_PATH="+prefix,
		"STEAM_COMPAT_CLIENT_INSTALL_PATH="+util.FirstNonEmpty(
			r.SteamRoot,
			filepath.Join(os.Getenv("HOME"), ".steam", "steam"),
		),
		"WINEDLLOVERRIDES=winemenubuilder.exe=d",
	)

	return cmd, nil
}
