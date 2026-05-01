package game

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	peMachineI386  = 0x014c
	peMachineAMD64 = 0x8664
	peMachineARM64 = 0xaa64
)

func DetectExecutableArchitecture(path string) (Architecture, error) {
	f, err := os.Open(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	defer f.Close()

	var dos [64]byte
	if _, err := io.ReadFull(f, dos[:]); err != nil {
		return "", fmt.Errorf("read DOS header: %w", err)
	}
	if dos[0] != 'M' || dos[1] != 'Z' {
		return "", fmt.Errorf("not a Windows PE executable: %s", path)
	}

	peOffset := int64(binary.LittleEndian.Uint32(dos[0x3c:]))
	if peOffset <= 0 {
		return "", fmt.Errorf("invalid PE header offset in %s", path)
	}

	var coff [6]byte
	if _, err := f.ReadAt(coff[:], peOffset); err != nil {
		return "", fmt.Errorf("read PE header: %w", err)
	}
	if string(coff[:4]) != "PE\x00\x00" {
		return "", fmt.Errorf("missing PE signature in %s", path)
	}

	switch machine := binary.LittleEndian.Uint16(coff[4:]); machine {
	case peMachineI386:
		return ArchitectureX86, nil
	case peMachineAMD64:
		return ArchitectureX64, nil
	case peMachineARM64:
		return ArchitectureARM64, nil
	default:
		return "", fmt.Errorf("unsupported PE machine 0x%04x in %s", machine, path)
	}
}

func WineArchitecture(arch Architecture) string {
	switch arch {
	case ArchitectureX86:
		return "win32"
	case ArchitectureX64, ArchitectureARM64:
		return "win64"
	default:
		return ""
	}
}
