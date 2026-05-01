package kirikiri2

import (
	"context"
	"os"
	"path/filepath"
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
	if g.TextHookLogFile != filepath.Join(root, "vntext.log") {
		t.Fatalf("TextHookLogFile = %q", g.TextHookLogFile)
	}
}
