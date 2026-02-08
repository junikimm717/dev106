package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"

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
		if stat != nil && stat.IsDir() {
			return realpath, nil
		}
		parent := filepath.Dir(realpath)
		if parent == realpath {
			return "", errors.New("Could not find git repository root!")
		}
		realpath = parent
	}
}

func ContainerName(dir string) string {
	u, err := user.Current()
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256([]byte(dir))
	id := hex.EncodeToString(sum[:])[:12]
	return fmt.Sprintf("%s_%s_%s", shared.CONTAINER_PREFIX, u.Username, id)
}

// compute the bind mounts that we'll need for a container.
func BindMounts(config *DevConfig, dir string) ([]string, error) {
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
	res = append(res, fmt.Sprintf("%s:%s:rw", dir, "/workspace"))

	// telerun credentials should be synced.
	telerun := filepath.Join(home, ".telerun")
	if config.Telerun {
		err := os.MkdirAll(telerun, 0o755)
		if err != nil {
			return res, err
		}
		res = append(res, fmt.Sprintf("%s:%s/.telerun:rw", telerun, shared.CONTAINER_HOME))
	}

	return res, nil
}
