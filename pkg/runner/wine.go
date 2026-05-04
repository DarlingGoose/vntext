package runner

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/util"
)

type WineRunner struct{}

func (r *WineRunner) Run(g *game.Game) (*ProcessStatus, error) {
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

func (r *WineRunner) RunBackground(g *game.Game) (*ProcessStatus, error) {
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

func (r *WineRunner) IsRunning(g *game.Game) (*ProcessStatus, error) {
	if g == nil {
		return nil, IsNotRunning
	}

	return alreadyRunningByPIDFile(g)
}

func (r *WineRunner) Stop(p *ProcessStatus) {
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

func (r *WineRunner) command(g *game.Game) (*exec.Cmd, error) {
	if err := validateGame(g); err != nil {
		return nil, err
	}
	ensureJapaneseFontsForGame(g)

	wine := util.FirstNonEmpty(g.RunnerPath, "wine")

	cmd := exec.Command(wine, wineDesktopArgsForGame(g)...)
	cmd.Dir = workingDir(g)
	cmd.Env = baseEnv(g)

	if g.PrefixPath != "" {
		cmd.Env = append(cmd.Env, "WINEPREFIX="+g.PrefixPath)
	}
	cmd.Env = append(cmd.Env, wineArchitectureEnvForNewPrefix(g)...)

	cmd.Env = append(cmd.Env, "WINEDLLOVERRIDES=winemenubuilder.exe=d")

	return cmd, nil
}
