package artemis

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/game"
)

func (e *Engine) InstallHook(ctx context.Context, g *game.Game) error {
	if g == nil {
		return errors.New("game is nil")
	}
	if g.GamePath == "" {
		return errors.New("game path is empty")
	}

	if err := e.ensurePFSInstalled(); err != nil {
		if roots, _ := findExtractedRoots(g.GamePath); len(roots) == 0 {
			return err
		}
	}

	if err := e.extractArchives(ctx, g.GamePath, false); err != nil {
		return err
	}

	roots, err := findExtractedRoots(g.GamePath)
	if err != nil {
		return err
	}
	if len(roots) == 0 {
		return fmt.Errorf("no extracted Artemis roots found under %s", g.GamePath)
	}

	pluginBytes, err := fs.ReadFile(pluginFS, "plugin/artemis_logger.lua")
	if err != nil {
		return err
	}
	if err := engine.InstallLogExe(context.Background(), g.WorkingDir); err != nil {
		return err
	}
	var patched []string
	modifiedRoots := map[string]bool{}

	for _, root := range roots {
		if err := installLoggerIntoRoot(root, pluginBytes); err != nil {
			return err
		}

		files, err := patchLikelyLuaEntryPoints(root, false)
		if err != nil {
			return err
		}

		if len(files) > 0 {
			patched = append(patched, files...)
			modifiedRoots[root] = true
		}
	}

	if len(patched) == 0 {
		return errors.New("installed logger file, but could not find a Lua entrypoint to patch")
	}

	if err := e.repackModifiedRoots(ctx, g.GamePath, modifiedRoots); err != nil {
		return err
	}

	g.TextHookLogFile = filepath.Join(g.GamePath, dialogueJSONLName)
	return nil
}

func (e *Engine) repackModifiedRoots(ctx context.Context, gameDir string, modifiedRoots map[string]bool) error {
	if len(modifiedRoots) == 0 {
		return nil
	}

	if err := e.ensurePFSInstalled(); err != nil {
		return err
	}

	roots := make([]string, 0, len(modifiedRoots))
	for root := range modifiedRoots {
		roots = append(roots, root)
	}

	sort.Slice(roots, func(i, j int) bool {
		return rootSortIndex(roots[i]) < rootSortIndex(roots[j])
	})

	for _, root := range roots {
		archive, ok := archiveForExtractedRoot(gameDir, root)
		if !ok {
			continue
		}

		if !exists(archive) {
			return fmt.Errorf("archive for root %q does not exist: %s", root, archive)
		}

		if err := backupOnce(archive); err != nil {
			return err
		}

		if err := e.repackRoot(ctx, root, archive); err != nil {
			return err
		}

		if err := removeExtractedRootDir(gameDir, root, archive); err != nil {
			return err
		}
	}

	return nil
}
func (e *Engine) repackRoot(ctx context.Context, rootDir, archivePath string) error {
	if err := e.ensurePFSInstalled(); err != nil {
		return err
	}

	rootDir = filepath.Clean(rootDir)
	archivePath = filepath.Clean(archivePath)

	tmpArchive := archivePath + ".yomuna.new"

	_ = os.Remove(tmpArchive)

	inputDir := rootDir + string(os.PathSeparator)
	cmd := exec.CommandContext(ctx, e.pfsBin, "create", "-f", "-o", tmpArchive, inputDir)
	cmd.Dir = filepath.Dir(rootDir)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"pfs-rs create %s failed: %w\n%s",
			rootDir,
			err,
			strings.TrimSpace(string(out)),
		)
	}

	if !exists(tmpArchive) {
		return fmt.Errorf(
			"pfs-rs create succeeded but expected output archive does not exist: %s\n%s",
			tmpArchive,
			strings.TrimSpace(string(out)),
		)
	}

	if err := replaceFile(tmpArchive, archivePath); err != nil {
		return err
	}

	return nil
}

func archiveForExtractedRoot(gameDir, root string) (string, bool) {
	root = filepath.Clean(root)
	gameDir = filepath.Clean(gameDir)

	base := filepath.Base(root)

	if base == "root" {
		return filepath.Join(gameDir, "root.pfs"), true
	}

	const prefix = "root"
	if !strings.HasPrefix(base, prefix) {
		return "", false
	}

	partText := strings.TrimPrefix(base, prefix)
	if len(partText) != 3 {
		return "", false
	}

	if _, err := strconv.Atoi(partText); err != nil {
		return "", false
	}

	// root000 -> root.pfs.000
	// root001 -> root.pfs.001
	// root002 -> root.pfs.002
	return filepath.Join(gameDir, "root.pfs."+partText), true
}

func outputDirForArchive(gameDir, archive string) string {
	base := filepath.Base(archive)

	if base == "root.pfs" {
		return filepath.Join(gameDir, "root")
	}

	const prefix = "root.pfs."
	if strings.HasPrefix(base, prefix) {
		partText := strings.TrimPrefix(base, prefix)

		// root.pfs.000 -> root000
		// root.pfs.001 -> root001
		// root.pfs.002 -> root002
		if _, err := strconv.Atoi(partText); err == nil {
			return filepath.Join(gameDir, "root"+partText)
		}
	}

	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.ReplaceAll(base, ".", "_")
	return filepath.Join(gameDir, base)
}

func backupOnce(path string) error {
	backup := path + ".yomuna.bak"
	if exists(backup) {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return os.WriteFile(backup, data, 0o644)
}

func replaceFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return err
	}
	return nil
	//return os.Remove(src)
}

func removeExtractedRootDir(gameDir, root, archivePath string) error {
	gameDir = filepath.Clean(gameDir)
	root = filepath.Clean(root)
	archivePath = filepath.Clean(archivePath)

	removePath := outputDirForArchive(gameDir, archivePath)
	if !strings.HasPrefix(root, removePath+string(os.PathSeparator)) && root != removePath {
		return fmt.Errorf("refusing to remove extracted root with unexpected archive output dir: root=%s output=%s", root, removePath)
	}

	rel, err := filepath.Rel(gameDir, removePath)
	if err != nil {
		return err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		return fmt.Errorf("refusing to remove extracted root outside game directory: %s", removePath)
	}
	if strings.Contains(rel, string(os.PathSeparator)) {
		return fmt.Errorf("refusing to remove nested extracted root: %s", removePath)
	}
	if !isExtractedRootDirName(filepath.Base(removePath)) {
		return fmt.Errorf("refusing to remove non-Artemis extracted root: %s", removePath)
	}

	return os.RemoveAll(removePath)
}

func upsertHookBlock(path, hook string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	text := string(data)

	backup := path + ".yomuna.bak"
	if !exists(backup) {
		if err := os.WriteFile(backup, data, 0o644); err != nil {
			return err
		}
	}

	start := strings.Index(text, hookMarkerStart)
	if start >= 0 {
		end := strings.Index(text[start:], hookMarkerEnd)
		if end >= 0 {
			endAbs := start + end + len(hookMarkerEnd)

			// Preserve trailing newline if present.
			for endAbs < len(text) && (text[endAbs] == '\n' || text[endAbs] == '\r') {
				endAbs++
			}

			text = text[:start] + hook + text[endAbs:]
			return os.WriteFile(path, []byte(text), 0o644)
		}
	}

	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}

	text += hook
	return os.WriteFile(path, []byte(text), 0o644)
}
