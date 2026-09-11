package host

import (
	"strings"
)

// Magic numbers from linux/magic.h.
const (
	magic9P   = 0x01021997
	magicFUSE = 0x65735546
	magicCIFS = 0xFF534D42
	magicSMB2 = 0xFE534D42
	magicNFS  = 0x6969
)

func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// WSL automounts Windows volumes at /mnt/<drive letter>. Matching the letter
// keeps an unrelated FUSE mount like /mnt/sshfs-host from being reported as a
// Windows drive.
func isWindowsDrivePath(root string) bool {
	rest, ok := strings.CutPrefix(root, "/mnt/")
	if !ok || rest == "" {
		return false
	}
	letter := rest[0] | 0x20
	if letter < 'a' || letter > 'z' {
		return false
	}
	return len(rest) == 1 || rest[1] == '/'
}
