// pkg/engines/kirikiri2/detect.go
package kirikiri2

import (
	"os"
	"path/filepath"
	"strings"
)

type KiriKiriProfile struct {
	IsKiriKiri      bool
	HasTJS          bool
	HasXP3          bool
	HasStartupTJS   bool
	HasDataXP3      bool
	HasMojibakeName bool
	HasJapaneseName bool
	Reason          string
}

func DetectKiriKiriProfile(root string) KiriKiriProfile {
	var p KiriKiriProfile

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		name := d.Name()
		lower := strings.ToLower(name)

		if d.IsDir() {
			switch lower {
			case ".git", "node_modules", "__macosx":
				return filepath.SkipDir
			}
			return nil
		}

		switch filepath.Ext(lower) {
		case ".tjs":
			p.HasTJS = true
		case ".xp3":
			p.HasXP3 = true
		}

		switch lower {
		case "startup.tjs":
			p.HasStartupTJS = true
		case "data.xp3", "patch.xp3", "scenario.xp3", "system.xp3":
			p.HasDataXP3 = true
		}

		if looksLikeMojibake(name) {
			p.HasMojibakeName = true
		}
		if containsJapanese(name) {
			p.HasJapaneseName = true
		}

		return nil
	})

	var reasons []string
	if p.HasStartupTJS {
		reasons = append(reasons, "startup.tjs detected")
	}
	if p.HasTJS {
		reasons = append(reasons, "TJS scripts detected")
	}
	if p.HasDataXP3 {
		reasons = append(reasons, "known KiriKiri XP3 archive detected")
	}
	if p.HasXP3 {
		reasons = append(reasons, "XP3 archives detected")
	}

	p.IsKiriKiri =
		p.HasStartupTJS ||
			(p.HasTJS && p.HasXP3) ||
			(p.HasDataXP3 && hasLikelyKiriKiriExe(root))

	p.Reason = strings.Join(reasons, ", ")
	return p
}

func hasLikelyKiriKiriExe(root string) bool {
	found := false

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || found {
			return nil
		}

		if !strings.EqualFold(filepath.Ext(path), ".exe") {
			return nil
		}

		name := strings.ToLower(filepath.Base(path))
		if name == "krkr.exe" ||
			name == "kirikiri.exe" ||
			name == "tvp.exe" ||
			!strings.Contains(name, "setup") && !strings.Contains(name, "install") && !strings.HasPrefix(name, "unins") {
			found = true
		}

		return nil
	})

	return found
}

func containsJapanese(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x3040 && r <= 0x309F:
			return true
		case r >= 0x30A0 && r <= 0x30FF:
			return true
		case r >= 0x4E00 && r <= 0x9FFF:
			return true
		}
	}
	return false
}

func looksLikeMojibake(s string) bool {
	return strings.Contains(s, "ƒ") ||
		strings.Contains(s, "‚") ||
		strings.Contains(s, "„") ||
		strings.Contains(s, "‰") ||
		strings.Contains(s, "Œ") ||
		strings.Contains(s, "Ž") ||
		strings.Contains(s, "Р") ||
		strings.Contains(s, "Ц")
}
