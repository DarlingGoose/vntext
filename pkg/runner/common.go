package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/util"
)

const DefaultVirtualDesktop = "1280x720"

func validateGame(g *game.Game) error {
	if g == nil {
		return errors.New("game is nil")
	}
	if strings.TrimSpace(g.Name) == "" {
		return errors.New("game name is required")
	}
	if strings.TrimSpace(g.Executable) == "" {
		return errors.New("game executable is required")
	}
	if _, err := os.Stat(executablePath(g)); err != nil {
		return fmt.Errorf("game executable is not accessible: %s: %w", executablePath(g), err)
	}
	return nil
}

func executablePath(g *game.Game) string {
	exe := strings.TrimSpace(g.Executable)
	if filepath.IsAbs(exe) {
		return exe
	}
	if strings.TrimSpace(g.GamePath) != "" {
		return filepath.Join(g.GamePath, exe)
	}
	return exe
}

func workingDir(g *game.Game) string {
	if strings.TrimSpace(g.WorkingDir) != "" {
		return g.WorkingDir
	}

	exe := executablePath(g)
	if exe != "" {
		return filepath.Dir(exe)
	}

	if strings.TrimSpace(g.GamePath) != "" {
		return g.GamePath
	}

	return "."
}

func processStatusFromCmd(cmd *exec.Cmd, msg string) *ProcessStatus {
	pid := 0
	if cmd != nil && cmd.Process != nil {
		pid = cmd.Process.Pid
	}

	return &ProcessStatus{
		PID:     pid,
		Message: msg,
		Status:  StatusRunning,
		cmd:     cmd,
	}
}

func isPIDRunning(pid int) bool {
	if pid <= 0 {
		return false
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Signal 0 checks existence without killing the process.
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func alreadyRunningByPIDFile(g *game.Game) (*ProcessStatus, error) {
	if process, _ := FindRunningGameProcess(g); process != nil {
		return process, nil
	}
	pid, ok := readPIDFile(g)
	if !ok {
		return nil, IsNotRunning
	}

	if isPIDRunning(pid) {
		return &ProcessStatus{
			PID:     pid,
			Message: "game is already running",
			Status:  StatusRunning,
		}, nil
	}

	_ = os.Remove(pidFilePath(g))
	return nil, IsNotRunning
}

func writePIDFile(g *game.Game, pid int) error {
	path := pidFilePath(g)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, []byte(fmt.Sprintf("%d\n", pid)), 0o644)
}

func readPIDFile(g *game.Game) (int, bool) {
	raw, err := os.ReadFile(pidFilePath(g))
	if err != nil {
		return 0, false
	}

	var pid int
	if _, err := fmt.Sscanf(string(raw), "%d", &pid); err != nil {
		return 0, false
	}

	return pid, pid > 0
}

func pidFilePath(g *game.Game) string {
	base := strings.TrimSpace(g.PrefixPath)
	if base == "" {
		base = strings.TrimSpace(g.GamePath)
	}
	if base == "" {
		base = os.TempDir()
	}

	name := util.SanitizeName(g.Name)
	if name == "" {
		name = "game"
	}

	return filepath.Join(base, ".vntext", name+".pid")
}

func cleanRunnerEnv(env []string) []string {
	out := make([]string, 0, len(env))

	for _, e := range env {
		switch {
		case strings.HasPrefix(e, "WINEPREFIX="):
			continue
		case strings.HasPrefix(e, "WINEARCH="):
			continue
		case strings.HasPrefix(e, "STEAM_COMPAT_DATA_PATH="):
			continue
		case strings.HasPrefix(e, "STEAM_COMPAT_CLIENT_INSTALL_PATH="):
			continue
		}

		out = append(out, e)
	}

	return out
}

func baseEnv(g *game.Game) []string {
	env := cleanRunnerEnv(os.Environ())

	if strings.TrimSpace(g.Locale) != "" {
		env = append(env,
			"LANG="+g.Locale,
			"LC_ALL="+g.Locale,
		)
	}

	for _, kv := range g.EnvVars {
		if strings.TrimSpace(kv.Key) == "" {
			continue
		}
		env = append(env, kv.Key+"="+kv.Value)
	}

	return env
}

func wineArchitectureEnvForNewPrefix(g *game.Game) []string {
	if g == nil || winePrefixInitialized(g) {
		return nil
	}

	wineArch := game.WineArchitecture(g.Architecture)
	if wineArch == "" {
		return nil
	}

	return []string{"WINEARCH=" + wineArch}
}

func winePrefixInitialized(g *game.Game) bool {
	prefix := strings.TrimSpace(g.PrefixPath)
	if prefix == "" {
		return true
	}

	switch g.Runner {
	case game.RunnerProton:
		return util.IsFile(filepath.Join(prefix, "pfx", "system.reg"))
	default:
		return util.IsFile(filepath.Join(prefix, "system.reg"))
	}
}

func wineVirtualDesktop(g *game.Game) string {
	if g == nil {
		return ""
	}

	desktop := strings.TrimSpace(g.VirtualDesktop)
	switch strings.ToLower(desktop) {
	case "off", "false", "none", "disabled", "disable", "0":
		return ""
	case "":
		return DefaultVirtualDesktop
	default:
		return desktop
	}
}

func wineDesktopArgsForGame(g *game.Game) []string {
	exe := windowsPathForWine(g)
	desktop := wineVirtualDesktop(g)
	if desktop == "" {
		return []string{exe}
	}

	return []string{"explorer", "/desktop=vntext," + desktop, exe}
}
