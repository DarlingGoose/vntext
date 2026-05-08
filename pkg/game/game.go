package game

import (
	"os"
	"strings"
	"time"

	"github.com/DarlingGoose/gr"
	"github.com/DarlingGoose/gr/autorunner"
	"github.com/DarlingGoose/vntext/pkg/util"
)

type RunnerType string

const (
	RunnerWine      RunnerType = "wine"
	RunnerGamescope RunnerType = "gamescope"
	RunnerProton    RunnerType = "proton"
	RunnerSteam     RunnerType = "steam"
)

type Architecture string

const (
	ArchitectureX86   Architecture = "x86"
	ArchitectureX64   Architecture = "x64"
	ArchitectureARM64 Architecture = "arm64"
)

func WineArchToArchitecture(arch autorunner.FileArch) Architecture {
	switch arch {
	case autorunner.ArchWin64:
		return ArchitectureX64
	case autorunner.ArchWin32:
		return ArchitectureX86
	default:
		return ArchitectureX86
	}
}

type Game struct {
	Name           string        `json:"name"`
	GamePath       string        `json:"game_path"`
	Executable     string        `json:"executable"`
	Architecture   Architecture  `json:"architecture,omitempty"`
	WorkingDir     string        `json:"working_dir"`
	IconPath       string        `json:"icon_path,omitempty"`
	ImagePath      string        `json:"image_path,omitempty"`
	Runner         RunnerType    `json:"runner"`
	RunnerPath     string        `json:"runner_path"`
	RunnerConfig   gr.Config     `json:"runner_config,omitempty"`
	PrefixPath     string        `json:"prefix_path,omitempty"`
	VirtualDesktop string        `json:"virtual_desktop,omitempty"`
	RequiresSteam  bool          `json:"requires_steam"`
	SteamAppID     string        `json:"steam_app_id,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	RuntimeInfo    RuntimeStatus `json:"runtime_info"`

	Locale        string `json:"locale,omitempty"`
	StageToPrefix bool   `json:"stage_to_prefix,omitempty"`
	StagedPath    string `json:"staged_path,omitempty"`

	TextHookLogFile string   `json:"text_hook_log_file"`
	EnvVars         []EnvVar `json:"env_vars"`
	EngineName      string   `json:"engine_name"`
}

func (g *Game) DeleteLog() error {
	if !util.IsFile(g.TextHookLogFile) {
		return nil
	}
	return os.Remove(g.TextHookLogFile)
}
func (g *Game) LogSize() (int64, error) {
	if !util.IsFile(g.TextHookLogFile) {
		return 0, nil
	}
	stat, err := os.Stat(g.TextHookLogFile)
	if err != nil {
		return 0, err
	}

	return stat.Size(), err
}

type EnvVar struct {
	Key   string
	Value string
}

func EnvStringToEnv(env []string) []EnvVar {
	var v []EnvVar
	for _, e := range env {
		key, value, ok := strings.Cut(e, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		v = append(v, EnvVar{Key: key, Value: value})
	}
	return v
}
