package runner

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/game"
)

const japaneseFontDirsEnv = "VNTEXT_JAPANESE_FONT_DIRS"

func ensureJapaneseFontsForGame(g *game.Game) {
	if g == nil || !isJapaneseLocale(g.Locale) || strings.TrimSpace(g.PrefixPath) == "" {
		return
	}

	_ = provisionJapaneseFonts(driveCPath(g), japaneseFontSearchDirs())
}

func isJapaneseLocale(locale string) bool {
	locale = strings.ToLower(strings.TrimSpace(locale))
	return locale == "ja" || strings.HasPrefix(locale, "ja_") || strings.HasPrefix(locale, "ja.")
}

func provisionJapaneseFonts(driveC string, searchDirs []string) error {
	fontsDir := filepath.Join(driveC, "windows", "Fonts")
	if err := os.MkdirAll(fontsDir, 0o755); err != nil {
		return err
	}

	fonts := findJapaneseFonts(searchDirs)
	for _, source := range fonts {
		target := filepath.Join(fontsDir, filepath.Base(source))
		if _, err := os.Stat(target); err == nil {
			continue
		}

		if err := os.Symlink(source, target); err == nil {
			continue
		}

		_ = copyFontFile(source, target)
	}

	return nil
}

func japaneseFontSearchDirs() []string {
	if raw := strings.TrimSpace(os.Getenv(japaneseFontDirsEnv)); raw != "" {
		return filepath.SplitList(raw)
	}

	dirs := []string{
		"/usr/share/fonts",
		"/usr/local/share/fonts",
	}

	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, ".local", "share", "fonts"),
			filepath.Join(home, ".fonts"),
		)
	}

	return dirs
}

func findJapaneseFonts(searchDirs []string) []string {
	var fonts []string
	seen := make(map[string]bool)

	for _, root := range searchDirs {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}

		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}

			if !isJapaneseFontFile(path) {
				return nil
			}

			if !seen[path] {
				seen[path] = true
				fonts = append(fonts, path)
			}
			return nil
		})
	}

	return fonts
}

func isJapaneseFontFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".ttf" && ext != ".ttc" && ext != ".otf" {
		return false
	}

	name := strings.ToLower(filepath.Base(path))
	markers := []string{
		"notosanscjk",
		"notoserifcjk",
		"sourcehansans",
		"sourcehanserif",
		"ipag",
		"ipam",
		"ipaex",
		"vlgothic",
		"vl-gothic",
		"takao",
		"migmix",
		"mplus",
		"msgothic",
		"msmincho",
		"yugothic",
		"yumincho",
	}

	for _, marker := range markers {
		if strings.Contains(name, marker) {
			return true
		}
	}

	return false
}

func copyFontFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(target)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(target)
		return closeErr
	}

	return nil
}
