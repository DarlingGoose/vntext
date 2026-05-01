// pkg/engines/kirikiri2/log_exe.go
package kirikiri2

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/DarlingGoose/vntext/pkg/util"
)

//go:embed log/main.go
var logMainGo []byte

var logMainGoMod []byte = []byte(
	`module vntextlog

go 1.26.2
`)

func (e *Engine) installLogExe(ctx context.Context, gameRoot string) error {
	outPath := filepath.Join(gameRoot, "log.exe")

	// Already exists: leave it alone.
	if util.IsFile(outPath) {
		return nil
	}
	logDir, err := os.MkdirTemp("", "log")
	if err != nil {
		return err
	}
	println(logDir)
	err = os.WriteFile(filepath.Join(logDir, "main.go"), logMainGo, 0777)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(logDir, "go.mod"), logMainGoMod, 0777)
	if err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp("", "wgl-krkr-logexe-*")
	if err != nil {
		return fmt.Errorf("create temp log build dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpOut := filepath.Join(tmpDir, "log.exe")

	cmd := exec.CommandContext(
		ctx,
		"go",
		"build",
		`-ldflags=-H=windowsgui`,
		"-o",
		tmpOut,
		".",
	)
	cmd.Dir = logDir
	cmd.Env = append(os.Environ(),
		"GOOS=windows",
		"GOARCH=386",
		"CGO_ENABLED=0",
	)

	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build log.exe: %w\n%s", err, stderr.String())
	}

	if err := copyFile(tmpOut, outPath, 0o755); err != nil {
		return fmt.Errorf("copy log.exe into game root: %w", err)
	}

	return nil
}
