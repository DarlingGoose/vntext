// pkg/engines/kirikiri2/stage.go
package kirikiri2

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/util"
)

func stageGameIntoPrefix(g *game.Game) error {
	if strings.TrimSpace(g.PrefixPath) == "" {
		return fmt.Errorf("prefix path is required for staging")
	}
	if strings.TrimSpace(g.Executable) == "" {
		return fmt.Errorf("executable path is required for staging")
	}

	sourceDir := g.WorkingDir
	if strings.TrimSpace(sourceDir) == "" {
		sourceDir = filepath.Dir(g.Executable)
	}

	stagedDir := filepath.Join(driveCPath(g), "Games", util.SanitizeName(g.Name))

	if err := os.RemoveAll(stagedDir); err != nil {
		return fmt.Errorf("remove old staged game: %w", err)
	}
	if err := copyDir(sourceDir, stagedDir); err != nil {
		return fmt.Errorf("copy game into prefix: %w", err)
	}

	stagedExe := filepath.Join(stagedDir, filepath.Base(g.Executable))
	if _, err := os.Stat(stagedExe); err != nil {
		return fmt.Errorf("staged executable missing: %s: %w", stagedExe, err)
	}

	g.StagedPath = stagedDir
	g.GamePath = stagedDir
	g.Executable = stagedExe
	g.WorkingDir = stagedDir
	g.StageToPrefix = true

	return nil
}

func driveCPath(g *game.Game) string {
	switch g.Runner {
	case game.RunnerProton:
		return filepath.Join(g.PrefixPath, "pfx", "drive_c")
	default:
		return filepath.Join(g.PrefixPath, "drive_c")
	}
}

func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("source is not a directory: %s", src)
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()

	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
