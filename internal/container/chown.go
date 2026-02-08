package container

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func ChownDirs(paths []string, excludePaths []string, uid, gid int) error {
	var errs []error

	// Normalize excludes once
	excludes := make([]string, 0, len(excludePaths))
	for _, p := range excludePaths {
		if p == "" {
			continue
		}
		excludes = append(excludes, filepath.Clean(p))
	}

	isExcluded := func(path string) bool {
		path = filepath.Clean(path)

		for _, ex := range excludes {
			if path == ex {
				return true
			}

			// subtree check
			if strings.HasPrefix(path, ex+string(os.PathSeparator)) {
				return true
			}
		}
		return false
	}

	for _, root := range paths {
		if root == "" {
			continue
		}

		root = filepath.Clean(root)

		if _, err := os.Lstat(root); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			errs = append(errs, fmt.Errorf("%s: %w", root, err))
			continue
		}

		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if isExcluded(path) {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}

			info, err := d.Info()
			if err != nil {
				return err
			}

			if err := os.Lchown(path, uid, gid); err != nil {
				return err
			}

			if info.Mode()&os.ModeSymlink != 0 {
				return fs.SkipDir
			}

			return nil
		})

		if err != nil && !errors.Is(err, fs.SkipDir) {
			errs = append(errs, fmt.Errorf("%s: %w", root, err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
