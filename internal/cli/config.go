package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/junikimm717/dev106/internal/shared"
	"github.com/junikimm717/dev106/internal/tui"
	"github.com/opencontainers/image-spec/specs-go/v1"
)

type DevConfig struct {
	Telerun    bool   `toml:"telerun"`
	Image      string `toml:"image"`
	FollowHost *bool  `toml:"follow_host"`
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

func defaultConfigContents(course courseOption) string {
	return fmt.Sprintf(`# dev106 configuration
# Required:
image = %q

# Optional (defaults to true):
telerun = true

# Use the host architecture instead of forcing linux/amd64.
# Enabled by default for 6.181; disabled for 6.106.
follow_host = %t
`, course.Image, course.FollowHost)
}

func (c *DevConfig) followHost() bool {
	if c.FollowHost != nil {
		return *c.FollowHost
	}
	return strings.Contains(c.Image, "6181")
}

func (c *DevConfig) linuxPlatform() v1.Platform {
	arch := "amd64"
	if c.followHost() {
		arch = runtime.GOARCH
	}
	return v1.Platform{
		OS:           "linux",
		Architecture: arch,
	}
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
		course, err := resolveCourse()
		if err != nil {
			return nil, err
		}

		if err := os.WriteFile(path, []byte(defaultConfigContents(course)), 0644); err != nil {
			return nil, err
		}

		fmt.Printf("Created config at %s\n", path)
		fmt.Printf("Using %s image %s\n", course.Name, course.Image)
		if !tui.HasTTY() {
			fmt.Println("No TTY available; defaulted to 6.181.")
		}
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

