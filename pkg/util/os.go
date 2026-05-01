package util

import (
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
