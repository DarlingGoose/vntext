// pkg/engines/kirikiri2/text_hook_xp3.go
package kirikiri2

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DarlingGoose/krkrxp3/pkg/xp3"
	"github.com/DarlingGoose/vntext/pkg/game"
	"github.com/DarlingGoose/vntext/pkg/textfile"
	"github.com/DarlingGoose/vntext/pkg/util"
)

const (
	textLoggerFileName = "text_logger.tjs"
	startupFileName    = "startup.tjs"
	wglPatchXP3Name    = "data.xp3"
)

type xp3PatchPlan struct {
	SourceArchive string
	OutputArchive string
	WorkDir       string
	DataDir       string
	StartupPath   string
	LoggerPath    string
	Flatten       bool
	OmitPathTerms bool
}

func (e *Engine) installXP3TextHook(ctx context.Context, g *game.Game, root string) error {

	archive, err := findBestKiriKiriArchive(root)
	if err != nil {
		return err
	}

	workDir, err := os.MkdirTemp("", "wgl-krkr-hook-*")
	if err != nil {
		return fmt.Errorf("create temp hook dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	plan := xp3PatchPlan{
		SourceArchive: archive,
		OutputArchive: filepath.Join(root, wglPatchXP3Name),
		WorkDir:       workDir,
	}
	if err := extractXP3ForPatch(ctx, &plan); err != nil {
		return err
	}

	if err := detectExtractedLayout(&plan); err != nil {
		return err
	}

	if err := writeTextLogger(plan.LoggerPath, plan.StartupPath); err != nil {
		return err
	}

	if err := patchStartupTJS(plan.StartupPath); err != nil {
		return err
	}

	if err := repackXP3Patch(ctx, &plan); err != nil {
		return err
	}

	g.TextHookLogFile = filepath.Join(root, "vntext.log")

	return nil
}

func findBestKiriKiriArchive(root string) (string, error) {
	names := []string{
		"patch.xp3",
		"data.xp3",
		"scenario.xp3",
	}

	for _, name := range names {
		path := filepath.Join(root, name)
		if util.IsFile(path) {
			return path, nil
		}
	}

	var archives []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".xp3") {
			archives = append(archives, path)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("scan xp3 archives: %w", err)
	}

	if len(archives) == 0 {
		return "", fmt.Errorf("no XP3 archive found in %s", root)
	}

	sortXP3Archives(archives)

	return archives[0], nil
}

func sortXP3Archives(paths []string) {
	score := func(path string) int {
		base := strings.ToLower(filepath.Base(path))
		switch base {
		case "patch.xp3":
			return 100
		case "data.xp3":
			return 90
		case "scenario.xp3":
			return 80
		default:
			if strings.Contains(base, "patch") {
				return 70
			}
			if strings.Contains(base, "data") {
				return 60
			}
			return 0
		}
	}

	sort.Slice(paths, func(i, j int) bool {
		si := score(paths[i])
		sj := score(paths[j])
		if si == sj {
			return paths[i] < paths[j]
		}
		return si > sj
	})
}

func extractXP3ForPatch(ctx context.Context, plan *xp3PatchPlan) error {
	reader, err := xp3.OpenReader(plan.SourceArchive)
	if err != nil {
		return fmt.Errorf("open xp3 archive: %w", err)
	}
	defer reader.Close()

	// First try normal extraction.
	err = reader.ExtractAll(plan.WorkDir, xp3.ExtractOptions{
		Logger: slog.Default(),
	})
	if err == nil {
		return nil
	}

	// Some archives need omit-path-terminator behavior on repack, not usually
	// on extract. Keep the failed extraction error clear for now.
	return fmt.Errorf("extract xp3 %s: %w", plan.SourceArchive, err)
}

func detectExtractedLayout(plan *xp3PatchPlan) error {
	candidates := []string{
		plan.WorkDir,
		filepath.Join(plan.WorkDir, "data"),
		filepath.Join(plan.WorkDir, strings.TrimSuffix(filepath.Base(plan.SourceArchive), filepath.Ext(plan.SourceArchive))),
	}

	for _, dir := range candidates {
		startup := filepath.Join(dir, startupFileName)
		if util.IsFile(startup) {
			plan.DataDir = dir
			plan.StartupPath = startup
			plan.LoggerPath = filepath.Join(dir, textLoggerFileName)
			plan.Flatten = false
			return nil
		}
	}

	var found []string
	_ = filepath.WalkDir(plan.WorkDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Base(path), startupFileName) {
			found = append(found, path)
		}
		return nil
	})

	if len(found) == 0 {
		return fmt.Errorf("extracted archive does not contain %s", startupFileName)
	}

	sort.Strings(found)

	plan.StartupPath = found[0]
	plan.DataDir = filepath.Dir(found[0])
	plan.LoggerPath = filepath.Join(plan.DataDir, textLoggerFileName)

	return nil
}

func writeTextLogger(loggerPath string, startupPath string) error {
	startup, err := textfile.Read(startupPath)
	if err != nil {
		return err
	}

	info, err := os.Stat(startupPath)
	if err != nil {
		return err
	}

	source := normalizeHookNewlines(textLoggerSource, startup.Text)

	return textfile.Write(loggerPath, source, startup.Style, info.Mode().Perm())
}

func patchStartupTJS(startupPath string) error {
	return textfile.Update(startupPath, func(text string, style textfile.FileStyle) (string, error) {
		line := `Scripts.execStorage("text_logger.tjs");`

		if strings.Contains(text, line) {
			return text, nil
		}

		insert := line + detectNewline(text)

		// Best case: insert right after Initialize.tjs include.
		markers := []string{
			`Scripts.execStorage("system/Initialize.tjs");`,
			`Scripts.execStorage('system/Initialize.tjs');`,
		}

		for _, marker := range markers {
			idx := strings.Index(text, marker)
			if idx >= 0 {
				end := idx + len(marker)

				// Preserve existing line ending after marker.
				if end < len(text) {
					if text[end] == '\r' && end+1 < len(text) && text[end+1] == '\n' {
						end += 2
					} else if text[end] == '\n' {
						end++
					}
				}

				return text[:end] + insert + text[end:], nil
			}
		}

		// Fallback: put it at top.
		return insert + text, nil
	})
}

func normalizeHookNewlines(source string, reference string) string {
	nl := detectNewline(reference)
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")

	if nl != "\n" {
		source = strings.ReplaceAll(source, "\n", nl)
	}

	if !strings.HasSuffix(source, nl) {
		source += nl
	}

	return source
}

func detectNewline(s string) string {
	if strings.Contains(s, "\r\n") {
		return "\r\n"
	}
	if strings.Contains(s, "\r") {
		return "\r"
	}
	return "\n"
}

func repackXP3Patch(ctx context.Context, plan *xp3PatchPlan) error {
	backupPath := plan.OutputArchive + ".bak"

	if util.IsFile(plan.OutputArchive) {
		_ = os.Remove(backupPath)
		if err := os.Rename(plan.OutputArchive, backupPath); err != nil {
			return fmt.Errorf("backup existing %s: %w", plan.OutputArchive, err)
		}
	}

	if err := createXP3FromFolder(plan.OutputArchive, plan.WorkDir, false); err == nil {
		return nil
	}

	// Retry with omit path terminators, because some KiriKiri games/tools are picky.
	if err := createXP3FromFolder(plan.OutputArchive, plan.WorkDir, true); err != nil {
		if util.IsFile(backupPath) {
			_ = os.Rename(backupPath, plan.OutputArchive)
		}
		return err
	}

	return nil
}

func createXP3FromFolder(output string, inputDir string, omitPathTerms bool) error {
	writer, err := xp3.CreateWriter(output)
	if err != nil {
		return fmt.Errorf("create xp3 writer: %w", err)
	}

	if err := writer.AddFolder(inputDir, xp3.AddFolderOptions{
		Flatten:             false,
		OmitPathTerminators: omitPathTerms,
		SaveTimestamps:      true,
		Logger:              slog.Default(),
	}); err != nil {
		closeErr := writer.Close()
		if closeErr != nil {
			return fmt.Errorf("pack xp3: %w; close: %v", err, closeErr)
		}
		return fmt.Errorf("pack xp3: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("close xp3 writer: %w", err)
	}

	return nil
}
