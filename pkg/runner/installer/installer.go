package installer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Backend string

const (
	BackendWine   Backend = "wine"
	BackendProton Backend = "proton"
)

type WineArch string

const (
	WineArchAuto  WineArch = ""
	WineArchWin32 WineArch = "win32"
	WineArchWin64 WineArch = "win64"
)

type InstallOptions struct {
	InstallerPath string
	PrefixPath    string

	Backend    Backend
	ProtonPath string

	WineArch WineArch // "", "win32", or "win64"

	Env     []string
	WorkDir string
}

type InstallResult struct {
	PrefixPath string   `json:"prefix_path"`
	DriveC     string   `json:"drive_c"`
	Candidates []string `json:"candidates"`
}

func InstallWindowsExe(ctx context.Context, opts InstallOptions) (*InstallResult, error) {
	if strings.TrimSpace(opts.InstallerPath) == "" {
		return nil, errors.New("installer path is required")
	}

	installerPath, err := filepath.Abs(expandHome(opts.InstallerPath))
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(installerPath); err != nil {
		return nil, err
	}

	prefixPath := expandHome(opts.PrefixPath)
	if strings.TrimSpace(prefixPath) == "" {
		prefixPath = defaultPrefixPath(installerPath)
	}

	prefixPath, err = filepath.Abs(prefixPath)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(prefixPath, 0o755); err != nil {
		return nil, err
	}

	backend := opts.Backend
	if backend == "" {
		backend = BackendWine
	}

	cmd, err := buildInstallCommand(ctx, backend, installerPath, prefixPath, opts.ProtonPath)
	if err != nil {
		return nil, err
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	cmd.Env = buildWineEnv(prefixPath, opts.WineArch, opts.Env)

	if strings.TrimSpace(opts.WorkDir) != "" {
		cmd.Dir = expandHome(opts.WorkDir)
	} else {
		cmd.Dir = filepath.Dir(installerPath)
	}

	err = cmd.Run()
	if err != nil && opts.Backend == BackendWine && opts.WineArch == WineArchWin32 {
		msg := err.Error()

		// Retry once with automatic Wine arch.
		if strings.Contains(strings.ToLower(msg), "exit status 1") {
			fmt.Fprintln(os.Stderr, "wine win32 prefix failed; retrying with automatic Wine architecture")

			_ = os.RemoveAll(prefixPath)

			cmd, buildErr := buildInstallCommand(ctx, backend, installerPath, prefixPath, opts.ProtonPath)
			if buildErr != nil {
				return nil, buildErr
			}

			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin
			cmd.Env = buildWineEnv(prefixPath, WineArchAuto, opts.Env)

			if strings.TrimSpace(opts.WorkDir) != "" {
				cmd.Dir = expandHome(opts.WorkDir)
			} else {
				cmd.Dir = filepath.Dir(installerPath)
			}

			err = cmd.Run()
		}
	}

	if err != nil {
		return nil, fmt.Errorf("run installer: %w", err)
	}

	driveC := filepath.Join(prefixPath, "drive_c")

	var candidates []string
	for _, o := range []string{driveC, strings.TrimSuffix(opts.InstallerPath, ".exe")} {
		c, err := FindInstalledGameCandidates(o)
		if err != nil {
			continue
		}
		candidates = append(candidates, c...)
	}

	return &InstallResult{
		PrefixPath: prefixPath,
		DriveC:     driveC,
		Candidates: candidates,
	}, nil
}

func buildWineEnv(prefixPath string, arch WineArch, extra []string) []string {
	env := append([]string{}, os.Environ()...)

	env = append(env, extra...)
	env = append(env, "WINEPREFIX="+prefixPath)

	// Important:
	// Do NOT set WINEARCH by default.
	// Modern Arch/Wine may use wow64 mode where WINEARCH=win32 fails.
	switch arch {
	case WineArchWin32:
		env = append(env, "WINEARCH=win32")
	case WineArchWin64:
		env = append(env, "WINEARCH=win64")
	case WineArchAuto:
		// Let Wine choose.
	}

	return env
}

func buildInstallCommand(
	ctx context.Context,
	backend Backend,
	installerPath string,
	prefixPath string,
	protonPath string,
) (*exec.Cmd, error) {
	switch backend {
	case BackendWine:
		wine, err := exec.LookPath("wine")
		if err != nil {
			return nil, errors.New("wine not found; install with: sudo pacman -S wine")
		}

		return exec.CommandContext(ctx, wine, installerPath), nil

	case BackendProton:
		if strings.TrimSpace(protonPath) == "" {
			return nil, errors.New("proton path is required for proton backend")
		}

		protonPath = expandHome(protonPath)

		var protonExe string
		switch {
		case fileExists(filepath.Join(protonPath, "proton")):
			protonExe = filepath.Join(protonPath, "proton")
		case fileExists(protonPath):
			protonExe = protonPath
		default:
			return nil, fmt.Errorf("proton executable not found: %s", protonPath)
		}

		cmd := exec.CommandContext(ctx, protonExe, "run", installerPath)
		cmd.Env = append(os.Environ(),
			"STEAM_COMPAT_DATA_PATH="+prefixPath,
			"STEAM_COMPAT_CLIENT_INSTALL_PATH="+steamInstallPath(),
		)

		return cmd, nil

	default:
		return nil, fmt.Errorf("unsupported installer backend: %s", backend)
	}
}

func PrefixDriveC(prefixPath string, backend Backend) string {
	prefixPath = expandHome(prefixPath)

	if backend == BackendProton {
		return filepath.Join(prefixPath, "pfx", "drive_c")
	}

	return filepath.Join(prefixPath, "drive_c")
}

func FindInstalledGameCandidates(root string) ([]string, error) {
	root = expandHome(root)

	if _, err := os.Stat(root); err != nil {
		return nil, err
	}

	var candidates []string
	seen := map[string]bool{}

	interestingExe := func(path string) bool {
		name := strings.ToLower(filepath.Base(path))

		if name == "unins000.exe" ||
			name == "uninstall.exe" ||
			name == "setup.exe" ||
			name == "installer.exe" ||
			strings.Contains(name, "uninstall") {
			return false
		}

		return strings.HasSuffix(name, ".exe")
	}

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if entry.IsDir() {
			name := strings.ToLower(entry.Name())
			switch name {
			case "windows", "system32", "syswow64", "winsxs", "temp", "users":
				return filepath.SkipDir
			}
			return nil
		}

		lower := strings.ToLower(path)

		// Engine markers.
		isMarker :=
			strings.HasSuffix(lower, ".xp3") ||
				strings.HasSuffix(lower, "startup.tjs") ||
				strings.HasSuffix(lower, ".ks") ||
				strings.HasSuffix(lower, ".tjs") ||
				strings.HasSuffix(lower, "www/js/rpg_core.js") ||
				strings.HasSuffix(lower, "www/js/rmmz_core.js") ||
				strings.HasSuffix(lower, "game.rgss3a") ||
				strings.HasSuffix(lower, ".rvdata") ||
				strings.HasSuffix(lower, ".rvdata2") ||
				strings.HasSuffix(lower, ".rxdata") ||
				strings.HasSuffix(lower, "unityplayer.dll") ||
				strings.HasSuffix(lower, "gameassembly.dll") ||
				strings.HasSuffix(lower, "data.win") ||
				strings.HasSuffix(lower, ".pck")

		if isMarker || interestingExe(path) {
			dir := filepath.Dir(path)

			// If marker is under www/js, use the game root above www.
			dir = normalizeGameRootFromMarker(dir)

			if !seen[dir] {
				seen[dir] = true
				candidates = append(candidates, dir)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidateScore(candidates[i]) > candidateScore(candidates[j])
	})

	return candidates, nil
}

func normalizeGameRootFromMarker(dir string) string {
	slash := filepath.ToSlash(dir)

	switch {
	case strings.HasSuffix(slash, "/www/js"):
		return filepath.Dir(filepath.Dir(dir))
	case strings.HasSuffix(slash, "/www"):
		return filepath.Dir(dir)
	case strings.HasSuffix(slash, "/game"):
		return filepath.Dir(dir)
	default:
		return dir
	}
}

func candidateScore(path string) int {
	lower := strings.ToLower(filepath.ToSlash(path))

	score := 0

	switch {
	case strings.Contains(lower, "/program files/"):
		score += 20
	case strings.Contains(lower, "/program files (x86)/"):
		score += 20
	case strings.Contains(lower, "/games/"):
		score += 25
	}

	badParts := []string{
		"/windows/",
		"/users/",
		"/appdata/",
		"/temp/",
		"/microsoft/",
	}

	for _, bad := range badParts {
		if strings.Contains(lower, bad) {
			score -= 50
		}
	}

	return score
}

func DefaultGamePrefix(installerPath string) string {
	name := strings.TrimSuffix(filepath.Base(installerPath), filepath.Ext(installerPath))
	name = slugify(name)

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".vntext", "prefixes", name)
	}

	return filepath.Join(home, ".cache", "vntext", "prefixes", name)
}

func defaultPrefixPath(installerPath string) string {
	return DefaultGamePrefix(installerPath)
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))

	var b strings.Builder
	lastDash := false

	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}

	return strings.Trim(b.String(), "-")
}
