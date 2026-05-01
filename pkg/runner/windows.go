package runner

import (
	"path/filepath"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/game"
)

func windowsPathForWine(g *game.Game) string {
	exe := executablePath(g)

	if strings.TrimSpace(g.PrefixPath) == "" {
		return exe
	}

	driveC := driveCPath(g)

	rel, err := filepath.Rel(driveC, exe)
	if err != nil {
		return exe
	}

	if strings.HasPrefix(rel, "..") {
		return exe
	}

	return `C:\` + strings.ReplaceAll(filepath.ToSlash(rel), "/", `\`)
}

func driveCPath(g *game.Game) string {
	switch g.Runner {
	case game.RunnerProton:
		return filepath.Join(g.PrefixPath, "pfx", "drive_c")
	default:
		return filepath.Join(g.PrefixPath, "drive_c")
	}
}
