// pkg/engines/kirikiri2/installer.go
package kirikiri2

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/util"
)

type InstallerArtifact struct {
	Path  string
	Type  string // setup, msi
	Score int
}

type InstallerScanResult struct {
	Installers      []InstallerArtifact
	GameExecutables []ExecutableCandidate
}

func (r InstallerScanResult) LooksInstallerOnly() bool {
	return len(r.Installers) > 0 && len(r.GameExecutables) == 0
}

func (r InstallerScanResult) BestInstaller() (InstallerArtifact, bool) {
	if len(r.Installers) == 0 {
		return InstallerArtifact{}, false
	}

	candidates := append([]InstallerArtifact(nil), r.Installers...)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	return candidates[0], true
}

func scanInstallerArtifacts(root string) InstallerScanResult {
	var result InstallerScanResult

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		name := strings.ToLower(filepath.Base(path))
		ext := strings.ToLower(filepath.Ext(path))

		if util.IsExeFile(path) {
			c := classifyExecutable(path)
			if c.Kind == ExecutableGame {
				result.GameExecutables = append(result.GameExecutables, c)
			}
		}

		switch {
		case name == "setup.exe":
			result.Installers = append(result.Installers, InstallerArtifact{
				Path:  path,
				Type:  "setup",
				Score: 100,
			})
		case ext == ".msi":
			result.Installers = append(result.Installers, InstallerArtifact{
				Path:  path,
				Type:  "msi",
				Score: 90,
			})
		case name == "install.exe" || strings.Contains(name, "installer"):
			result.Installers = append(result.Installers, InstallerArtifact{
				Path:  path,
				Type:  "setup",
				Score: 80,
			})
		}

		return nil
	})

	return result
}

func (e *Engine) runInstaller(ctx context.Context, g *game.Game, installer InstallerArtifact) error {
	switch g.Runner {
	case game.RunnerWine:
		return e.runWineInstaller(ctx, g, installer)
	case game.RunnerProton:
		return e.runProtonInstaller(ctx, g, installer)
	default:
		return fmt.Errorf("installer flow only supports wine/proton, got %q", g.Runner)
	}
}

func (e *Engine) runWineInstaller(ctx context.Context, g *game.Game, installer InstallerArtifact) error {
	wine := util.FirstNonEmpty(g.RunnerPath, e.RunnerPath, "wine")

	args := []string{installer.Path}
	if installer.Type == "msi" {
		args = []string{"msiexec", "/i", installer.Path}
	}

	cmd := exec.CommandContext(ctx, wine, args...)
	cmd.Dir = filepath.Dir(installer.Path)
	cmd.Env = baseWineEnv(g)
	cmd.Env = append(cmd.Env, "WINEPREFIX="+g.PrefixPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

func (e *Engine) runProtonInstaller(ctx context.Context, g *game.Game, installer InstallerArtifact) error {
	proton := util.FirstNonEmpty(g.RunnerPath, e.RunnerPath)
	if proton == "" {
		return errors.New("proton path is required")
	}

	args := []string{"run", installer.Path}
	if installer.Type == "msi" {
		args = []string{"run", "msiexec", "/i", installer.Path}
	}

	cmd := exec.CommandContext(ctx, proton, args...)
	cmd.Dir = filepath.Dir(installer.Path)
	cmd.Env = baseWineEnv(g)
	cmd.Env = append(cmd.Env,
		"STEAM_COMPAT_DATA_PATH="+g.PrefixPath,
		"STEAM_COMPAT_CLIENT_INSTALL_PATH="+util.FirstNonEmpty(
			e.SteamRoot,
			filepath.Join(os.Getenv("HOME"), ".steam", "steam"),
		),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

func findInstalledKiriKiriExecutable(prefixPath string) (string, error) {
	roots := []string{
		filepath.Join(prefixPath, "pfx", "drive_c", "Program Files"),
		filepath.Join(prefixPath, "pfx", "drive_c", "Program Files (x86)"),
		filepath.Join(prefixPath, "drive_c", "Program Files"),
		filepath.Join(prefixPath, "drive_c", "Program Files (x86)"),
	}

	var candidates []ExecutableCandidate

	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !util.IsExeFile(path) {
				return nil
			}

			c := classifyExecutable(path)
			if c.Kind == ExecutableGame {
				// Prefer installed dirs that actually contain KiriKiri files.
				profile := DetectKiriKiriProfile(filepath.Dir(path))
				if profile.IsKiriKiri {
					c.Score += 200
				}
				candidates = append(candidates, c)
			}

			return nil
		})
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no installed KiriKiri executable found in prefix: %s", prefixPath)
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].Score > candidates[j].Score
	})

	return candidates[0].Path, nil
}
