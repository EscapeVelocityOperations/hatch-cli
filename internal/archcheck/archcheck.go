package archcheck

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ArchType represents the detected binary architecture.
type ArchType int

const (
	ArchUnknown    ArchType = iota
	ArchLinuxAMD64
	ArchLinuxARM64
	ArchMacOS
	ArchWindows
)

// NativeDepResult holds the results of scanning native dependencies.
type NativeDepResult struct {
	BadFiles []string // paths to non-linux-amd64 files
	Total    int      // total native dep files found
}

// DetectArch reads magic bytes from a binary file and returns its architecture type.
func DetectArch(path string) ArchType {
	f, err := os.Open(path)
	if err != nil {
		return ArchUnknown
	}
	defer f.Close()

	var magic [4]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return ArchUnknown
	}

	m := binary.BigEndian.Uint32(magic[:])

	// Mach-O magic numbers (macOS)
	switch m {
	case 0xfeedface, 0xfeedfacf, 0xcefaedfe, 0xcffaedfe:
		return ArchMacOS
	}

	// PE (Windows: MZ header)
	if magic[0] == 'M' && magic[1] == 'Z' {
		return ArchWindows
	}

	// ELF
	if magic[0] == 0x7f && magic[1] == 'E' && magic[2] == 'L' && magic[3] == 'F' {
		var hdr [20]byte
		if _, err := f.Seek(0, 0); err != nil {
			return ArchUnknown
		}
		if _, err := io.ReadFull(f, hdr[:]); err != nil {
			return ArchUnknown
		}
		machine := binary.LittleEndian.Uint16(hdr[18:20])
		switch machine {
		case 0x3E: // EM_X86_64
			return ArchLinuxAMD64
		case 0xB7: // EM_AARCH64
			return ArchLinuxARM64
		}
		return ArchUnknown
	}

	return ArchUnknown
}

// CheckNativeDeps scans a directory for native dependency files (.node for node/bun,
// .so for python) and checks their architecture. Returns an error if ALL native deps
// are wrong-arch. Returns a NativeDepResult for the caller to handle warnings.
func CheckNativeDeps(dir, runtime string) (*NativeDepResult, error) {
	var ext string
	switch runtime {
	case "node", "bun":
		ext = ".node"
	case "python":
		ext = ".so"
	default:
		return nil, nil
	}

	result := &NativeDepResult{}

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ext) {
			return nil
		}

		arch := DetectArch(path)
		if arch == ArchUnknown {
			return nil // not a binary, skip
		}

		result.Total++
		rel, _ := filepath.Rel(dir, path)

		if arch != ArchLinuxAMD64 {
			result.BadFiles = append(result.BadFiles, rel)
		}
		return nil
	})

	if result.Total == 0 {
		return nil, nil
	}

	if len(result.BadFiles) == 0 {
		return nil, nil
	}

	// ALL bad = hard error
	if len(result.BadFiles) == result.Total {
		return result, formatNativeDepError(result, runtime, ext)
	}

	// Mixed = return result for caller to warn, no error
	return result, nil
}

func formatNativeDepError(result *NativeDepResult, runtime, ext string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "found %d native module(s) compiled for the wrong platform (Hatch runs linux/amd64):\n", len(result.BadFiles))
	limit := len(result.BadFiles)
	if limit > 5 {
		limit = 5
	}
	for _, f := range result.BadFiles[:limit] {
		fmt.Fprintf(&b, "  - %s\n", f)
	}
	if len(result.BadFiles) > 5 {
		fmt.Fprintf(&b, "  ... and %d more\n", len(result.BadFiles)-5)
	}

	b.WriteString(fmt.Sprintf("\nThese %s files won't work on Hatch (linux/amd64).\n\n", ext))

	switch runtime {
	case "node", "bun":
		b.WriteString("Fix options:\n")
		b.WriteString("  1. Deploy source + package.json + package-lock.json WITHOUT node_modules\n")
		b.WriteString("     The platform runs npm install inside the linux/amd64 container.\n")
		b.WriteString("  2. Rebuild for Linux: npm rebuild --platform=linux --arch=x64\n")
		b.WriteString("  3. Use Docker: docker run --platform=linux/amd64 -v $PWD:/app -w /app node:20 npm install\n")
	case "python":
		b.WriteString("Fix options:\n")
		b.WriteString("  1. Deploy source + requirements.txt WITHOUT venv or site-packages\n")
		b.WriteString("     The platform runs pip install inside the linux/amd64 container.\n")
		b.WriteString("  2. Use Linux wheels: pip install --platform manylinux2014_x86_64 --only-binary=:all: -r requirements.txt -t ./deps\n")
	}

	return fmt.Errorf("%s", b.String())
}
