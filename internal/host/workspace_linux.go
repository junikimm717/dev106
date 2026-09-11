//go:build linux

package host

import (
	"os"
	"syscall"
)

func statfsType(path string) (int64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, false
	}
	// Type is int32 on 32-bit arches, int64 on 64-bit ones.
	return int64(st.Type), true
}

// IsWSL only selects the wording of a warning, never whether to warn, so a
// false positive is harmless. Checks run most to least trustworthy: the
// kernel version string is last because a custom WSL2 kernel drops the
// "microsoft" marker and a native kernel could carry it.
func IsWSL() bool {
	for _, path := range []string{
		"/proc/sys/fs/binfmt_misc/WSLInterop",
		"/proc/sys/fs/binfmt_misc/WSLInterop-late",
		"/run/WSL",
	} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
		return true
	}

	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		if containsFold(string(b), "microsoft") || containsFold(string(b), "wsl") {
			return true
		}
	}
	return false
}

// WorkspaceWarning returns a warning for repo roots on a filesystem dev106
// cannot work properly on, or "" when the path is fine.
func WorkspaceWarning(root string) string {
	fsType, ok := statfsType(root)
	if !ok {
		return ""
	}
	return workspaceAdvice(fsType, IsWSL(), root)
}
