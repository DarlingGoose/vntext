package rpgmaker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DarlingGoose/vntext/pkg/game"
)

func TestInstallGameUsesDirectExeInput(t *testing.T) {
	root := makeRPGMakerProject(t)
	exe := filepath.Join(root, "Custom_Game.exe")
	otherExe := filepath.Join(root, "Game.exe")
	writeFile(t, exe, "")
	writeFile(t, otherExe, "")

	eng := New()
	if !eng.IsEngine(exe) {
		t.Fatalf("expected direct exe input to be detected as RPG Maker")
	}

	g, err := eng.InstallGame(exe)
	if err != nil {
		t.Fatalf("InstallGame(%q) returned error: %v", exe, err)
	}

	if g.Executable != exe {
		t.Fatalf("expected direct exe %q, got %q", exe, g.Executable)
	}
	if g.WorkingDir != root {
		t.Fatalf("expected working dir %q, got %q", root, g.WorkingDir)
	}
	if g.Name != "Custom Game" {
		t.Fatalf("expected name from direct exe, got %q", g.Name)
	}
}

func TestGetFileFindsProjectAsset(t *testing.T) {
	root := makeRPGMakerProject(t)
	audioPath := filepath.Join(root, "audio", "se", "voice001.ogg")
	if err := os.MkdirAll(filepath.Dir(audioPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, audioPath, "voice-data")

	eng := New()
	info, err := eng.GetFile(&game.Game{
		GamePath:   root,
		WorkingDir: root,
		Executable: filepath.Join(root, "Game.exe"),
	}, "audio/se/voice001")
	if err != nil {
		t.Fatalf("GetFile() returned error: %v", err)
	}
	if string(info.Data) != "voice-data" {
		t.Fatalf("GetFile().Data = %q, want voice-data", string(info.Data))
	}
	if info.Name != "voice001.ogg" {
		t.Fatalf("GetFile().Name = %q, want voice001.ogg", info.Name)
	}
	if info.Ext != ".ogg" {
		t.Fatalf("GetFile().Ext = %q, want .ogg", info.Ext)
	}
	if info.MediaType != "audio/ogg" {
		t.Fatalf("GetFile().MediaType = %q, want audio/ogg", info.MediaType)
	}
}

func TestPluginSourceLogsSpeakerAndVoice(t *testing.T) {
	plugin := New().GetDefaultPlugin()
	required := []string{
		"let currentVoiceFile = \"\";",
		"function setCurrentVoice(kind, audio)",
		"header += \"[speaker:\" + speaker + \"]\";",
		"header += \"[voice:\" + voice + \"]\";",
		"AudioManager.playSe",
		"AudioManager.playVoice",
	}

	for _, snippet := range required {
		if !strings.Contains(plugin, snippet) {
			t.Fatalf("plugin missing snippet: %s", snippet)
		}
	}
}

func TestSetCustomPluginOverridesInstalledSource(t *testing.T) {
	eng := New()
	if err := eng.SetCustomPlugin("// custom plugin"); err != nil {
		t.Fatalf("SetCustomPlugin() error = %v", err)
	}
	if got := eng.GetDefaultPlugin(); got != "// custom plugin" {
		t.Fatalf("GetDefaultPlugin() = %q", got)
	}
}

//func TestInstallGameDerivesNameFromExeWhenProjectRootIsWWW(t *testing.T) {
//	root := t.TempDir()
//	www := filepath.Join(root, "www")
//	if err := os.MkdirAll(filepath.Join(www, "js", "plugins"), 0o755); err != nil {
//		t.Fatalf("create project dirs: %v", err)
//	}
//	writeFile(t, filepath.Join(www, "js", "rpg_core.js"), "")
//	writeFile(t, filepath.Join(www, "js", "plugins.js"), "var $plugins = [];\n")
//	writeFile(t, filepath.Join(root, "Sonataria.exe"), "")
//
//	eng := New()
//	g, err := eng.InstallGame(root)
//	if err != nil {
//		t.Fatalf("InstallGame(%q) returned error: %v", root, err)
//	}
//
//	if g.Name != "Sonataria" {
//		t.Fatalf("expected name from exe, got %q", g.Name)
//	}
//	if g.GamePath != www {
//		t.Fatalf("expected game path %q, got %q", www, g.GamePath)
//	}
//}

func makeRPGMakerProject(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "js", "plugins"), 0o755); err != nil {
		t.Fatalf("create project dirs: %v", err)
	}
	writeFile(t, filepath.Join(root, "js", "rmmz_core.js"), "")
	writeFile(t, filepath.Join(root, "js", "plugins.js"), "var $plugins = [];\n")

	return root
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
