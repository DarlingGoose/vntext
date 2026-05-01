package runner

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/DarlingGoose/vntext/pkg/game"
)

type NativeRunner struct{}

func (r *NativeRunner) Run(g *game.Game) (*ProcessStatus, error) {
	if err := validateGame(g); err != nil {
		return nil, err
	}

	if status, err := r.IsRunning(g); err == nil && status.Status == StatusRunning {
		return status, IsAlreadyRunning
	}

	cmd := exec.Command(executablePath(g))
	cmd.Dir = workingDir(g)
	cmd.Env = baseEnv(g)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	_ = writePIDFile(g, cmd.Process.Pid)

	err := cmd.Wait()
	status := processStatusFromCmd(cmd, "game exited")
	status.Status = StatusExited
	_ = os.Remove(pidFilePath(g))

	return status, err
}

func (r *NativeRunner) RunBackground(g *game.Game) (*ProcessStatus, error) {
	if err := validateGame(g); err != nil {
		return nil, err
	}

	if status, err := r.IsRunning(g); err == nil && status.Status == StatusRunning {
		return status, IsAlreadyRunning
	}

	cmd := exec.Command(executablePath(g))
	cmd.Dir = workingDir(g)
	cmd.Env = baseEnv(g)
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

func (r *NativeRunner) IsRunning(g *game.Game) (*ProcessStatus, error) {
	if g == nil {
		return nil, IsNotRunning
	}

	return alreadyRunningByPIDFile(g)
}

func (r *NativeRunner) Stop(p *ProcessStatus) {
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
