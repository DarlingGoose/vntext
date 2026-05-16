package engine

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed log/main.go
var logMainGo []byte

var logMainGoMod []byte = []byte(
	`module vntextlog

go 1.26.2
`)

func InstallLogExe(ctx context.Context, gameRoot string) error {
	slog.Info("installing log.exe", "dir", gameRoot)
	outPath := filepath.Join(gameRoot, "log.exe")

	// Already exists: leave it alone.
	//if util.IsFile(outPath) { // re install
	//	return nil
	//}
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
		"-trimpath",
		"-ldflags",
		"-H=windowsgui -s -w",
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
