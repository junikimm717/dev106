package cli

import (
	"fmt"
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

// workspaceAdvice keys off the filesystem type rather than the WSL detection,
// which only picks wording: a Windows drive is 9p or virtiofs regardless of
// what /proc/version claims, and the Linux VHD is ext4 regardless.
func workspaceAdvice(fsType int64, wsl bool, root string) string {
	windowsDrive := fsType == magic9P ||
		(fsType == magicFUSE && wsl && strings.HasPrefix(root, "/mnt/"))

	if windowsDrive {
		transport := "9p"
		if fsType == magicFUSE {
			transport = "virtiofs"
		}
		return fmt.Sprintf(`warning: %s is on a Windows drive, not your Linux filesystem.

  WSL reaches Windows files over %s, so every open and stat becomes a
  round trip out of the VM. Builds are many times slower, chown does not
  stick (so dev106 cannot match your UID/GID), and file watching is dead.

  Clone into your Linux home instead:
      cd ~ && git clone <url>

`, root, transport)
	}

	switch fsType {
	case magicNFS, magicCIFS, magicSMB2:
		return fmt.Sprintf(`warning: %s is on a network filesystem.

  Builds will be slow and file ownership may not survive, so dev106 may not
  be able to match your UID/GID. A local disk is strongly preferred.

`, root)
	}

	return ""
}
