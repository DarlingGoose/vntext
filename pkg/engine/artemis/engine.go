package artemis

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DarlingGoose/gr"
	"github.com/DarlingGoose/tr/pkg/textractor"
	"github.com/DarlingGoose/vntext/pkg/app"
	"github.com/DarlingGoose/vntext/pkg/engine"
	"github.com/DarlingGoose/vntext/pkg/engine/enginerun"
	"github.com/DarlingGoose/vntext/pkg/game"
)

//go:embed plugin/artemis_logger.lua
var pluginFS embed.FS

const (
	engineName        = "artemis"
	pfsBinName        = "pfs-rs"
	loggerFileName    = "artemis_logger.lua"
	dialogueJSONLName = "vntext.log"
	dialogueLogName   = "yomuna-dialogue.log"

	hookMarkerStart = "-- YOMUNA_ARTEMIS_LOGGER_START"
	hookMarkerEnd   = "-- YOMUNA_ARTEMIS_LOGGER_END"
)

var _ engine.EngineV2 = &Engine{}

type Engine struct {
	mu sync.RWMutex

	pfsBin string

	Runner     game.RunnerType
	RunnerPath string
	PrefixRoot string

	managed map[string]*game.Game
}

type Runner interface {
	Run(ctx context.Context, g *game.Game) (*gr.Process, error)
	Stop(ctx context.Context, proc *gr.Process) (*gr.Process, error)
}

type Line struct {
	Engine  string    `json:"engine"`
	Game    string    `json:"game,omitempty"`
	Source  string    `json:"source,omitempty"`
	Speaker string    `json:"speaker,omitempty"`
	Voice   string    `json:"voice,omitempty"`
	Text    string    `json:"text"`
	Time    time.Time `json:"time"`
	Raw     string    `json:"raw,omitempty"`
}

type FollowGameOptions struct {
	MaxLines int
	History  bool
	Filters  []func(l *Line) *Line
}

type EngineFileInfo struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time,omitempty"`
}

func New() (*Engine, error) {
	pfsBin, err := exec.LookPath(pfsBinName)
	if err != nil {
		return nil, fmt.Errorf("missing bin name: %s", pfsBin)
	}
	return &Engine{
		pfsBin:  pfsBin,
		Runner:  game.RunnerWine,
		managed: make(map[string]*game.Game),
	}, nil
}

func (e *Engine) Name() string {
	return engineName
}

func (e *Engine) runner() game.RunnerType {
	if e != nil && e.Runner != "" {
		return e.Runner
	}
	return game.RunnerWine
}

func (e *Engine) prefixPathFor(name string) (string, error) {
	prefixRoot := ""
	if e != nil {
		prefixRoot = e.PrefixRoot
	}
	return enginerun.PrefixPath(app.Name(), prefixRoot, name)
}

func detectExecutableArchitecture(path string) game.Architecture {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	arch, err := game.DetectExecutableArchitecture(path)
	if err != nil {
		return ""
	}
	return arch
}

func (e *Engine) prepareGameForRun(g *game.Game) error {
	if g == nil {
		return errors.New("game is nil")
	}

	if strings.TrimSpace(g.Name) == "" && strings.TrimSpace(g.Executable) != "" {
		g.Name = strings.TrimSuffix(filepath.Base(g.Executable), filepath.Ext(g.Executable))
	}

	if strings.TrimSpace(g.PrefixPath) == "" {
		prefixPath, err := e.prefixPathFor(g.Name)
		if err != nil {
			return err
		}
		g.PrefixPath = prefixPath
	}

	if g.Runner == "" {
		g.Runner = e.runner()
	}

	if strings.TrimSpace(g.RunnerPath) == "" && e != nil {
		g.RunnerPath = e.RunnerPath
	}

	if strings.TrimSpace(g.WorkingDir) == "" {
		g.WorkingDir = enginerun.WorkingDir(g)
	}

	if g.Architecture == "" {
		g.Architecture = detectExecutableArchitecture(g.Executable)
	}

	if !hasRunnerConfig(g.RunnerConfig) {
		if err := enginerun.ConfigureRunner(g); err != nil {
			return err
		}
	}

	if strings.TrimSpace(g.EngineName) == "" {
		g.EngineName = engineName
	}

	if strings.TrimSpace(g.TextHookLogFile) == "" && strings.TrimSpace(g.GamePath) != "" {
		g.TextHookLogFile = filepath.Join(g.GamePath, dialogueJSONLName)
	}

	if g.WineConfig != nil && strings.TrimSpace(g.WineConfig.DefaultPrefix) == "" {
		g.WineConfig.DefaultPrefix = g.PrefixPath
	}

	if g.GamescopeConfig != nil && strings.TrimSpace(g.GamescopeConfig.DefaultWinePrefix) == "" {
		g.GamescopeConfig.DefaultWinePrefix = g.PrefixPath
	}

	if strings.TrimSpace(g.RunnerConfig.WinePrefix) == "" {
		g.RunnerConfig.WinePrefix = g.PrefixPath
	}

	return nil
}

func hasRunnerConfig(c gr.Config) bool {
	return c.WorkingDir != "" ||
		len(c.Args) > 0 ||
		len(c.Envs) > 0 ||
		c.SystemArch != "" ||
		c.WinePrefix != "" ||
		len(c.Dependencies) > 0 ||
		c.Name != "" ||
		c.PID != 0 ||
		c.Session != "" ||
		c.SessionID != ""
}

func (e *Engine) IsEngine(dir string) bool {
	if dir == "" {
		return false
	}
	slog.Info(dir)

	if exists(filepath.Join(dir, "root.pfs")) {
		slog.Info("matches root.pfs.*")
		return true
	}

	if matches, _ := filepath.Glob(filepath.Join(dir, "root.pfs.*")); len(matches) > 0 {
		slog.Info("matches root.pfs.*")
		return true
	}

	if matches, _ := filepath.Glob(filepath.Join(dir, "*.pfs")); len(matches) > 0 {
		slog.Info(" matches *.pfs")
		return true
	}

	// Already extracted Artemis-like layout.
	if matches, _ := filepath.Glob(filepath.Join(dir, "root*", "system", "adv", "*.lua")); len(matches) > 0 {
		return true
	}

	return false
}

func (e *Engine) AddGame(ctx context.Context, gamePath string) (*game.Game, error) {
	if gamePath == "" {
		return nil, errors.New("game path is empty")
	}

	abs, err := filepath.Abs(gamePath)
	if err != nil {
		return nil, err
	}

	if !e.IsEngine(abs) {
		return nil, fmt.Errorf("%s does not look like an Artemis game", abs)
	}

	exe, err := findExecutable(abs)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
	prefixPath, err := e.prefixPathFor(name)
	if err != nil {
		return nil, err
	}

	g := &game.Game{
		Name:            name,
		GamePath:        abs,
		Executable:      exe,
		Architecture:    detectExecutableArchitecture(exe),
		WorkingDir:      filepath.Dir(exe),
		Runner:          e.runner(),
		RunnerPath:      e.RunnerPath,
		PrefixPath:      prefixPath,
		EngineName:      engineName,
		TextHookLogFile: filepath.Join(abs, dialogueJSONLName),
		CreatedAt:       time.Now().UTC(),
	}

	if err := enginerun.ConfigureRunner(g); err != nil {
		return nil, err
	}

	e.mu.Lock()
	e.managed[g.GamePath] = g
	e.mu.Unlock()

	_ = ctx
	return g, nil
}

func (e *Engine) RunGame(ctx context.Context, g *game.Game) (*gr.Process, error) {
	if err := e.prepareGameForRun(g); err != nil {
		return nil, err
	}

	return enginerun.RunGame(ctx, g)
}

func (e *Engine) StopGame(ctx context.Context, proc *gr.Process) (*gr.Process, error) {
	return enginerun.StopGame(ctx, proc)
}

func (e *Engine) FollowGameText(
	ctx context.Context,
	g *game.Game,
	opts ...engine.FollowGameOptions,
) (chan engine.Line, error) {
	return enginerun.FollowGameText(ctx, g, opts...)
}
func (e *Engine) GetFile(g *game.Game, file string) (*engine.EngineFileInfo, error) {
	if g == nil {
		return nil, errors.New("game is nil")
	}

	file = strings.TrimSpace(file)
	if file == "" {
		return nil, errors.New("file is empty")
	}

	path := file
	if !filepath.IsAbs(path) {
		path = filepath.Join(g.GamePath, file)
	}

	path = filepath.Clean(path)

	// Prevent accidental path traversal outside the game directory for relative paths.
	if !filepath.IsAbs(file) {
		gameRoot, err := filepath.Abs(g.GamePath)
		if err != nil {
			return nil, err
		}

		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}

		rel, err := filepath.Rel(gameRoot, absPath)
		if err != nil {
			return nil, err
		}

		if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return nil, fmt.Errorf("file %q is outside game path", file)
		}
	}

	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if st.IsDir() {
		return nil, fmt.Errorf("file %q is a directory", file)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(path))
	mediaType := mime.TypeByExtension(ext)
	if mediaType == "" {
		mediaType = http.DetectContentType(data)
	}

	return &engine.EngineFileInfo{
		Name:      filepath.Base(path),
		Data:      data,
		MediaType: mediaType,
		Ext:       ext,
	}, nil
}

func (e *Engine) Shutdown() error {
	return nil
}

func (e *Engine) ManagedGames() []*game.Game {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]*game.Game, 0, len(e.managed))
	for _, g := range e.managed {
		out = append(out, g)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})

	return out
}

func (e *Engine) GetTextractor(g *game.Game) *textractor.Client {
	return nil
}

// Optional: call this only after testing loose patching.
func (e *Engine) Repack(ctx context.Context, g *game.Game) error {
	if g == nil {
		return errors.New("game is nil")
	}
	if err := e.ensurePFSInstalled(); err != nil {
		return err
	}

	// pfs-rs create syntax may differ depending on version.
	// Verify with: pfs-rs create --help
	//
	// Keep this conservative and backup originals first.
	return errors.New("repack intentionally not enabled; verify pfs-rs create syntax and backup archives first")
}

func (e *Engine) ensurePFSInstalled() error {
	if e.pfsBin != "" {
		return nil
	}

	p, err := exec.LookPath(pfsBinName)
	if err != nil {
		return fmt.Errorf("%s is not installed or not on PATH", pfsBinName)
	}

	e.pfsBin = p
	return nil
}

func (e *Engine) extractArchives(ctx context.Context, dir string, overwrite bool) error {
	if err := e.ensurePFSInstalled(); err != nil {
		return err
	}

	archives, err := findPFSArchives(dir)
	if err != nil {
		return err
	}

	for _, archive := range archives {
		outDir := outputDirForArchive(dir, archive)

		if exists(outDir) && !overwrite {
			if roots, _ := findExtractedRootsInDir(outDir); len(roots) > 0 {
				continue
			}
			_ = os.RemoveAll(outDir)
		}

		if exists(outDir) && !overwrite {
			continue
		}

		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return err
		}

		cmd := exec.CommandContext(ctx, e.pfsBin, "extract", archive, outDir)
		cmd.Dir = dir

		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("pfs-rs extract failed for %s: %w\n%s", archive, err, strings.TrimSpace(string(out)))
		}
	}

	return nil
}

func findPFSArchives(dir string) ([]string, error) {
	var archives []string

	// Main archive first.
	main := filepath.Join(dir, "root.pfs")
	if exists(main) {
		archives = append(archives, main)
	}

	// Some tools need extracting split parts directly; some only need root.pfs.
	// Keep support for both because your TODO calls out root.pfs.002 separately.
	matches, err := filepath.Glob(filepath.Join(dir, "root.pfs.*"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	for _, match := range matches {
		if isGeneratedOrBackupArchive(match) {
			continue
		}
		archives = append(archives, match)
	}

	// Other pfs files.
	other, err := filepath.Glob(filepath.Join(dir, "*.pfs"))
	if err != nil {
		return nil, err
	}
	sort.Strings(other)
	for _, p := range other {
		if filepath.Base(p) == "root.pfs" || isGeneratedOrBackupArchive(p) {
			continue
		}
		archives = append(archives, p)
	}

	return uniqueStrings(archives), nil
}

func findExtractedRoots(gameDir string) ([]string, error) {
	return findExtractedRootsInDir(gameDir)
}

func findExtractedRootsInDir(gameDir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(gameDir, "root*"))
	if err != nil {
		return nil, err
	}

	var roots []string
	for _, candidate := range matches {
		root, ok := normalizeExtractedRootCandidate(candidate)
		if !ok {
			continue
		}
		roots = append(roots, root)
	}

	sort.Slice(roots, func(i, j int) bool {
		return rootSortIndex(roots[i]) < rootSortIndex(roots[j])
	})

	return roots, nil
}

func normalizeExtractedRootCandidate(candidate string) (string, bool) {
	st, err := os.Stat(candidate)
	if err != nil || !st.IsDir() {
		return "", false
	}

	base := filepath.Base(candidate)
	if !isExtractedRootDirName(base) {
		return "", false
	}

	if looksLikeExtractedArtemisRoot(candidate) {
		return candidate, true
	}

	nested := filepath.Join(candidate, base)
	if st, err := os.Stat(nested); err == nil && st.IsDir() && looksLikeExtractedArtemisRoot(nested) {
		return nested, true
	}

	return "", false
}

func isGeneratedOrBackupArchive(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.Contains(base, ".yomuna.") ||
		strings.HasSuffix(base, ".yomuna") ||
		strings.HasSuffix(base, ".bak") ||
		strings.HasSuffix(base, ".new")
}

func isExtractedRootDirName(base string) bool {
	if base == "root" {
		return true
	}

	if !strings.HasPrefix(base, "root") {
		return false
	}

	partText := strings.TrimPrefix(base, "root")
	if len(partText) != 3 {
		return false
	}

	_, err := strconv.Atoi(partText)
	return err == nil
}

func rootSortIndex(path string) int {
	base := filepath.Base(path)
	if base == "root" {
		return 0
	}

	partText := strings.TrimPrefix(base, "root")
	n, err := strconv.Atoi(partText)
	if err != nil {
		return 1_000_000
	}

	return n + 1
}

func looksLikeExtractedArtemisRoot(root string) bool {
	if exists(filepath.Join(root, "system", "first.iet")) ||
		exists(filepath.Join(root, "system", "msg.iet")) ||
		exists(filepath.Join(root, "scenario", "start.txt")) ||
		exists(filepath.Join(root, "scenario", "macro.txt")) {
		return true
	}

	if matches, _ := filepath.Glob(filepath.Join(root, "system", "adv", "*.lua")); len(matches) > 0 {
		return true
	}

	if matches, _ := filepath.Glob(filepath.Join(root, "scenario", "**", "*.txt")); len(matches) > 0 {
		return true
	}

	return false
}

func installLoggerIntoRoot(root string, pluginBytes []byte) error {
	paths := []string{
		filepath.Join(root, loggerFileName),
		filepath.Join(root, "system", loggerFileName),
		filepath.Join(root, "system", "adv", loggerFileName),
	}

	for _, p := range paths {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, pluginBytes, 0o644); err != nil {
			return err
		}
	}

	return nil
}

func likelyLuaEntryPoints(root string) []string {
	return []string{
		//filepath.Join(root, "system", "adv", "init.lua"), filepath.Join(root, "system", "init.lua"),
		filepath.Join(root, "system", "ui", "backlog.lua"),
		//filepath.Join(root, "system", "macro.lua"),
	}
}

func debugLuaEntryPoints(root string) []string {
	return []string{
		filepath.Join(root, "system", "adv", "init.lua"),
		filepath.Join(root, "system", "adv", "adv.lua"),
		filepath.Join(root, "system", "adv", "system.lua"),
		filepath.Join(root, "system", "macro.lua"),
		filepath.Join(root, "scenario", "macro.txt"),
		filepath.Join(root, "scenario", "start.txt"),
		filepath.Join(root, "system", "ui", "backlog.lua"),
		filepath.Join(root, "system", "ui", "dialog.lua"),
		filepath.Join(root, "system", "ui", "char.lua"),
		filepath.Join(root, "system", "adv", "sound.lua"),
	}
}

func patchLikelyLuaEntryPoints(root string, debug bool) ([]string, error) {
	candidates := likelyLuaEntryPoints(root)
	if debug {
		candidates = debugLuaEntryPoints(root)
	}

	hook := artemisHookBlock(debug)

	var patched []string
	for _, p := range candidates {
		if !exists(p) {
			continue
		}

		relToRoot, _ := filepath.Rel(root, p)
		if err := upsertHookBlock(p, hook); err != nil {
			return nil, fmt.Errorf("patch %s: %w", relToRoot, err)
		}

		patched = append(patched, p)
	}

	return patched, nil
}

func artemisHookBlock(debug bool) string {
	lines := []string{
		`if __ymn_artemis_backlog_logger_loaded ~= true then`,
		`__ymn_artemis_backlog_logger_loaded = true`,

		`local function __ymn_quote(s)`,
		`    s = tostring(s or "")`,
		`    s = string.gsub(s, "\\", "\\\\")`,
		`    s = string.gsub(s, '"', '\\"')`,
		`    return '"' .. s .. '"'`,
		`end`,

		`local function __ymn_log(msg, speaker, voice)`,
		`    msg = tostring(msg or "")`,
		`    if msg == "" then return end`,
		``,
		`    local cmd = "log.exe "`,
		`    if speaker ~= nil and tostring(speaker) ~= "" then`,
		`        cmd = cmd .. "-c " .. __ymn_quote(speaker) .. " "`,
		`    end`,
		`    if voice ~= nil and tostring(voice) ~= "" then`,
		`        cmd = cmd .. "-v " .. __ymn_quote(voice) .. " "`,
		`    end`,
		`    cmd = cmd .. __ymn_quote(msg) .. " > NUL 2>&1"`,
		``,
		`    pcall(os.execute, cmd)`,
		`end`,

		`local function __ymn_clean(s)`,
		`    s = tostring(s or "")`,
		`    s = string.gsub(s, "\r", "")`,
		`    s = string.gsub(s, "\n", "\\n")`,
		`    s = string.gsub(s, "^%s+", "")`,
		`    s = string.gsub(s, "%s+$", "")`,
		`    return s`,
		`end`,

		`local __ymn_current_speaker = ""`,
		`local __ymn_last_text = ""`,

		`if type(set_backlog_name) == "function" and not __ymn_original_set_backlog_name then`,
		`    __ymn_original_set_backlog_name = set_backlog_name`,
		`    set_backlog_name = function(name)`,
		`        __ymn_current_speaker = __ymn_clean(name)`,
		`        return __ymn_original_set_backlog_name(name)`,
		`    end`,
		`    __ymn_log("Yomuna wrapped set_backlog_name")`,
		`end`,

		`if type(set_backlog_text) == "function" and not __ymn_original_set_backlog_text then`,
		`    __ymn_original_set_backlog_text = set_backlog_text`,
		`    set_backlog_text = function(com, param)`,
		`        if type(param) == "table" then`,
		`            local text = __ymn_clean(param.data or param.text or param["0"] or "")`,
		`            if text ~= "" and text ~= __ymn_last_text then`,
		`                __ymn_last_text = text`,
		`                __ymn_log(text, __ymn_current_speaker, "")`,
		`            end`,
		`        end`,
		`        return __ymn_original_set_backlog_text(com, param)`,
		`    end`,
		`    __ymn_log("Yomuna wrapped set_backlog_text")`,
		`end`,

		`__ymn_log("Yomuna Artemis backlog logger loaded")`,
		`end`,
	}

	if debug {
		lines = append([]string{
			`pcall(os.execute, "log.exe \"Yomuna direct hook reached backlog\"")`,
		}, lines...)
	}

	println(strings.Join(lines, "\n"))
	return "\n" +
		hookMarkerStart + "\n" +
		strings.Join(lines, "\n") + "\n" +
		hookMarkerEnd + "\n"
}

func appendHookOnce(path, hook string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	text := string(data)
	if strings.Contains(text, hookMarkerStart) {
		return nil
	}

	backup := path + ".yomuna.bak"
	if !exists(backup) {
		if err := os.WriteFile(backup, data, 0o644); err != nil {
			return err
		}
	}

	return os.WriteFile(path, append(data, []byte(hook)...), 0o644)
}

func followFile(ctx context.Context, path string, opt FollowGameOptions, out chan<- Line) error {
	var offset int64

	if opt.History {
		lines, err := readExistingLines(path, opt.MaxLines)
		if err == nil {
			for _, raw := range lines {
				if l := parseLogLine(raw); l != nil {
					l = applyFilters(l, opt.Filters)
					if l != nil {
						select {
						case out <- *l:
						case <-ctx.Done():
							return ctx.Err()
						}
					}
				}
			}
		}
	}

	if st, err := os.Stat(path); err == nil && !opt.History {
		offset = st.Size()
	}

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			next, err := readNewLines(path, offset, out, opt)
			if err == nil {
				offset = next
			}
		}
	}
}

func readNewLines(path string, offset int64, out chan<- Line, opt FollowGameOptions) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return offset, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return offset, err
	}

	if st.Size() < offset {
		offset = 0
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}

	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 1024*1024)
	sc.Buffer(buf, 8*1024*1024)

	for sc.Scan() {
		raw := sc.Text()
		l := parseLogLine(raw)
		if l == nil {
			continue
		}

		l = applyFilters(l, opt.Filters)
		if l == nil {
			continue
		}

		out <- *l
	}

	pos, _ := f.Seek(0, io.SeekCurrent)
	return pos, sc.Err()
}

func readExistingLines(path string, max int) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	if max > 0 && len(lines) > max {
		lines = lines[len(lines)-max:]
	}

	return lines, nil
}

func parseLogLine(raw string) *Line {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var rec struct {
		Engine  string `json:"engine"`
		Game    string `json:"game"`
		Source  string `json:"source"`
		Speaker string `json:"speaker"`
		Voice   string `json:"voice"`
		Text    string `json:"text"`
		Time    string `json:"time"`
	}

	if strings.HasPrefix(raw, "{") {
		if err := json.Unmarshal([]byte(raw), &rec); err == nil && strings.TrimSpace(rec.Text) != "" {
			t, _ := time.Parse(time.RFC3339, rec.Time)
			if t.IsZero() {
				t = time.Now()
			}

			engine := rec.Engine
			if engine == "" {
				engine = engineName
			}

			return &Line{
				Engine:  engine,
				Game:    rec.Game,
				Source:  rec.Source,
				Speaker: rec.Speaker,
				Voice:   rec.Voice,
				Text:    strings.TrimSpace(rec.Text),
				Time:    t,
				Raw:     raw,
			}
		}
	}

	// Plain fallback:
	// [2026-...][engine:artemis][source:x]: text
	text := raw
	if idx := strings.LastIndex(raw, "]: "); idx >= 0 && idx+3 < len(raw) {
		text = raw[idx+3:]
	}

	return &Line{
		Engine: engineName,
		Source: "plain",
		Text:   strings.TrimSpace(text),
		Time:   time.Now(),
		Raw:    raw,
	}
}

func applyFilters(l *Line, filters []func(l *Line) *Line) *Line {
	if l == nil {
		return nil
	}

	for _, f := range filters {
		if f == nil {
			continue
		}
		l = f(l)
		if l == nil {
			return nil
		}
	}

	return l
}

func findExecutable(dir string) (string, error) {
	var exes []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := strings.ToLower(d.Name())
			if name == "root" || name == "root1" || name == "root2" || name == "root3" || name == "stand_gallery" {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.EqualFold(filepath.Ext(path), ".exe") {
			exes = append(exes, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	if len(exes) == 0 {
		return "", fmt.Errorf("no .exe found under %s", dir)
	}

	sort.Slice(exes, func(i, j int) bool {
		// Prefer top-level exe.
		di := strings.Count(strings.TrimPrefix(exes[i], dir), string(os.PathSeparator))
		dj := strings.Count(strings.TrimPrefix(exes[j], dir), string(os.PathSeparator))
		if di != dj {
			return di < dj
		}
		return len(exes[i]) < len(exes[j])
	})

	return exes[0], nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))

	for _, s := range in {
		if s == "" {
			continue
		}
		clean := filepath.Clean(s)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}

	return out
}

// Useful filters.

var japaneseLooseRE = regexp.MustCompile(`[\p{Hiragana}\p{Katakana}\p{Han}ー]`)

func FilterJapaneseOnly(l *Line) *Line {
	if l == nil || !japaneseLooseRE.MatchString(l.Text) {
		return nil
	}
	return l
}

func FilterDedupe() func(l *Line) *Line {
	var last string
	return func(l *Line) *Line {
		if l == nil {
			return nil
		}
		text := strings.TrimSpace(l.Text)
		if text == "" || text == last {
			return nil
		}
		last = text
		l.Text = text
		return l
	}
}
