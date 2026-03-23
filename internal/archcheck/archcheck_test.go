package archcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeBin(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	os.MkdirAll(filepath.Dir(p), 0755)
	if err := os.WriteFile(p, data, 0755); err != nil {
		t.Fatal(err)
	}
	return p
}

func elfAMD64Header() []byte {
	h := make([]byte, 20)
	h[0], h[1], h[2], h[3] = 0x7f, 'E', 'L', 'F'
	h[18], h[19] = 0x3E, 0x00
	return h
}

func elfARM64Header() []byte {
	h := make([]byte, 20)
	h[0], h[1], h[2], h[3] = 0x7f, 'E', 'L', 'F'
	h[18], h[19] = 0xB7, 0x00
	return h
}

func TestDetectArch(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name string
		data []byte
		want ArchType
	}{
		{"macho-be", []byte{0xfe, 0xed, 0xfa, 0xcf, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, ArchMacOS},
		{"macho-le", []byte{0xcf, 0xfa, 0xed, 0xfe, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, ArchMacOS},
		{"macho-32be", []byte{0xfe, 0xed, 0xfa, 0xce, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, ArchMacOS},
		{"macho-32le", []byte{0xce, 0xfa, 0xed, 0xfe, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, ArchMacOS},
		{"elf-amd64", elfAMD64Header(), ArchLinuxAMD64},
		{"elf-arm64", elfARM64Header(), ArchLinuxARM64},
		{"pe-windows", append([]byte{'M', 'Z'}, make([]byte, 18)...), ArchWindows},
		{"script", []byte("#!/bin/bash\necho hi\n"), ArchUnknown},
		{"empty", []byte{}, ArchUnknown},
		{"too-small", []byte{0x7f}, ArchUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := writeBin(t, dir, tt.name, tt.data)
			got := DetectArch(p)
			if got != tt.want {
				t.Errorf("DetectArch(%s) = %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

func TestCheckNativeDeps_AllBadNode(t *testing.T) {
	dir := t.TempDir()
	macho := []byte{0xcf, 0xfa, 0xed, 0xfe, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	writeBin(t, dir, "node_modules/sharp/build/sharp.node", macho)
	writeBin(t, dir, "node_modules/bcrypt/lib/bcrypt.node", macho)

	_, err := CheckNativeDeps(dir, "node")
	if err == nil {
		t.Fatal("expected error for all-bad native deps")
	}
	if !strings.Contains(err.Error(), "wrong platform") {
		t.Errorf("error should mention wrong platform: %v", err)
	}
	if !strings.Contains(err.Error(), "npm rebuild") {
		t.Errorf("error should contain fix instructions: %v", err)
	}
}

func TestCheckNativeDeps_AllGoodNode(t *testing.T) {
	dir := t.TempDir()
	writeBin(t, dir, "node_modules/sharp/build/sharp.node", elfAMD64Header())

	result, err := CheckNativeDeps(dir, "node")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for all-good, got %+v", result)
	}
}

func TestCheckNativeDeps_Mixed(t *testing.T) {
	dir := t.TempDir()
	macho := []byte{0xcf, 0xfa, 0xed, 0xfe, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	writeBin(t, dir, "node_modules/sharp/build/sharp.node", macho)
	writeBin(t, dir, "node_modules/other/lib/other.node", elfAMD64Header())

	result, err := CheckNativeDeps(dir, "node")
	if err != nil {
		t.Fatalf("mixed should not return error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for mixed")
	}
	if len(result.BadFiles) != 1 {
		t.Errorf("expected 1 bad file, got %d", len(result.BadFiles))
	}
}

func TestCheckNativeDeps_NoneFound(t *testing.T) {
	dir := t.TempDir()
	writeBin(t, dir, "index.js", []byte("console.log('hi')"))

	result, err := CheckNativeDeps(dir, "node")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for no native deps, got %+v", result)
	}
}

func TestCheckNativeDeps_SkipGo(t *testing.T) {
	dir := t.TempDir()
	macho := []byte{0xcf, 0xfa, 0xed, 0xfe, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	writeBin(t, dir, "node_modules/sharp/build/sharp.node", macho)

	result, err := CheckNativeDeps(dir, "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("go runtime should skip scan, got %+v", result)
	}
}

func TestCheckNativeDeps_PythonSO(t *testing.T) {
	dir := t.TempDir()
	macho := []byte{0xcf, 0xfa, 0xed, 0xfe, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	writeBin(t, dir, "venv/lib/numpy/core/_multiarray.so", macho)

	_, err := CheckNativeDeps(dir, "python")
	if err == nil {
		t.Fatal("expected error for macOS .so files")
	}
	if !strings.Contains(err.Error(), "pip install") {
		t.Errorf("error should contain python fix instructions: %v", err)
	}
}

func TestCheckNativeDeps_ELFArm64(t *testing.T) {
	dir := t.TempDir()
	writeBin(t, dir, "node_modules/sharp/build/sharp.node", elfARM64Header())

	_, err := CheckNativeDeps(dir, "node")
	if err == nil {
		t.Fatal("expected error for arm64 .node files")
	}
}
