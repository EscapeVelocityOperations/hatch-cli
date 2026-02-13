package configure

import (
	"os"
	"strings"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/config"
)

func setupTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

type mockDeps struct {
	readInput func(prompt string) (string, error)
	saveCfg   func(cfg *config.Config) error
	loadCfg   func() (*config.Config, error)
}

func setMockDeps(d *mockDeps) func() {
	oldReadInput := readInputFn
	oldSaveCfg := saveCfgFn
	oldLoadCfg := loadCfgFn

	if d.readInput != nil {
		readInputFn = d.readInput
	}
	if d.saveCfg != nil {
		saveCfgFn = d.saveCfg
	}
	if d.loadCfg != nil {
		loadCfgFn = d.loadCfg
	}

	return func() {
		readInputFn = oldReadInput
		saveCfgFn = oldSaveCfg
		loadCfgFn = oldLoadCfg
	}
}

func TestRunConfigure_Success(t *testing.T) {
	setupTestConfig(t)
	var savedCfg *config.Config

	restore := setMockDeps(&mockDeps{
		readInput: func(prompt string) (string, error) {
			return "hatch_test_token_123", nil
		},
		saveCfg: func(cfg *config.Config) error {
			savedCfg = cfg
			return config.Save(cfg)
		},
		loadCfg: func() (*config.Config, error) {
			return &config.Config{}, nil
		},
	})
	defer restore()

	cmd := NewCmd()
	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if savedCfg == nil {
		t.Fatal("config was not saved")
	}
	if savedCfg.Token != "hatch_test_token_123" {
		t.Errorf("saved token = %q, want %q", savedCfg.Token, "hatch_test_token_123")
	}
}

func TestRunConfigure_EmptyToken(t *testing.T) {
	setupTestConfig(t)

	restore := setMockDeps(&mockDeps{
		readInput: func(prompt string) (string, error) {
			return "", nil
		},
		loadCfg: func() (*config.Config, error) {
			return &config.Config{}, nil
		},
	})
	defer restore()

	cmd := NewCmd()
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for empty token")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRunConfigure_InvalidTokenFormat(t *testing.T) {
	setupTestConfig(t)

	tests := []struct {
		name  string
		token string
	}{
		{"missing prefix", "invalid_token_123"},
		{"wrong prefix", "tok_123"},
		{"just random text", "randomstring"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore := setMockDeps(&mockDeps{
				readInput: func(prompt string) (string, error) {
					return tt.token, nil
				},
				loadCfg: func() (*config.Config, error) {
					return &config.Config{}, nil
				},
			})
			defer restore()

			cmd := NewCmd()
			err := cmd.RunE(cmd, nil)
			if err == nil {
				t.Fatal("expected error for invalid token format")
			}
			if !strings.Contains(err.Error(), "invalid token format") {
				t.Errorf("unexpected error message: %v", err)
			}
		})
	}
}

func TestRunConfigure_ValidTokenFormats(t *testing.T) {
	setupTestConfig(t)

	validTokens := []string{
		"hatch_abc123",
		"hatch_" + strings.Repeat("a", 50),
		"hatch_token-with_underscores",
	}

	for _, token := range validTokens {
		t.Run("token_"+token[:10], func(t *testing.T) {
			var savedCfg *config.Config

			restore := setMockDeps(&mockDeps{
				readInput: func(prompt string) (string, error) {
					return token, nil
				},
				saveCfg: func(cfg *config.Config) error {
					savedCfg = cfg
					return config.Save(cfg)
				},
				loadCfg: func() (*config.Config, error) {
					return &config.Config{}, nil
				},
			})
			defer restore()

			cmd := NewCmd()
			err := cmd.RunE(cmd, nil)
			if err != nil {
				t.Fatalf("unexpected error for valid token %q: %v", token, err)
			}

			if savedCfg.Token != token {
				t.Errorf("saved token = %q, want %q", savedCfg.Token, token)
			}
		})
	}
}

func TestRunConfigure_ReadInputError(t *testing.T) {
	setupTestConfig(t)

	restore := setMockDeps(&mockDeps{
		readInput: func(prompt string) (string, error) {
			return "", os.ErrClosed
		},
		loadCfg: func() (*config.Config, error) {
			return &config.Config{}, nil
		},
	})
	defer restore()

	cmd := NewCmd()
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error from read input")
	}
}

func TestRunConfigure_SaveConfigError(t *testing.T) {
	setupTestConfig(t)

	restore := setMockDeps(&mockDeps{
		readInput: func(prompt string) (string, error) {
			return "hatch_test_token", nil
		},
		saveCfg: func(cfg *config.Config) error {
			return os.ErrPermission
		},
		loadCfg: func() (*config.Config, error) {
			return &config.Config{}, nil
		},
	})
	defer restore()

	cmd := NewCmd()
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error from save config")
	}
}

func TestRunConfigure_PreservesExistingConfig(t *testing.T) {
	setupTestConfig(t)
	var savedCfg *config.Config

	// Pre-populate config with API host
	restore := setMockDeps(&mockDeps{
		readInput: func(prompt string) (string, error) {
			return "hatch_new_token", nil
		},
		saveCfg: func(cfg *config.Config) error {
			savedCfg = cfg
			return config.Save(cfg)
		},
		loadCfg: func() (*config.Config, error) {
			return &config.Config{
				APIHost: "https://custom.api.example.com",
			}, nil
		},
	})
	defer restore()

	cmd := NewCmd()
	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if savedCfg.Token != "hatch_new_token" {
		t.Errorf("saved token = %q, want %q", savedCfg.Token, "hatch_new_token")
	}
	if savedCfg.APIHost != "https://custom.api.example.com" {
		t.Errorf("API host not preserved, got %q", savedCfg.APIHost)
	}
}

func TestRunConfigure_LoadConfigError(t *testing.T) {
	setupTestConfig(t)

	restore := setMockDeps(&mockDeps{
		readInput: func(prompt string) (string, error) {
			return "hatch_test_token", nil
		},
		saveCfg: func(cfg *config.Config) error {
			return config.Save(cfg)
		},
		loadCfg: func() (*config.Config, error) {
			return nil, os.ErrNotExist
		},
	})
	defer restore()

	cmd := NewCmd()
	err := cmd.RunE(cmd, nil)
	// Should succeed - load error creates empty config
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewCmd(t *testing.T) {
	cmd := NewCmd()

	if cmd.Use != "configure" {
		t.Errorf("Use = %q, want %q", cmd.Use, "configure")
	}
	if cmd.Short == "" {
		t.Error("expected non-empty Short description")
	}
	if cmd.Long == "" {
		t.Error("expected non-empty Long description")
	}
}

func TestRunConfigure_TrimWhitespace(t *testing.T) {
	setupTestConfig(t)
	var savedCfg *config.Config

	restore := setMockDeps(&mockDeps{
		readInput: func(prompt string) (string, error) {
			return "  hatch_test_token  \n", nil
		},
		saveCfg: func(cfg *config.Config) error {
			savedCfg = cfg
			return config.Save(cfg)
		},
		loadCfg: func() (*config.Config, error) {
			return &config.Config{}, nil
		},
	})
	defer restore()

	cmd := NewCmd()
	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if savedCfg.Token != "hatch_test_token" {
		t.Errorf("saved token = %q, want %q (trimmed)", savedCfg.Token, "hatch_test_token")
	}
}
