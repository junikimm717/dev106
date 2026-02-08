package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/junikimm717/dev106/internal/shared"
)

type DevConfig struct {
	Telerun bool   `toml:"telerun"`
	Image   string `toml:"image"`
}

func configDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, shared.APPNAME), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".config", shared.APPNAME), nil
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// TODO: update the default image!
func defaultConfigContents() string {
	return `# dev106 configuration
# Required:
image = "ghcr.io/junikimm717/dev106/nvim"

# Optional (defaults to true):
telerun = true
`
}

func LoadConfig() (*DevConfig, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	// If missing → create + exit
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, []byte(defaultConfigContents()), 0644); err != nil {
			return nil, err
		}

		fmt.Printf("Created config at %s\n", path)
		fmt.Println("Please edit it and re-run dev106.")
		os.Exit(0)
	}

	cfg := &DevConfig{
		Telerun: true, // default
	}

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if cfg.Image == "" {
		return nil, errors.New("config: image is required")
	}

	return cfg, nil
}

