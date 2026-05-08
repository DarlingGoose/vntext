package util

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const configDirName = "wgl"

func ConfigBaseDir() string {
	homeDir, err := os.UserCacheDir()
	if err != nil {
		return configDirName
	}
	return filepath.Join(homeDir, configDirName)
}

func IsExeFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".exe")
}

func IsImageFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".bmp", ".gif", ".ico", ".svg":
		return true
	default:
		return false
	}
}

func IsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func IsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func FindExe(path string) (string, error) {
	if IsExeFile(path) {
		if IsFile(path) {
			return path, nil
		}
		return "", os.ErrNotExist
	}
	if !IsDir(path) {
		return "", os.ErrNotExist
	}
	var exeFiles []string
	root := filepath.Clean(path)
	err := filepath.Walk(path, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if IsExeFile(info.Name()) {
			exeFiles = append(exeFiles, path)
		}
		if info.IsDir() && filepath.Clean(path) != root {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(exeFiles) == 1 {
		return exeFiles[0], nil
	}
	if len(exeFiles) == 0 {
		return "", os.ErrNotExist
	}
	return "", errors.New("found multiple exe files in dir")
}

func ScoreIconCandidate(searchRoot, path string) int {
	score := scoreSharedAssetCandidate(searchRoot, path)
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))

	switch {
	case strings.Contains(name, "icon"):
		score += 120
	case strings.Contains(name, "logo"):
		score += 100
	case strings.Contains(name, "favicon"):
		score += 90
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".ico":
		score += 60
	case ".png":
		score += 25
	case ".svg":
		score += 20
	}

	if strings.Contains(name, "banner") || strings.Contains(name, "cover") || strings.Contains(name, "screenshot") {
		score -= 40
	}
	return score
}

func ScoreImageCandidate(searchRoot, path string) int {
	score := scoreSharedAssetCandidate(searchRoot, path)
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))

	switch {
	case strings.Contains(name, "cover"):
		score += 120
	case strings.Contains(name, "poster"):
		score += 100
	case strings.Contains(name, "banner"):
		score += 90
	case strings.Contains(name, "hero"):
		score += 80
	case strings.Contains(name, "art"):
		score += 60
	case strings.Contains(name, "image"):
		score += 40
	}

	if strings.Contains(name, "icon") || strings.Contains(name, "favicon") {
		score -= 35
	}
	if strings.Contains(name, "screenshot") || strings.Contains(name, "thumb") {
		score -= 20
	}
	return score
}

func scoreSharedAssetCandidate(searchRoot, path string) int {
	relativePath, err := filepath.Rel(searchRoot, path)
	if err != nil {
		relativePath = path
	}

	score := 10
	depth := strings.Count(filepath.Clean(relativePath), string(os.PathSeparator))
	score -= depth * 5

	lowerPath := strings.ToLower(relativePath)
	for _, token := range []string{"assets", "images", "img", "artwork", "media"} {
		if strings.Contains(lowerPath, token) {
			score += 15
			break
		}
	}

	return score
}

func FindFileAndRead(startDir, partialPath string) (absPath string, data []byte, err error) {
	if strings.TrimSpace(startDir) == "" {
		return "", nil, fmt.Errorf("startDir is required")
	}
	if strings.TrimSpace(partialPath) == "" {
		return "", nil, fmt.Errorf("partialPath is required")
	}

	startAbs, err := filepath.Abs(startDir)
	if err != nil {
		return "", nil, fmt.Errorf("resolve start dir: %w", err)
	}

	info, err := os.Stat(startAbs)
	if err != nil {
		return "", nil, fmt.Errorf("stat start dir: %w", err)
	}
	if !info.IsDir() {
		return "", nil, fmt.Errorf("startDir is not a directory: %s", startAbs)
	}

	partialClean := normalizePathForMatch(partialPath)

	var found string

	err = filepath.WalkDir(startAbs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Skip dirs/files we cannot read instead of failing the whole search.
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		abs, err := filepath.Abs(path)
		if err != nil {
			return nil
		}

		rel, err := filepath.Rel(startAbs, abs)
		if err != nil {
			return nil
		}

		relNorm := normalizePathForMatch(rel)
		absNorm := normalizePathForMatch(abs)

		if strings.Contains(relNorm, partialClean) || strings.Contains(absNorm, partialClean) {
			found = abs
			return filepath.SkipAll
		}

		return nil
	})
	if err != nil {
		return "", nil, fmt.Errorf("walk dir: %w", err)
	}

	if found == "" {
		return "", nil, fmt.Errorf("file matching %q not found under %s", partialPath, startAbs)
	}

	data, err = os.ReadFile(found)
	if err != nil {
		return "", nil, fmt.Errorf("read file %s: %w", found, err)
	}

	return found, data, nil
}

func normalizePathForMatch(p string) string {
	p = filepath.Clean(p)
	p = filepath.ToSlash(p)
	p = strings.TrimPrefix(p, "./")
	return strings.ToLower(p)
}
