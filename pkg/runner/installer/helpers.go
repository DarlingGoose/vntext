package installer

import (
	"os"
	"path/filepath"
	"strings"
)

func expandHome(path string) string {
	path = strings.TrimSpace(path)

	if path == "" {
		return ""
	}

	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
		return path
	}

	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}

	return path
}

func fileExists(path string) bool {
	path = expandHome(path)
	if strings.TrimSpace(path) == "" {
		return false
	}

	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	path = expandHome(path)
	if strings.TrimSpace(path) == "" {
		return false
	}

	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func steamInstallPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	candidates := []string{
		filepath.Join(home, ".local", "share", "Steam"),
		filepath.Join(home, ".steam", "steam"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam"),
	}

	for _, candidate := range candidates {
		if dirExists(candidate) {
			return candidate
		}
	}

	return filepath.Join(home, ".local", "share", "Steam")
}
