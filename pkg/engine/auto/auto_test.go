package auto

import (
	"testing"

	"github.com/DarlingGoose/vntext/pkg/app"
)

func TestDefaultEngineSelectorV2ReusesEngines(t *testing.T) {
	orig := app.ProgramName
	app.ProgramName = app.DefaultProgramName
	t.Cleanup(func() { app.ProgramName = orig })

	first := DefaultEngineSelectorV2()
	second := DefaultEngineSelectorV2()

	if first != second {
		t.Fatal("DefaultEngineSelectorV2 should reuse the selector for the same program name")
	}

	if first.ByName("rpgmaker") != second.ByName("rpgmaker") {
		t.Fatal("rpgmaker engine should be reused")
	}
	if first.ByName("kirikiri") != second.ByName("krkr2") {
		t.Fatal("kirikiri aliases should resolve to the same engine")
	}
}

func TestNewEngineSelectorV2DefaultsProgramName(t *testing.T) {
	selector := NewEngineSelectorV2("")
	if selector.programName != app.DefaultProgramName {
		t.Fatalf("programName = %q, want %q", selector.programName, app.DefaultProgramName)
	}
}
