package runner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/game"
)

func FindRunningGameProcess(g *game.Game) (*ProcessStatus, error) {
	if g == nil {
		return nil, fmt.Errorf("game is nil")
	}

	executable := strings.TrimSpace(g.Executable)
	if executable == "" {
		return nil, fmt.Errorf("game executable is empty")
	}

	wantPath, err := normalizePath(executable)
	if err != nil {
		return nil, err
	}

	wantBase := strings.ToLower(filepath.Base(wantPath))
	if wantBase == "." || wantBase == "" {
		return nil, fmt.Errorf("invalid game executable: %s", executable)
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	self := os.Getpid()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 || pid == self {
			continue
		}

		if isZombieProcess(pid) {
			continue
		}

		cmdline, _ := readProcCmdline(pid)
		procExe, _ := os.Readlink(filepath.Join("/proc", entry.Name(), "exe"))

		if processMatchesGame(pid, procExe, cmdline, wantPath, wantBase, g) {
			return &ProcessStatus{
				PID:     pid,
				Message: fmt.Sprintf("game already running: %s", strings.Join(cmdline, " ")),
				Status:  StatusRunning,
			}, nil
		}
	}

	return nil, IsNotRunning
}

func processMatchesGame(
	pid int,
	procExe string,
	cmdline []string,
	wantPath string,
	wantBase string,
	g *game.Game,
) bool {
	if samePath(procExe, wantPath) {
		return true
	}

	for _, arg := range cmdline {
		if samePath(arg, wantPath) {
			return true
		}
	}

	// Wine / Proton fallback.
	//
	// A Windows game may show up as something like:
	//
	//   wine64-preloader Z:\home\n9s\Games\foo\Game.exe
	//   proton waitforexitandrun /path/to/Game.exe
	//
	// In those cases /proc/<pid>/exe may point at wine/proton instead of the game.
	for _, arg := range cmdline {
		if wineishArgMatchesExecutable(arg, wantPath, wantBase, g) {
			return true
		}
	}

	return false
}

func readProcCmdline(pid int) ([]string, error) {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return nil, err
	}

	if len(b) == 0 {
		return nil, nil
	}

	raw := strings.Split(string(b), "\x00")

	args := make([]string, 0, len(raw))
	for _, arg := range raw {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			args = append(args, arg)
		}
	}

	return args, nil
}

func isZombieProcess(pid int) bool {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}

	// /proc/<pid>/stat looks roughly like:
	// 1234 (Game.exe) S ...
	//
	// The state is after the final ")".
	s := string(b)
	idx := strings.LastIndex(s, ")")
	if idx < 0 || idx+2 >= len(s) {
		return false
	}

	state := s[idx+2]
	return state == 'Z'
}

func normalizePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("empty path")
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	eval, err := filepath.EvalSymlinks(abs)
	if err == nil {
		abs = eval
	}

	return filepath.Clean(abs), nil
}

func samePath(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)

	if a == "" || b == "" {
		return false
	}

	na, err := normalizePath(a)
	if err != nil {
		na = filepath.Clean(a)
	}

	nb, err := normalizePath(b)
	if err != nil {
		nb = filepath.Clean(b)
	}

	return na == nb
}

func wineishArgMatchesExecutable(arg string, wantPath string, wantBase string, g *game.Game) bool {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return false
	}

	normalizedArg := strings.ToLower(strings.ReplaceAll(arg, "\\", "/"))
	normalizedWant := strings.ToLower(strings.ReplaceAll(wantPath, "\\", "/"))

	if normalizedArg == normalizedWant {
		return true
	}

	if strings.Contains(normalizedArg, normalizedWant) {
		return true
	}

	// Conservative basename fallback.
	//
	// This helps with Wine paths like:
	// Z:/home/n9s/Games/foo/Game.exe
	//
	// But avoids matching every random `Game.exe` by also checking for a parent
	// directory name or configured game name when available.
	if !strings.Contains(normalizedArg, wantBase) {
		return false
	}

	parentName := strings.ToLower(filepath.Base(filepath.Dir(wantPath)))
	gameName := strings.ToLower(strings.TrimSpace(g.Name))

	if parentName != "" && parentName != "." && strings.Contains(normalizedArg, parentName) {
		return true
	}

	if gameName != "" {
		slugName := strings.NewReplacer(
			" ", "",
			"_", "",
			"-", "",
			".", "",
		).Replace(gameName)

		slugArg := strings.NewReplacer(
			" ", "",
			"_", "",
			"-", "",
			".", "",
		).Replace(normalizedArg)

		if strings.Contains(slugArg, slugName) {
			return true
		}
	}

	return false
}
