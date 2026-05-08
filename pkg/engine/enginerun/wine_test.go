package enginerun

import (
	"os"
	"strings"
	"testing"

	"github.com/DarlingGoose/gr"
	"github.com/DarlingGoose/gr/gamescope"
	"github.com/DarlingGoose/vntext/pkg/game"
)

func TestRunnerForGameSupportsGamescope(t *testing.T) {
	g := &game.Game{
		Runner:     game.RunnerGamescope,
		RunnerPath: "/usr/bin/gamescope",
		PrefixPath: "/tmp/prefix",
	}

	r, err := RunnerForGame(g)
	if err != nil {
		t.Fatalf("RunnerForGame returned error: %v", err)
	}

	gs, ok := r.(*gamescope.Runner)
	if !ok {
		t.Fatalf("RunnerForGame returned %T, want *gamescope.Runner", r)
	}
	if !gs.UseWine {
		t.Fatal("gamescope runner should run target through wine")
	}
	if gs.GamescopeBin != g.RunnerPath {
		t.Fatalf("GamescopeBin = %q, want %q", gs.GamescopeBin, g.RunnerPath)
	}
	if gs.DefaultWinePrefix != g.PrefixPath {
		t.Fatalf("DefaultWinePrefix = %q, want %q", gs.DefaultWinePrefix, g.PrefixPath)
	}
}

func TestWineBinIgnoresGamescopeRunnerPath(t *testing.T) {
	g := &game.Game{
		Runner:     game.RunnerGamescope,
		RunnerPath: "/usr/bin/gamescope",
	}

	if got := WineBin(g); got != "" {
		t.Fatalf("WineBin = %q, want empty for gamescope runner", got)
	}
}

func TestWineOptionsGameLocaleOverridesStoredCUTF8ForStagedPrefixGame(t *testing.T) {
	g := &game.Game{
		Name:       "lpk-30003",
		Executable: "/home/n9s/.config/vntext/prefixes/lpk-30003/drive_c/Games/lpk-30003/KSH_dl.exe",
		WorkingDir: "/home/n9s/.config/vntext/prefixes/lpk-30003/drive_c/Games/lpk-30003",
		PrefixPath: "/home/n9s/.config/vntext/prefixes/lpk-30003",
		Runner:     game.RunnerWine,
		Locale:     "ja_JP.UTF-8",
		RunnerConfig: gr.Config{
			WorkingDir: "/home/n9s/.config/vntext/prefixes/lpk-30003/drive_c/Games/lpk-30003",
			WinePrefix: "/home/n9s/.config/vntext/prefixes/lpk-30003",
			Envs: []string{
				"LANG=C.UTF-8",
				"LC_ALL=C.UTF-8",
				"LC_CTYPE=C.UTF-8",
				"LC_MESSAGES=C.UTF-8",
				"WINEDEBUG=-all",
			},
		},
	}

	opts, err := WineOptions(g)
	if err != nil {
		t.Fatalf("WineOptions returned error: %v", err)
	}

	cfg := gr.ApplyOptions(opts...).Config()
	want := map[string]string{
		"LANG":        "ja_JP.UTF-8",
		"LC_ALL":      "ja_JP.UTF-8",
		"LC_CTYPE":    "ja_JP.UTF-8",
		"LC_MESSAGES": "ja_JP.UTF-8",
	}
	for key, value := range want {
		if got := envValue(cfg.Envs, key); got != value {
			t.Fatalf("%s = %q, want %q; envs=%v", key, got, value, cfg.Envs)
		}
	}
	if got := envValue(cfg.Envs, "WINEDEBUG"); got != "-all" {
		t.Fatalf("WINEDEBUG = %q, want -all", got)
	}
}

func TestWineOptionsDetectsLocaleFromLocalStagedPrefixExecutable(t *testing.T) {
	exe := "/home/n9s/.config/vntext/prefixes/lpk-30003/drive_c/Games/lpk-30003/KSH_dl.exe"
	if _, err := os.Stat(exe); err != nil {
		t.Skipf("local game executable is not present: %s", exe)
	}

	g := &game.Game{
		Name:       "lpk-30003",
		Executable: exe,
		WorkingDir: "/home/n9s/.config/vntext/prefixes/lpk-30003/drive_c/Games/lpk-30003",
		PrefixPath: "/home/n9s/.config/vntext/prefixes/lpk-30003",
		Runner:     game.RunnerWine,
		RunnerConfig: gr.Config{
			WorkingDir: "/home/n9s/.config/vntext/prefixes/lpk-30003/drive_c/Games/lpk-30003",
			WinePrefix: "/home/n9s/.config/vntext/prefixes/lpk-30003",
			Envs: []string{
				"LANG=C.UTF-8",
				"LC_ALL=C.UTF-8",
				"LC_CTYPE=C.UTF-8",
				"LC_MESSAGES=C.UTF-8",
			},
		},
	}

	if got := GameWineLocale(g); got != "ja_JP.UTF-8" {
		t.Fatalf("GameWineLocale = %q, want ja_JP.UTF-8", got)
	}

	opts, err := WineOptions(g)
	if err != nil {
		t.Fatalf("WineOptions returned error: %v", err)
	}

	cfg := gr.ApplyOptions(opts...).Config()
	for _, key := range []string{"LANG", "LC_ALL", "LC_CTYPE", "LC_MESSAGES"} {
		if got := envValue(cfg.Envs, key); got != "ja_JP.UTF-8" {
			t.Fatalf("%s = %q, want ja_JP.UTF-8; envs=%v", key, got, cfg.Envs)
		}
	}
}

func envValue(envs []string, key string) string {
	prefix := key + "="
	for _, env := range envs {
		if strings.HasPrefix(env, prefix) {
			return strings.TrimPrefix(env, prefix)
		}
	}
	return ""
}
