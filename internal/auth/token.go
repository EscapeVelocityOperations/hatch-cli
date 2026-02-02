package auth

import (
	"os"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/config"
)

// tokenFlag holds a token set via the --token CLI flag.
var tokenFlag string

// SetTokenFlag sets the token from the --token CLI flag.
func SetTokenFlag(token string) {
	tokenFlag = token
}

func IsLoggedIn() (bool, error) {
	tok, err := GetToken()
	if err != nil {
		return false, err
	}
	return tok != "", nil
}

func SaveToken(token string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Token = token
	return config.Save(cfg)
}

// GetToken returns a token using the following precedence:
//  1. --token CLI flag
//  2. HATCH_TOKEN environment variable
//  3. Token from config file (~/.hatch/config.json)
func GetToken() (string, error) {
	if tokenFlag != "" {
		return tokenFlag, nil
	}
	if envToken := os.Getenv("HATCH_TOKEN"); envToken != "" {
		return envToken, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return cfg.Token, nil
}

func ClearToken() error {
	return config.ClearToken()
}
