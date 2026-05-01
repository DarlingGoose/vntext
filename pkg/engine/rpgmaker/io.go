package rpgmaker

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/util"
)

func resolveProjectRoot(inputPath string) (string, string, error) {
	resolvedPath, err := filepath.Abs(strings.TrimSpace(inputPath))
	if err != nil {
		return "", "", fmt.Errorf("resolve path: %w", err)
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "", "", fmt.Errorf("stat path: %w", err)
	}

	var searchRoots []string
	if info.IsDir() {
		searchRoots = append(searchRoots, resolvedPath)
	} else {
		searchRoots = append(searchRoots, filepath.Dir(resolvedPath))
	}

	// Common packed/export layouts:
	//   Game/
	//     Game.exe
	//     www/js/...
	//   Game/
	//     Game.exe
	//     js/...
	base := searchRoots[0]
	searchRoots = append(searchRoots,
		filepath.Join(base, "www"),
		filepath.Dir(base),
		filepath.Join(filepath.Dir(base), "www"),
	)

	seen := map[string]bool{}
	for _, root := range searchRoots {
		root = filepath.Clean(root)
		if root == "." || seen[root] {
			continue
		}
		seen[root] = true

		if engine, ok := detectEngine(root); ok {
			return root, engine, nil
		}
	}

	return "", "", fmt.Errorf("could not find an RPG Maker MV/MZ project under %s", resolvedPath)
}

func findExecutableForInput(resolvedPath string, info os.FileInfo, projectRoot string) (string, error) {
	if !info.IsDir() {
		if !util.IsExeFile(resolvedPath) {
			return "", fmt.Errorf("path must be a directory or .exe file: %s", resolvedPath)
		}
		return resolvedPath, nil
	}

	return findExecutable(projectRoot)
}

func findExecutable(projectRoot string) (string, error) {
	candidates := []string{
		filepath.Join(projectRoot, "Game.exe"),
		filepath.Join(filepath.Dir(projectRoot), "Game.exe"),
	}

	for _, candidate := range candidates {
		if util.IsFile(candidate) {
			return candidate, nil
		}
	}

	var found []string
	searchRoot := projectRoot
	if filepath.Base(projectRoot) == "www" {
		searchRoot = filepath.Dir(projectRoot)
	}

	err := filepath.WalkDir(searchRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := strings.ToLower(d.Name())
			if base == "www" || base == "js" || base == "img" || base == "audio" || base == "movies" {
				return nil
			}
			if strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.EqualFold(filepath.Ext(path), ".exe") {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("scan for executable: %w", err)
	}

	sort.Slice(found, func(i, j int) bool {
		ai := exeScore(found[i])
		aj := exeScore(found[j])
		if ai == aj {
			return strings.ToLower(found[i]) < strings.ToLower(found[j])
		}
		return ai > aj
	})

	if len(found) == 0 {
		return "", fmt.Errorf("could not find RPG Maker executable near %s", projectRoot)
	}

	return found[0], nil
}

func findFirstExisting(root string, rels ...string) string {
	for _, rel := range rels {
		path := filepath.Join(root, rel)
		if util.IsFile(path) {
			return path
		}
	}
	return ""
}

func findLikelyImage(projectRoot string) string {
	searchRoots := []string{
		filepath.Join(projectRoot, "img", "titles1"),
		filepath.Join(projectRoot, "img", "titles2"),
		filepath.Join(projectRoot, "www", "img", "titles1"),
		filepath.Join(projectRoot, "www", "img", "titles2"),
		filepath.Join(projectRoot, "img", "pictures"),
		filepath.Join(projectRoot, "www", "img", "pictures"),
	}

	for _, root := range searchRoots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp" {
				return filepath.Join(root, entry.Name())
			}
		}
	}

	return ""
}
