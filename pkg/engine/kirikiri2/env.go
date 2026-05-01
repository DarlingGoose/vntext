// pkg/engines/kirikiri2/env.go
package kirikiri2

import (
	"os"
	"strings"

	"github.com/DarlingGoose/vntext/pkg/game"
)

func baseWineEnv(g *game.Game) []string {
	env := cleanWineEnv(os.Environ())

	overrides := "winemenubuilder.exe=d"

	env = append(env, "WINEDLLOVERRIDES="+overrides)
	if wineArch := game.WineArchitecture(g.Architecture); wineArch != "" {
		env = append(env, "WINEARCH="+wineArch)
	}

	if strings.TrimSpace(g.Locale) != "" {
		env = append(env,
			"LANG="+g.Locale,
			"LC_ALL="+g.Locale,
		)
	}

	for _, kv := range g.EnvVars {
		if strings.TrimSpace(kv.Key) == "" {
			continue
		}
		env = append(env, kv.Key+"="+kv.Value)
	}

	return env
}

func cleanWineEnv(env []string) []string {
	out := make([]string, 0, len(env))

	for _, e := range env {
		switch {
		case strings.HasPrefix(e, "WINEARCH="):
			continue
		case strings.HasPrefix(e, "WINEPREFIX="):
			continue
		case strings.HasPrefix(e, "STEAM_COMPAT_DATA_PATH="):
			continue
		case strings.HasPrefix(e, "STEAM_COMPAT_CLIENT_INSTALL_PATH="):
			continue
		}

		out = append(out, e)
	}

	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
