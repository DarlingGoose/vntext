// pkg/engines/kirikiri2/exe.go
package kirikiri2

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/util"
)

type ExecutableKind string

const (
	ExecutableGame      ExecutableKind = "game"
	ExecutableInstaller ExecutableKind = "installer"
	ExecutableRuntime   ExecutableKind = "runtime"
)

type ExecutableCandidate struct {
	Path   string
	Kind   ExecutableKind
	Reason string
	Score  int
}

func classifyExecutable(path string) ExecutableCandidate {
	name := strings.ToLower(filepath.Base(path))
	dir := strings.ToLower(filepath.ToSlash(filepath.Dir(path)))

	c := ExecutableCandidate{
		Path:  path,
		Kind:  ExecutableGame,
		Score: 100,
	}

	switch {
	case strings.Contains(dir, "/codec"):
		c.Kind = ExecutableRuntime
		c.Reason = "inside codec directory"
		c.Score = -100

	case strings.Contains(name, "codec") ||
		strings.Contains(name, "wmp") ||
		strings.Contains(name, "wm9") ||
		strings.Contains(name, "wmv") ||
		strings.Contains(name, "wmfdist"):
		c.Kind = ExecutableRuntime
		c.Reason = "looks like media/runtime installer"
		c.Score = -100

	case name == "setup.exe" ||
		name == "install.exe" ||
		name == "installer.exe":
		c.Kind = ExecutableInstaller
		c.Reason = "looks like installer executable"
		c.Score = -100

	case name == "instmsia.exe" ||
		name == "instmsiw.exe" ||
		name == "dxsetup.exe" ||
		strings.HasPrefix(name, "vcredist"):
		c.Kind = ExecutableRuntime
		c.Reason = "looks like dependency/runtime installer"
		c.Score = -100

	case strings.HasPrefix(name, "unins"):
		c.Kind = ExecutableInstaller
		c.Reason = "looks like uninstaller"
		c.Score = -100

	case name == "krkr.exe" ||
		name == "kirikiri.exe" ||
		name == "tvp.exe":
		c.Score = 200

	case strings.Contains(name, "config"):
		c.Score -= 40
	}

	return c
}

func resolveKiriKiriExecutable(resolvedPath string, info os.FileInfo) (string, string, error) {
	if !info.IsDir() {
		if !util.IsExeFile(resolvedPath) {
			return "", "", fmt.Errorf("path must be a directory or .exe file: %s", resolvedPath)
		}

		c := classifyExecutable(resolvedPath)
		if c.Kind != ExecutableGame {
			return "", "", fmt.Errorf("%s does not look like the game executable: %s", resolvedPath, c.Reason)
		}

		return resolvedPath, filepath.Dir(resolvedPath), nil
	}

	var candidates []ExecutableCandidate

	err := filepath.WalkDir(resolvedPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := strings.ToLower(d.Name())
			if name == ".git" || name == "codec" {
				if name == "codec" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !util.IsExeFile(path) {
			return nil
		}

		candidates = append(candidates, classifyExecutable(path))
		return nil
	})
	if err != nil {
		return "", "", fmt.Errorf("scan executables: %w", err)
	}

	if len(candidates) == 0 {
		return "", "", fmt.Errorf("no .exe files found in %s", resolvedPath)
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].Score > candidates[j].Score
	})

	best := candidates[0]
	if best.Kind != ExecutableGame {
		return "", "", fmt.Errorf("no likely game executable found in %s", resolvedPath)
	}

	return best.Path, filepath.Dir(best.Path), nil
}
