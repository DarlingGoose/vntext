package game

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectExecutableArchitecture(t *testing.T) {
	tests := []struct {
		name    string
		machine []byte
		want    Architecture
	}{
		{name: "x86", machine: []byte{0x4c, 0x01}, want: ArchitectureX86},
		{name: "x64", machine: []byte{0x64, 0x86}, want: ArchitectureX64},
		{name: "arm64", machine: []byte{0x64, 0xaa}, want: ArchitectureARM64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "game.exe")
			raw := make([]byte, 0x80+6)
			raw[0] = 'M'
			raw[1] = 'Z'
			raw[0x3c] = 0x80
			copy(raw[0x80:], []byte{'P', 'E', 0, 0})
			copy(raw[0x84:], tt.machine)

			if err := os.WriteFile(path, raw, 0o644); err != nil {
				t.Fatal(err)
			}

			got, err := DetectExecutableArchitecture(path)
			if err != nil {
				t.Fatalf("DetectExecutableArchitecture() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("DetectExecutableArchitecture() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWineArchitecture(t *testing.T) {
	if got := WineArchitecture(ArchitectureX86); got != "win32" {
		t.Fatalf("WineArchitecture(x86) = %q", got)
	}
	if got := WineArchitecture(ArchitectureX64); got != "win64" {
		t.Fatalf("WineArchitecture(x64) = %q", got)
	}
}
