package kirikiri2

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DarlingGoose/krkrxp3/pkg/xp3"
	"github.com/DarlingGoose/vntext/pkg/game"
)

func TestInstallXP3TextHookDetectsStartupAfterExtract(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	startupSource := `Scripts.execStorage("system/Initialize.tjs");` + "\r\n"
	if err := os.WriteFile(filepath.Join(sourceDir, startupFileName), []byte(startupSource), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := createXP3FromFolder(filepath.Join(root, "data.xp3"), sourceDir, false); err != nil {
		t.Fatalf("create fixture xp3: %v", err)
	}

	g := &game.Game{}
	e := New()
	if err := e.installXP3TextHook(context.Background(), g, root); err != nil {
		t.Fatalf("installXP3TextHook() error = %v", err)
	}

	reader, err := xp3.OpenReader(filepath.Join(root, wglPatchXP3Name))
	if err != nil {
		t.Fatalf("open patch xp3: %v", err)
	}
	defer reader.Close()

	outDir := filepath.Join(root, "out")
	if err := reader.ExtractAll(outDir, xp3.ExtractOptions{}); err != nil {
		t.Fatalf("extract patch xp3: %v", err)
	}

	patchedStartup, err := os.ReadFile(filepath.Join(outDir, startupFileName))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(patchedStartup); got != `Scripts.execStorage("system/Initialize.tjs");`+"\r\n"+`Scripts.execStorage("text_logger.tjs");`+"\r\n" {
		t.Fatalf("startup.tjs mismatch:\n%s", got)
	}

	if _, err := os.Stat(filepath.Join(outDir, textLoggerFileName)); err != nil {
		t.Fatalf("text logger was not packed: %v", err)
	}
	loggerSource, err := os.ReadFile(filepath.Join(outDir, textLoggerFileName))
	if err != nil {
		t.Fatal(err)
	}
	wantLogExe := escapeTJSString(winePathForHostPath(filepath.Join(root, "log.exe")))
	if !strings.Contains(string(loggerSource), `var __TEXT_LOGGER_EXE_PATH = "`+wantLogExe+`";`) {
		t.Fatalf("text logger does not use absolute log.exe path %q", wantLogExe)
	}
	if g.TextHookLogFile != filepath.Join(root, "vntext.log") {
		t.Fatalf("TextHookLogFile = %q", g.TextHookLogFile)
	}
}

func TestTextLoggerDoesNotInstallChoiceAndMenuTagHooks(t *testing.T) {
	tags := []string{
		"button",
		"link",
		"glink",
		"select",
		"seladd",
		"selopt",
		"mselect",
		"mseladd",
		"mselopt",
		"checkbox",
		"edit",
	}

	for _, tag := range tags {
		install := `    __tl_install_tag("` + tag + `",`
		if strings.Contains(textLoggerSource, install) {
			t.Fatalf("text logger installs save/menu-sensitive tag hook %s", tag)
		}
	}

	required := []string{
		"var __TEXT_LOGGER_SHOW_POPUP = false;",
		"var __TEXT_LOGGER_LOG_CONTROL_TAGS = false;",
		"if (!__TEXT_LOGGER_LOG_CONTROL_TAGS)",
		"spawning log.exe while those UIs are being built can freeze old KAG games",
		`appendTextLog("[text_logger][system] text_logger installed");`,
	}
	for _, snippet := range required {
		if !strings.Contains(textLoggerSource, snippet) {
			t.Fatalf("text logger missing safety snippet: %s", snippet)
		}
	}
}

func TestTextLoggerNormalizesRepeatedSpeakerPrefixBeforeFlush(t *testing.T) {
	required := []string{
		"function __tl_normalize_dialogue_text(text)",
		"var __tl_current_character_fresh = false;",
		"function __tl_clear_stale_character()",
		"function __tl_strip_leading_noise(text)",
		"if (!__tl_try_infer_speaker_from_repeated_prefix(text))",
		"function __tl_is_unknown_speaker_prefix(prefix)",
		"candidate + candidate == prefix",
		"__tl_strip_speaker_artifacts(text, __tl_current_character);",
		"function __tl_set_voice_from_params(kind, params)",
		"__tl_get_voice_arg()",
		"__tl_install_tag(\"playse\", __tl_wrap_playse);",
		"var text = __tl_normalize_dialogue_text(__tl_text_buffer);",
		"__tl_current_character_fresh = false;",
		"__tl_current_voice = \"\";",
	}

	for _, snippet := range required {
		if !strings.Contains(textLoggerSource, snippet) {
			t.Fatalf("text logger missing speaker normalization snippet: %s", snippet)
		}
	}
}

func TestKiriKiriArchiveCacheDirChangesWhenArchiveChanges(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "data.xp3")
	if err := os.WriteFile(archive, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := kirikiriArchiveCacheDir(archive)
	if err != nil {
		t.Fatalf("kirikiriArchiveCacheDir() error = %v", err)
	}

	if err := os.WriteFile(archive, []byte("two-two"), 0o644); err != nil {
		t.Fatal(err)
	}

	second, err := kirikiriArchiveCacheDir(archive)
	if err != nil {
		t.Fatalf("kirikiriArchiveCacheDir() error = %v", err)
	}

	if first == second {
		t.Fatalf("cache dir did not change after archive content metadata changed: %s", first)
	}
}

func TestCachedKiriKiriArchiveDataDirReusesCompletedExtraction(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "source")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, startupFileName), []byte(`Scripts.execStorage("system/Initialize.tjs");`), 0o644); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(root, "data.xp3")
	if err := createXP3FromFolder(archive, sourceDir, false); err != nil {
		t.Fatalf("create fixture xp3: %v", err)
	}

	first, err := cachedKiriKiriArchiveDataDir(context.Background(), archive)
	if err != nil {
		t.Fatalf("cachedKiriKiriArchiveDataDir() first error = %v", err)
	}
	second, err := cachedKiriKiriArchiveDataDir(context.Background(), archive)
	if err != nil {
		t.Fatalf("cachedKiriKiriArchiveDataDir() second error = %v", err)
	}

	if first != second {
		t.Fatalf("cache data dir was not reused: first=%s second=%s", first, second)
	}

	cacheDir, err := kirikiriArchiveCacheDir(archive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, ".vntext-complete")); err != nil {
		t.Fatalf("cache marker missing: %v", err)
	}
}
