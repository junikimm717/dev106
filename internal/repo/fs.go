package repo

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"

	"github.com/junikimm717/dev106/internal/config"
	"github.com/junikimm717/dev106/internal/shared"
)

func FindRoot(dir string) (string, error) {
	abspath, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	realpath, err := filepath.EvalSymlinks(abspath)
	if err != nil {
		return "", err
	}
	for {
		gitpath := filepath.Join(realpath, ".git")
		stat, err := os.Stat(gitpath)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if stat != nil && (stat.IsDir() || stat.Mode().IsRegular()) {
			return realpath, nil
		}
		parent := filepath.Dir(realpath)
		if parent == realpath {
			return "", errors.New("not inside a git repository (dev106 needs a repo root to name the container).\ncd into a clone, or use `dev106 pull` which works anywhere")
		}
		realpath = parent
	}
}

func RootID(dir string) string {
	sum := sha256.Sum256([]byte(dir))
	return hex.EncodeToString(sum[:])[:12]
}

func ContainerName(dir string) (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("could not determine current user: %w", err)
	}
	return fmt.Sprintf("%s_%s_%s", shared.CONTAINER_PREFIX, u.Username, RootID(dir)), nil
}

func ContainerLabels(root string) map[string]string {
	return map[string]string{
		shared.LabelManaged: "true",
		shared.LabelRoot:    root,
	}
}

// compute the bind mounts that we'll need for a container.
func BindMounts(cfg *config.DevConfig, dir string) ([]string, error) {
	res := make([]string, 0, 2)
	// bruh so the home directory should not be something skibidi.
	home, err := os.UserHomeDir()
	if err != nil {
		return res, err
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return res, err
	}
	home, err = filepath.EvalSymlinks(home)
	if err != nil {
		return res, err
	}

	// checks for the repository root.
	if !filepath.IsAbs(dir) {
		return res, fmt.Errorf("%s is not an absolute path!", dir)
	}
	stat, err := os.Stat(dir)
	if err != nil {
		return res, err
	}
	if !stat.IsDir() {
		return res, fmt.Errorf("%s is not a directory!", dir)
	}
	res = append(res, fmt.Sprintf("%s:%s:rw", dir, shared.CONTAINER_WORKSPACE))

	// telerun credentials should be synced.
	telerun := filepath.Join(home, ".telerun")
	if cfg.Telerun {
		err := os.MkdirAll(telerun, 0o755)
		if err != nil {
			return res, err
		}
		res = append(res, fmt.Sprintf("%s:%s/.telerun:rw", telerun, shared.CONTAINER_HOME))
	}

	// Keeping lab-bc's config dir on the host means logging in once.
	if cfg.LabBC {
		labbc := LabBCConfigDir(home)
		if err := os.MkdirAll(labbc, 0o700); err != nil {
			return res, err
		}
		res = append(res, fmt.Sprintf("%s:%s/.config/lab-bc:rw", labbc, shared.CONTAINER_HOME))
	}

	return res, nil
}

// LabBCConfigDir follows XDG_CONFIG_HOME, matching run_6205.sh.
func LabBCConfigDir(home string) string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "lab-bc")
	}
	return filepath.Join(home, ".config", "lab-bc")
}
