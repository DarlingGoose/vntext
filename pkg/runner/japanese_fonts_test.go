package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DarlingGoose/vntext/pkg/game"
)

func TestIsJapaneseLocale(t *testing.T) {
	tests := map[string]bool{
		"ja":          true,
		"ja_JP.UTF-8": true,
		"ja.UTF-8":    true,
		"en_US.UTF-8": false,
		"":            false,
	}

	for locale, want := range tests {
		if got := isJapaneseLocale(locale); got != want {
			t.Fatalf("isJapaneseLocale(%q) = %v, want %v", locale, got, want)
		}
	}
}

func TestFindJapaneseFonts(t *testing.T) {
	root := t.TempDir()
	fontDir := filepath.Join(root, "truetype", "noto")
	if err := os.MkdirAll(fontDir, 0o755); err != nil {
		t.Fatal(err)
	}

	jpFont := filepath.Join(fontDir, "NotoSansCJK-Regular.ttc")
	if err := os.WriteFile(jpFont, []byte("font"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fontDir, "DejaVuSans.ttf"), []byte("font"), 0o644); err != nil {
		t.Fatal(err)
	}

	fonts := findJapaneseFonts([]string{root})
	if len(fonts) != 1 || fonts[0] != jpFont {
		t.Fatalf("findJapaneseFonts() = %v, want [%s]", fonts, jpFont)
	}
}

func TestProvisionJapaneseFontsIntoWinePrefix(t *testing.T) {
	sourceRoot := t.TempDir()
	sourceFont := filepath.Join(sourceRoot, "VL-Gothic-Regular.ttf")
	if err := os.WriteFile(sourceFont, []byte("font"), 0o644); err != nil {
		t.Fatal(err)
	}

	driveC := filepath.Join(t.TempDir(), "drive_c")
	if err := provisionJapaneseFonts(driveC, []string{sourceRoot}); err != nil {
		t.Fatalf("provisionJapaneseFonts() error = %v", err)
	}

	target := filepath.Join(driveC, "windows", "Fonts", filepath.Base(sourceFont))
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("provisioned font missing: %v", err)
	}
}

func TestEnsureJapaneseFontsForGameUsesProtonDriveC(t *testing.T) {
	sourceRoot := t.TempDir()
	sourceFont := filepath.Join(sourceRoot, "ipaexg.ttf")
	if err := os.WriteFile(sourceFont, []byte("font"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(japaneseFontDirsEnv, sourceRoot)

	prefix := t.TempDir()
	g := &game.Game{
		Locale:     "ja_JP.UTF-8",
		PrefixPath: prefix,
		Runner:     game.RunnerProton,
	}

	ensureJapaneseFontsForGame(g)

	target := filepath.Join(prefix, "pfx", "drive_c", "windows", "Fonts", filepath.Base(sourceFont))
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("provisioned proton font missing: %v", err)
	}
}
