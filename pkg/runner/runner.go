package runner

import (
	"errors"
	"os"
	"syscall"

	"github.com/DarlingGoose/vntext/pkg/game"
)

var IsAlreadyRunning = errors.New("game is already running")
var IsNotRunning = errors.New("game is not running")

type Runner interface {
	Run(game *game.Game) (*ProcessStatus, error)
	RunBackground(game *game.Game) (*ProcessStatus, error)
	IsRunning(game *game.Game) (*ProcessStatus, error)
	Stop(p *ProcessStatus)
}

type ProcessStatus struct {
	PID     int    `json:"pid"`
	Message string `json:"message"`
	Status  int    `json:"status"`
}

const (
	StatusUnknown = iota
	StatusRunning
	StatusExited
	StatusStopped
)

type AutoRunner struct {
	Native *NativeRunner
	Wine   *WineRunner
	Proton *ProtonRunner
}

func New() *AutoRunner {
	return &AutoRunner{
		Native: &NativeRunner{},
		Wine:   &WineRunner{},
		Proton: &ProtonRunner{},
	}
}

func (r *AutoRunner) Run(g *game.Game) (*ProcessStatus, error) {
	return r.runnerFor(g).Run(g)
}

func (r *AutoRunner) RunBackground(g *game.Game) (*ProcessStatus, error) {
	return r.runnerFor(g).RunBackground(g)
}

func (r *AutoRunner) IsRunning(g *game.Game) (*ProcessStatus, error) {
	return r.runnerFor(g).IsRunning(g)
}

func (r *AutoRunner) Stop(p *ProcessStatus) {
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

func (r *AutoRunner) runnerFor(g *game.Game) Runner {
	if g == nil {
		return r.Native
	}

	switch g.Runner {
	case game.RunnerWine:
		return r.Wine
	case game.RunnerProton:
		return r.Proton
	default:
		return r.Native
	}
}
