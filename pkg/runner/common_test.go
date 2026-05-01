package runner

import (
	"strings"
	"testing"

	"github.com/DarlingGoose/vntext/pkg/game"
)

func TestBaseEnvAddsWineArchFromGameArchitecture(t *testing.T) {
	g := &game.Game{
		Architecture: game.ArchitectureX86,
	}

	got := strings.Join(baseEnv(g), "\n")
	if !strings.Contains(got, "WINEARCH=win32") {
		t.Fatalf("baseEnv() did not include WINEARCH=win32:\n%s", got)
	}
}
