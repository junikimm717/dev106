package config

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

// RepoConfigName layers over the global config, so a 6.205 checkout can
// differ from the 6.181 default without either being edited.
const RepoConfigName = ".dev106.toml"

type DevConfig struct {
	Telerun    bool   `toml:"telerun"`
	Image      string `toml:"image"`
	FollowHost *bool  `toml:"follow_host"`
	LabBC      bool   `toml:"labbc"`
	USB        bool   `toml:"usb"`

	// USBIDs overrides the built-in programmer list, as "vid:pid" strings.
	// Empty means the defaults.
	USBIDs []string `toml:"usb_ids"`

	// USBLabel names the hardware in warnings. Setting it means the device
	// is not an FPGA board, so the openFPGALoader-specific advice is dropped.
	USBLabel string `toml:"usb_label"`

	// Where the values came from, for `dev106 config`.
	GlobalPath string `toml:"-"`
	RepoPath   string `toml:"-"`
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

# Sync ~/.telerun into the container. On for 6.106; off elsewhere.
telerun = %t

# Use the host architecture instead of forcing linux/amd64.
# Enabled by default for 6.181 and 6.205; disabled for 6.106.
follow_host = %t

# Persist lab-bc credentials on the host, so "lab-bc configure" is a
# one-time step rather than once per container. 6.205 only.
labbc = %t

# Pass the FPGA board through to the container for flashing and UART.
# 6.205 only. Works on Linux, WSL2, and OrbStack; Docker Desktop for Mac
# has no direct USB passthrough.
usb = %t

# Taking more than one class? Drop a %s at the root of a repo to
# override any of these for that repo alone:
#
#     image = "ghcr.io/junikimm717/dev106/mit_6205:latest"
#     labbc = true
#     usb = true
`, course.Image, course.Telerun, course.FollowHost, course.LabBC, course.USB, RepoConfigName)
}

func (c *DevConfig) followHost() bool {
	if c.FollowHost != nil {
		return *c.FollowHost
	}
	return strings.Contains(c.Image, "6181") || strings.Contains(c.Image, "6205")
}

func (c *DevConfig) LinuxPlatform() v1.Platform {
	arch := "amd64"
	if c.followHost() {
		arch = runtime.GOARCH
	}
	return v1.Platform{
		OS:           "linux",
		Architecture: arch,
	}
}

// Load reads the global config, then layers RepoConfigName over it. A
// key absent from the repo file keeps its global value. Pass "" to skip.
func Load(root string) (*DevConfig, error) {
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
		fmt.Printf("Using %s image %s (follow_host=%t)\n", course.Name, course.Image, course.FollowHost)
		if !tui.HasTTY() {
			fmt.Println("No TTY available; defaulted to 6.181.")
		}
		fmt.Println()
		fmt.Println("Get started:")
		fmt.Println("  1. cd into a course assignment repo")
		fmt.Println("  2. dev106 pull")
		fmt.Println("  3. dev106")
		fmt.Println("Edit the config anytime to change image, telerun, or follow_host.")
	}

	cfg := &DevConfig{
		Telerun:    true, // default
		GlobalPath: path,
	}

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config %s: %w", path, err)
	}

	if root != "" {
		repoPath := filepath.Join(root, RepoConfigName)
		if _, err := os.Stat(repoPath); err == nil {
			if _, err := toml.DecodeFile(repoPath, cfg); err != nil {
				return nil, fmt.Errorf("failed to parse %s: %w", repoPath, err)
			}
			cfg.RepoPath = repoPath
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	if cfg.Image == "" {
		return nil, fmt.Errorf("config: image is required in %s", path)
	}

	return cfg, nil
}
