package auth

import (
	"github.com/EscapeVelocityOperations/hatch-cli/internal/config"
)

func IsLoggedIn() (bool, error) {
	cfg, err := config.Load()
	if err != nil {
		return false, err
	}
	return cfg.Token != "", nil
}

func SaveToken(token string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Token = token
	return config.Save(cfg)
}

func GetToken() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return cfg.Token, nil
}

func ClearToken() error {
	return config.ClearToken()
}
