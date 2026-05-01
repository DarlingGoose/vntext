package rpgmaker

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"unicode"

	"github.com/DarlingGoose/vntext/pkg/util"
)

func DetectGameLocale(projectRoot string) string {
	score := scoreJapaneseProject(projectRoot)

	if score >= 8 {
		return "ja_JP.UTF-8"
	}

	// Good default for most Wine/Proton games on Linux.
	return "en_US.UTF-8"
}

func scoreJapaneseProject(projectRoot string) int {
	score := 0

	roots := []string{
		projectRoot,
		filepath.Join(projectRoot, "www"),
	}

	for _, root := range roots {
		dataDir := filepath.Join(root, "data")
		if !util.IsDir(dataDir) {
			continue
		}

		// High-value RPG Maker files.
		for _, name := range []string{
			"System.json",
			"Actors.json",
			"Classes.json",
			"Skills.json",
			"Items.json",
			"Weapons.json",
			"Armors.json",
			"Enemies.json",
			"Troops.json",
			"States.json",
			"MapInfos.json",
		} {
			path := filepath.Join(dataDir, name)
			if !util.IsFile(path) {
				continue
			}

			s, err := scoreJapaneseFile(path, 512*1024)
			if err == nil {
				score += s
			}
		}

		// Maps/dialogue can be huge, so sample a few.
		matches, _ := filepath.Glob(filepath.Join(dataDir, "Map*.json"))
		sort.Strings(matches)

		for i, path := range matches {
			if i >= 5 {
				break
			}
			s, err := scoreJapaneseFile(path, 512*1024)
			if err == nil {
				score += s
			}
		}
	}

	// Path/name signal.
	for _, r := range projectRoot {
		if isJapaneseRune(r) {
			score += 2
			break
		}
	}

	return score
}

func scoreJapaneseFile(path string, maxBytes int64) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	limited := io.LimitReader(f, maxBytes)
	data, err := io.ReadAll(limited)
	if err != nil {
		return 0, err
	}

	text := string(data)

	japanese := 0
	latin := 0

	for _, r := range text {
		switch {
		case isJapaneseRune(r):
			japanese++
		case r >= 'A' && r <= 'Z':
			latin++
		case r >= 'a' && r <= 'z':
			latin++
		}
	}

	switch {
	case japanese >= 200:
		return 6, nil
	case japanese >= 50:
		return 4, nil
	case japanese >= 10:
		return 2, nil
	case japanese > 0:
		return 1, nil
	}

	// If it has lots of Latin and no Japanese, gently bias away from JP.
	if latin > 500 {
		return -1, nil
	}

	return 0, nil
}

func isJapaneseRune(r rune) bool {
	return unicode.In(r,
		unicode.Hiragana,
		unicode.Katakana,
		unicode.Han,
	)
}
