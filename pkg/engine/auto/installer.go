package auto

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type InstallerDetection struct {
	Name        string
	IsInstaller bool     `json:"is_installer"`
	Confidence  int      `json:"confidence"` // 0-100
	Kind        string   `json:"kind,omitempty"`
	Reasons     []string `json:"reasons,omitempty"`
	FileOutput  string   `json:"file_output,omitempty"`
	StringHits  []string `json:"string_hits,omitempty"`
}

func DetectInstallerExe(ctx context.Context, exePath string) (*InstallerDetection, error) {
	exePath = expandHome(exePath)

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	d := &InstallerDetection{}

	if out, err := runCmd(ctx, "file", exePath); err == nil {
		d.FileOutput = strings.TrimSpace(out)
		scoreInstallerText(d, d.FileOutput, "file output")
	} else {
		d.Reasons = append(d.Reasons, "could not run file: "+err.Error())
	}

	if hits := installerStringHits(ctx, exePath); len(hits) > 0 {
		d.StringHits = hits
		for _, hit := range hits {
			scoreInstallerText(d, hit, "exe string")
		}
	}

	// Filename is weak evidence, but useful.
	name := strings.ToLower(filepath.Base(exePath))
	switch {
	case strings.Contains(name, "setup"):
		addInstallerScore(d, 15, "generic setup filename", "filename contains setup")
	case strings.Contains(name, "install"):
		addInstallerScore(d, 15, "generic installer filename", "filename contains install")
	case regexp.MustCompile(`(?i)(patch|update|updater|upgrade)`).MatchString(name):
		addInstallerScore(d, 10, "patch/updater", "filename looks like patch/update installer")
	}

	if d.Confidence >= 40 {
		d.IsInstaller = true
	}

	if d.Confidence > 100 {
		d.Confidence = 100
	}

	return d, nil
}

func DetectEngineFromPath(ctx context.Context, inputPath string) (*EngineDetection, error) {
	inputPath = expandHome(inputPath)

	info, err := os.Stat(inputPath)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	d := &EngineDetection{
		Engine: EngineUnknown,
	}

	var scanRoot string
	var exePath string

	if info.IsDir() {
		scanRoot = inputPath
	} else {
		exePath = inputPath
		scanRoot = filepath.Dir(inputPath)

		if out, err := runCmd(ctx, "file", exePath); err == nil {
			d.FileOutput = strings.TrimSpace(out)
		} else {
			d.Reasons = append(d.Reasons, "could not run file: "+err.Error())
		}
	}

	files, err := collectNearbyFiles(scanRoot, 5)
	if err != nil {
		return nil, err
	}

	d.Files = interestingFiles(scanRoot, files)

	if exePath != "" {
		d.StringHits = exeStringHits(ctx, exePath)
	} else {
		mainExe := findMainExe(scanRoot, files)
		if mainExe != "" {
			d.StringHits = exeStringHits(ctx, mainExe)

			if out, err := runCmd(ctx, "file", mainExe); err == nil {
				d.FileOutput = strings.TrimSpace(out)
			}
		}
	}

	scores := map[Engine]int{}
	reasons := map[Engine][]string{}

	add := func(e Engine, score int, reason string) {
		scores[e] += score
		reasons[e] = append(reasons[e], reason)
	}

	scoreEngineFiles(scanRoot, files, add)
	scoreEngineStringHits(d.StringHits, add)

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

func findMainExe(root string, files []string) string {
	type candidate struct {
		path  string
		score int
	}

	var candidates []candidate

	for _, abs := range files {
		base := strings.ToLower(filepath.Base(abs))
		if !strings.HasSuffix(base, ".exe") {
			continue
		}

		score := 0

		switch base {
		case "game.exe":
			score += 100
		case "rpg_rt.exe":
			score += 90
		}

		if strings.Contains(base, "unins") ||
			strings.Contains(base, "uninstall") ||
			strings.Contains(base, "setup") ||
			strings.Contains(base, "install") {
			score -= 100
		}

		rel := slashRel(root, abs)
		depth := strings.Count(rel, "/")
		score -= depth * 5

		candidates = append(candidates, candidate{
			path:  abs,
			score: score,
		})
	}

	if len(candidates) == 0 {
		return ""
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	return candidates[0].path
}

func scoreEngineStringHits(hits []string, add func(Engine, int, string)) {
	for _, hit := range hits {
		lower := strings.ToLower(hit)

		switch {
		case strings.Contains(lower, "kirikiri") ||
			strings.Contains(lower, "startup.tjs") ||
			strings.Contains(lower, "data.xp3") ||
			strings.Contains(lower, "tvp(kirikiri)") ||
			strings.Contains(lower, "tvpinit"):
			add(EngineKiriKiri, 35, "exe string hit: "+hit)

		case strings.Contains(lower, "tyranoscript") ||
			strings.Contains(lower, "tyrano_data"):
			add(EngineTyrano, 35, "exe string hit: "+hit)

		case strings.Contains(lower, "rpg maker") ||
			strings.Contains(lower, "rpg_core.js") ||
			strings.Contains(lower, "rmmz_core.js"):
			add(EngineRPGMakerMV, 35, "exe string hit: "+hit)

		case strings.Contains(lower, "rgss301.dll") ||
			strings.Contains(lower, "rgss202e.dll") ||
			strings.Contains(lower, "rgss102e.dll") ||
			strings.Contains(lower, "game.rgss3a") ||
			strings.Contains(lower, "game.rgss2a") ||
			strings.Contains(lower, "game.rgssad"):
			add(EngineRPGMakerOld, 45, "exe string hit: "+hit)

		case strings.Contains(lower, "renpy") ||
			strings.Contains(lower, "ren'py"):
			add(EngineRenPy, 35, "exe string hit: "+hit)

		case strings.Contains(lower, "unityplayer.dll") ||
			strings.Contains(lower, "gameassembly.dll"):
			add(EngineUnity, 40, "exe string hit: "+hit)

		case strings.Contains(lower, "godot"):
			add(EngineGodot, 35, "exe string hit: "+hit)

		case strings.Contains(lower, "data.win"):
			add(EngineGameMaker, 35, "exe string hit: "+hit)

		case strings.Contains(lower, "nw.js") ||
			strings.Contains(lower, "node.dll"):
			add(EngineNWJS, 25, "exe string hit: "+hit)
		}
	}
}
func scoreEngineFiles(root string, files []string, add func(Engine, int, string)) {
	for _, abs := range files {
		rel := slashRel(root, abs)
		lower := strings.ToLower(rel)
		base := strings.ToLower(filepath.Base(rel))

		switch {
		// KiriKiri / KAG
		case base == "data.xp3":
			add(EngineKiriKiri, 45, "found data.xp3: "+rel)
		case strings.HasSuffix(lower, ".xp3"):
			add(EngineKiriKiri, 30, "found XP3 archive: "+rel)
		case base == "startup.tjs":
			add(EngineKiriKiri, 50, "found startup.tjs: "+rel)
		case strings.HasSuffix(lower, ".tjs"):
			add(EngineKiriKiri, 20, "found TJS script: "+rel)
		case strings.HasSuffix(lower, ".ks"):
			add(EngineKiriKiri, 8, "found .ks scenario script: "+rel)

		// TyranoScript
		case strings.Contains(lower, "tyrano"):
			add(EngineTyrano, 40, "found Tyrano path/file: "+rel)
		case strings.Contains(lower, "data/scenario") && strings.HasSuffix(lower, ".ks"):
			add(EngineTyrano, 45, "found Tyrano-style scenario file: "+rel)

		// RPG Maker MV/MZ
		case lower == "www/index.html":
			add(EngineRPGMakerMV, 35, "found www/index.html")
		case lower == "www/js/rpg_core.js":
			add(EngineRPGMakerMV, 55, "found RPG Maker MV rpg_core.js")
		case lower == "www/js/rmmz_core.js":
			add(EngineRPGMakerMV, 60, "found RPG Maker MZ rmmz_core.js")
		case base == "nw.pak":
			add(EngineNWJS, 25, "found nw.pak")
			add(EngineRPGMakerMV, 10, "found nw.pak, common in RPG Maker MV/MZ")
		case base == "node.dll":
			add(EngineNWJS, 20, "found node.dll")
		case base == "package.json":
			add(EngineNWJS, 15, "found package.json")

		// RPG Maker XP/VX/Ace
		case base == "game.rgss3a":
			add(EngineRPGMakerOld, 80, "found RPG Maker VX Ace archive Game.rgss3a")
		case base == "game.rgss2a":
			add(EngineRPGMakerOld, 75, "found RPG Maker VX archive Game.rgss2a")
		case base == "game.rgssad":
			add(EngineRPGMakerOld, 75, "found RPG Maker XP archive Game.rgssad")
		case strings.HasPrefix(base, "rgss") && strings.HasSuffix(base, ".dll"):
			add(EngineRPGMakerOld, 55, "found RGSS DLL: "+rel)
		case strings.HasSuffix(lower, ".rxdata"):
			add(EngineRPGMakerOld, 45, "found RPG Maker XP .rxdata: "+rel)
		case strings.HasSuffix(lower, ".rvdata"):
			add(EngineRPGMakerOld, 45, "found RPG Maker VX .rvdata: "+rel)
		case strings.HasSuffix(lower, ".rvdata2"):
			add(EngineRPGMakerOld, 55, "found RPG Maker VX Ace .rvdata2: "+rel)
		case base == "game.ini":
			add(EngineRPGMakerOld, 20, "found RPG Maker-style Game.ini")
		case base == "game.exe":
			add(EngineRPGMakerOld, 10, "found RPG Maker-style Game.exe")

		// Ren'Py
		case strings.HasPrefix(lower, "renpy/"):
			add(EngineRenPy, 50, "found renpy directory")
		case strings.HasPrefix(lower, "game/") && strings.HasSuffix(lower, ".rpy"):
			add(EngineRenPy, 50, "found Ren'Py source script: "+rel)
		case strings.HasPrefix(lower, "game/") && strings.HasSuffix(lower, ".rpyc"):
			add(EngineRenPy, 45, "found Ren'Py compiled script: "+rel)

		// Unity
		case base == "unityplayer.dll":
			add(EngineUnity, 60, "found UnityPlayer.dll")
		case base == "gameassembly.dll":
			add(EngineUnity, 60, "found GameAssembly.dll")
		case strings.Contains(lower, "_data/globalgamemanagers"):
			add(EngineUnity, 65, "found Unity globalgamemanagers")

		// Godot
		case base == "data.pck":
			add(EngineGodot, 60, "found data.pck")
		case strings.HasSuffix(lower, ".pck"):
			add(EngineGodot, 45, "found Godot .pck: "+rel)

		// GameMaker
		case base == "data.win":
			add(EngineGameMaker, 65, "found GameMaker data.win")

		// Wolf RPG
		case base == "game.dat" || strings.Contains(lower, "wolf"):
			add(EngineWolfRPG, 25, "found possible Wolf RPG file/path: "+rel)
		}
	}
}
func scoreInstallerText(d *InstallerDetection, text, source string) {
	lower := strings.ToLower(text)

	switch {
	case strings.Contains(lower, "installshield self-extracting archive"):
		addInstallerScore(d, 80, "installshield", source+": InstallShield self-extracting archive")

	case strings.Contains(lower, "installshield"):
		addInstallerScore(d, 55, "installshield", source+": InstallShield marker")

	case strings.Contains(lower, "inno setup"):
		addInstallerScore(d, 80, "inno setup", source+": Inno Setup marker")

	case strings.Contains(lower, "nullsoft") ||
		strings.Contains(lower, "nsis"):
		addInstallerScore(d, 75, "nsis", source+": NSIS/Nullsoft marker")

	case strings.Contains(lower, "wise installation") ||
		strings.Contains(lower, "wise installer"):
		addInstallerScore(d, 70, "wise", source+": Wise installer marker")

	case strings.Contains(lower, "sfx") ||
		strings.Contains(lower, "self-extracting archive") ||
		strings.Contains(lower, "self extracting archive"):
		addInstallerScore(d, 60, "self-extracting archive", source+": self-extracting archive marker")

	case strings.Contains(lower, "cabinet") ||
		strings.Contains(lower, "data1.cab") ||
		strings.Contains(lower, "setup.inx") ||
		strings.Contains(lower, "setup.iss"):
		addInstallerScore(d, 45, "installshield-like", source+": cabinet/setup payload marker")

	case strings.Contains(lower, "msi") ||
		strings.Contains(lower, "windows installer"):
		addInstallerScore(d, 40, "msi/windows installer", source+": MSI/Windows Installer marker")

	case strings.Contains(lower, "setup launcher") ||
		strings.Contains(lower, "setup initialization") ||
		strings.Contains(lower, "install wizard") ||
		strings.Contains(lower, "uninstall"):
		addInstallerScore(d, 30, "generic installer", source+": generic installer marker")
	}
}

func addInstallerScore(d *InstallerDetection, score int, kind, reason string) {
	d.Confidence += score

	if d.Kind == "" || score >= 50 {
		d.Kind = kind
	}

	for _, existing := range d.Reasons {
		if existing == reason {
			return
		}
	}
	d.Reasons = append(d.Reasons, reason)
}

func installerStringHits(ctx context.Context, exePath string) []string {
	patterns := []string{
		"installshield",
		"inno setup",
		"nullsoft",
		"nsis",
		"wise installation",
		"wise installer",
		"self-extracting archive",
		"self extracting archive",
		"setup launcher",
		"setup initialization",
		"install wizard",
		"windows installer",
		"data1.cab",
		"setup.inx",
		"setup.iss",
		".msi",
		"uninstall",
	}

	out, err := runCmd(ctx, "strings", "-a", exePath)
	if err != nil {
		return nil
	}

	var hits []string
	seen := map[string]bool{}

	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		lower := strings.ToLower(trimmed)
		for _, p := range patterns {
			if strings.Contains(lower, p) {
				if !seen[trimmed] {
					hits = append(hits, trimmed)
					seen[trimmed] = true
				}
				break
			}
		}

		if len(hits) >= 80 {
			break
		}
	}

	return hits
}
