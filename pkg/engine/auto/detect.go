package auto

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Engine string

const (
	EngineUnknown     Engine = "unknown"
	EngineKiriKiri    Engine = "kirikiri/kag"
	EngineTyrano      Engine = "tyranoscript"
	EngineRPGMakerMV  Engine = "rpg-maker-mv-mz"
	EngineRPGMakerOld Engine = "rpg-maker-xp-vx-ace"
	EngineRenPy       Engine = "renpy"
	EngineUnity       Engine = "unity"
	EngineGodot       Engine = "godot"
	EngineGameMaker   Engine = "gamemaker"
	EngineNWJS        Engine = "nwjs/electron-like"
	EngineWolfRPG     Engine = "wolf-rpg"
)

type EngineDetection struct {
	Engine     Engine   `json:"engine"`
	Confidence int      `json:"confidence"` // 0-100
	Reasons    []string `json:"reasons"`
	Files      []string `json:"files"`
	StringHits []string `json:"string_hits"`
	FileOutput string   `json:"file_output,omitempty"`
}

type signature struct {
	Engine Engine
	Score  int
	Reason string
}

func DetectEngineFromExe(ctx context.Context, exePath string) (*EngineDetection, error) {
	exePath = expandHome(exePath)

	info, err := os.Stat(exePath)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("expected exe file, got directory: %s", exePath)
	}

	dir := filepath.Dir(exePath)

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	d := &EngineDetection{
		Engine: EngineUnknown,
	}

	if out, err := runCmd(ctx, "file", exePath); err == nil {
		d.FileOutput = strings.TrimSpace(out)
	} else {
		d.Reasons = append(d.Reasons, "could not run file: "+err.Error())
	}

	files, err := collectNearbyFiles(dir, 4)
	if err != nil {
		return nil, err
	}

	d.Files = interestingFiles(dir, files)

	stringHits := exeStringHits(ctx, exePath)
	d.StringHits = stringHits

	scores := map[Engine]int{}
	reasons := map[Engine][]string{}

	add := func(e Engine, score int, reason string) {
		scores[e] += score
		reasons[e] = append(reasons[e], reason)
	}

	// Check nearby files first. These are usually the strongest signals.
	for _, abs := range files {
		rel := slashRel(dir, abs)
		lower := strings.ToLower(rel)
		base := strings.ToLower(filepath.Base(rel))

		switch {
		// KiriKiri / KAG
		case base == "data.xp3":
			add(EngineKiriKiri, 35, "found data.xp3")
		case strings.HasSuffix(lower, ".xp3"):
			add(EngineKiriKiri, 25, "found XP3 archive: "+rel)
		case base == "startup.tjs":
			add(EngineKiriKiri, 35, "found startup.tjs")
		case strings.HasSuffix(lower, ".tjs"):
			add(EngineKiriKiri, 16, "found TJS script: "+rel)
		case strings.HasSuffix(lower, ".ks"):
			add(EngineKiriKiri, 10, "found KAG/Tyrano scenario script: "+rel)

		// TyranoScript
		case strings.Contains(lower, "tyrano"):
			add(EngineTyrano, 35, "found Tyrano path/file: "+rel)
		case strings.Contains(lower, "data/scenario") && strings.HasSuffix(lower, ".ks"):
			add(EngineTyrano, 30, "found Tyrano-style scenario file: "+rel)

		// RPG Maker MV/MZ
		case lower == "www/index.html":
			add(EngineRPGMakerMV, 35, "found www/index.html")
		case lower == "www/js/rpg_core.js":
			add(EngineRPGMakerMV, 45, "found RPG Maker MV rpg_core.js")
		case lower == "www/js/rmmz_core.js":
			add(EngineRPGMakerMV, 50, "found RPG Maker MZ rmmz_core.js")
		case base == "nw.pak":
			add(EngineNWJS, 20, "found nw.pak")
			add(EngineRPGMakerMV, 10, "found nw.pak, common in RPG Maker MV/MZ")
		case base == "package.json":
			add(EngineNWJS, 15, "found package.json")

		// RPG Maker old
		case strings.HasPrefix(base, "rgss") && strings.HasSuffix(base, ".dll"):
			add(EngineRPGMakerOld, 45, "found RGSS DLL: "+rel)
		case strings.HasSuffix(lower, ".rxdata"):
			add(EngineRPGMakerOld, 35, "found RPG Maker XP .rxdata: "+rel)
		case strings.HasSuffix(lower, ".rvdata") || strings.HasSuffix(lower, ".rvdata2"):
			add(EngineRPGMakerOld, 35, "found RPG Maker VX/Ace data: "+rel)

		// Ren'Py
		case strings.HasPrefix(lower, "renpy/"):
			add(EngineRenPy, 40, "found renpy directory")
		case strings.HasPrefix(lower, "game/") && strings.HasSuffix(lower, ".rpy"):
			add(EngineRenPy, 45, "found Ren'Py source script: "+rel)
		case strings.HasPrefix(lower, "game/") && strings.HasSuffix(lower, ".rpyc"):
			add(EngineRenPy, 35, "found Ren'Py compiled script: "+rel)

		// Unity
		case base == "unityplayer.dll":
			add(EngineUnity, 55, "found UnityPlayer.dll")
		case base == "gameassembly.dll":
			add(EngineUnity, 50, "found GameAssembly.dll")
		case strings.Contains(lower, "_data/globalgamemanagers"):
			add(EngineUnity, 55, "found Unity globalgamemanagers")

		// Godot
		case strings.HasSuffix(lower, ".pck"):
			add(EngineGodot, 45, "found Godot .pck: "+rel)
		case base == "data.pck":
			add(EngineGodot, 55, "found data.pck")

		// GameMaker
		case base == "data.win":
			add(EngineGameMaker, 60, "found GameMaker data.win")

		// Wolf RPG
		case strings.Contains(lower, "wolf") || base == "game.dat":
			add(EngineWolfRPG, 20, "found possible Wolf RPG file/path: "+rel)
		}
	}

	// Check strings inside exe.
	for _, hit := range stringHits {
		lower := strings.ToLower(hit)

		switch {
		case strings.Contains(lower, "kirikiri") ||
			strings.Contains(lower, "krkr") ||
			strings.Contains(lower, "tvp") ||
			strings.Contains(lower, ".xp3") ||
			strings.Contains(lower, ".tjs"):
			add(EngineKiriKiri, 25, "exe string hit: "+hit)

		case strings.Contains(lower, "tyrano"):
			add(EngineTyrano, 30, "exe string hit: "+hit)

		case strings.Contains(lower, "rpg maker") ||
			strings.Contains(lower, "rpg_core") ||
			strings.Contains(lower, "rmmz_core"):
			add(EngineRPGMakerMV, 30, "exe string hit: "+hit)

		case strings.Contains(lower, "rgss"):
			add(EngineRPGMakerOld, 35, "exe string hit: "+hit)

		case strings.Contains(lower, "renpy") || strings.Contains(lower, "ren'py"):
			add(EngineRenPy, 35, "exe string hit: "+hit)

		case strings.Contains(lower, "unityplayer") ||
			strings.Contains(lower, "gameassembly") ||
			strings.Contains(lower, "mono"):
			add(EngineUnity, 35, "exe string hit: "+hit)

		case strings.Contains(lower, "godot"):
			add(EngineGodot, 35, "exe string hit: "+hit)

		case strings.Contains(lower, "data.win"):
			add(EngineGameMaker, 35, "exe string hit: "+hit)

		case strings.Contains(lower, "nw.js") ||
			strings.Contains(lower, "node.dll") ||
			strings.Contains(lower, "chromium"):
			add(EngineNWJS, 25, "exe string hit: "+hit)
		}
	}

	bestEngine := EngineUnknown
	bestScore := 0
	for e, score := range scores {
		if score > bestScore {
			bestEngine = e
			bestScore = score
		}
	}

	d.Engine = bestEngine
	d.Confidence = clamp(bestScore, 0, 100)

	if bestEngine != EngineUnknown {
		d.Reasons = append(d.Reasons, reasons[bestEngine]...)
	}

	sort.Strings(d.Reasons)
	sort.Strings(d.Files)
	sort.Strings(d.StringHits)

	return d, nil
}

func collectNearbyFiles(root string, maxDepth int) ([]string, error) {
	var out []string

	root = filepath.Clean(root)
	rootDepth := len(strings.Split(root, string(os.PathSeparator)))

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if path == root {
			return nil
		}

		depth := len(strings.Split(filepath.Clean(path), string(os.PathSeparator))) - rootDepth
		if entry.IsDir() {
			name := strings.ToLower(entry.Name())

			if depth > maxDepth {
				return filepath.SkipDir
			}

			// Skip noisy dirs.
			switch name {
			case ".git", "node_modules", "__macosx":
				return filepath.SkipDir
			}

			return nil
		}

		if depth <= maxDepth {
			out = append(out, path)
		}

		return nil
	})

	return out, err
}

func interestingFiles(root string, files []string) []string {
	var out []string

	for _, abs := range files {
		rel := slashRel(root, abs)
		lower := strings.ToLower(rel)
		base := strings.ToLower(filepath.Base(rel))

		if strings.HasSuffix(lower, ".xp3") ||
			strings.HasSuffix(lower, ".tjs") ||
			strings.HasSuffix(lower, ".ks") ||
			strings.HasSuffix(lower, ".rpy") ||
			strings.HasSuffix(lower, ".rpyc") ||
			strings.HasSuffix(lower, ".rxdata") ||
			strings.HasSuffix(lower, ".rvdata") ||
			strings.HasSuffix(lower, ".rvdata2") ||
			strings.HasSuffix(lower, ".pck") ||
			base == "data.win" ||
			base == "package.json" ||
			base == "nw.pak" ||
			base == "node.dll" ||
			base == "unityplayer.dll" ||
			base == "gameassembly.dll" ||
			base == "startup.tjs" ||
			strings.Contains(lower, "globalgamemanagers") ||
			strings.Contains(lower, "rpg_core.js") ||
			strings.Contains(lower, "rmmz_core.js") ||
			strings.Contains(lower, "tyrano") {
			out = append(out, rel)
		}
	}

	return out
}

func exeStringHits(ctx context.Context, exePath string) []string {
	patterns := []string{
		"kirikiri",
		"krkr",
		"kag",
		"tvp",
		"rpg maker",
		"rpg_core",
		"rmmz_core",
		"rgss",
		"nw.js",
		"chromium",
		"electron",
		"unity",
		"UnityPlayer",
		"GameAssembly",
		"mono",
		"godot",
		"renpy",
		"ren'py",
		"tyrano",
		"wolf",
		"data.win",
		".xp3",
		".tjs",
		".ks",
		"node.dll",
	}

	out, err := runCmd(ctx, "strings", "-a", exePath)
	if err != nil {
		return nil
	}

	var hits []string
	seen := map[string]bool{}

	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)

		if trimmed == "" {
			continue
		}

		for _, p := range patterns {
			if strings.Contains(lower, strings.ToLower(p)) {
				if !seen[trimmed] {
					hits = append(hits, trimmed)
					seen[trimmed] = true
				}
				break
			}
		}

		if len(hits) >= 100 {
			break
		}
	}

	return hits
}

func runCmd(ctx context.Context, name string, args ...string) (string, error) {
	if _, err := exec.LookPath(name); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, name, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}

	return stdout.String(), nil
}

func slashRel(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}

	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}

	return path
}
