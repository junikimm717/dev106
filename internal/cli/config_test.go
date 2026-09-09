package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFollowHostInference(t *testing.T) {
	on := true
	off := false

	cases := []struct {
		name string
		cfg  DevConfig
		want bool
	}{
		{"6181 image infers on", DevConfig{Image: "ghcr.io/x/nvim_6181:latest"}, true},
		{"6106 image infers off", DevConfig{Image: "ghcr.io/x/nvim:2.1.0"}, false},
		{"explicit false wins", DevConfig{Image: "ghcr.io/x/nvim_6181:latest", FollowHost: &off}, false},
		{"explicit true wins", DevConfig{Image: "ghcr.io/x/nvim:2.1.0", FollowHost: &on}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.followHost(); got != tc.want {
				t.Fatalf("followHost() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLinuxPlatform(t *testing.T) {
	off := false
	on := true

	forcedCfg := DevConfig{Image: "x", FollowHost: &off}
	forced := forcedCfg.linuxPlatform()
	if forced.OS != "linux" || forced.Architecture != "amd64" {
		t.Fatalf("forced platform = %+v", forced)
	}

	hostCfg := DevConfig{Image: "x", FollowHost: &on}
	host := hostCfg.linuxPlatform()
	if host.OS != "linux" || host.Architecture != runtime.GOARCH {
		t.Fatalf("host platform = %+v, GOARCH = %s", host, runtime.GOARCH)
	}
}

func TestDefaultConfigContents(t *testing.T) {
	got := defaultConfigContents(courseOptions[0])
	if !strings.Contains(got, `follow_host = true`) {
		t.Fatalf("6.181 config should enable follow_host:\n%s", got)
	}
	if !strings.Contains(got, `telerun = false`) {
		t.Fatalf("6.181 config should disable telerun:\n%s", got)
	}
	if !strings.Contains(got, courseOptions[0].Image) {
		t.Fatalf("6.181 config missing image:\n%s", got)
	}

	got = defaultConfigContents(courseOptions[1])
	if !strings.Contains(got, `follow_host = false`) {
		t.Fatalf("6.106 config should disable follow_host:\n%s", got)
	}
	if !strings.Contains(got, `telerun = true`) {
		t.Fatalf("6.106 config should enable telerun:\n%s", got)
	}
}

func TestLoadConfigCreatesAndContinues(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Image == "" {
		t.Fatal("expected generated image")
	}

	path := filepath.Join(dir, "dev106", "config.toml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "follow_host") {
		t.Fatalf("generated config missing follow_host:\n%s", raw)
	}

	// Second load must not rewrite or fail.
	again, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if again.Image != cfg.Image {
		t.Fatalf("second load image %q != %q", again.Image, cfg.Image)
	}
}

func TestLoadConfigReportsPathOnBadTOML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "dev106", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("image = [\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error should include path, got %v", err)
	}
}

func TestLoadConfigEmptyImage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "dev106", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("telerun = true\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected empty image error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error should include path, got %v", err)
	}
}
