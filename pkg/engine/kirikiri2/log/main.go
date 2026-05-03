package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	exe, err := os.Executable()
	if err != nil {
		exe = "."
	}

	dir := filepath.Dir(exe)
	logPath := filepath.Join(dir, "vntext.log")

	msg := strings.Join(os.Args[1:], " ")

	// If no args were passed, try stdin.
	if strings.TrimSpace(msg) == "" {
		if b, err := os.ReadFile("/dev/stdin"); err == nil {
			msg = string(b)
		}
	}

	if strings.TrimSpace(msg) == "" {
		msg = "(empty)"
	}

	line := fmt.Sprintf("[%s]: %s\n", time.Now().Format(time.RFC3339), msg)

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		os.Exit(1)
	}
	defer f.Close()

	if _, err := f.WriteString(line); err != nil {
		os.Exit(1)
	}
}
