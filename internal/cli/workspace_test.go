package cli

import (
	"strings"
	"testing"
)

const (
	testMagicExt4  = 0xEF53
	testMagicBtrfs = 0x9123683E
)

func TestWorkspaceAdvice(t *testing.T) {
	cases := []struct {
		name   string
		fsType int64
		wsl    bool
		root   string
		want   string // substring, "" means no warning at all
	}{
		{"9p windows drive", magic9P, true, "/mnt/c/Users/x/repo", "Windows drive"},
		{"9p names the transport", magic9P, true, "/mnt/c/Users/x/repo", "over 9p"},
		{"virtiofs windows drive", magicFUSE, true, "/mnt/c/Users/x/repo", "virtiofs"},
		{"ext4 linux home is fine", testMagicExt4, true, "/home/x/repo", ""},
		{"btrfs linux home is fine", testMagicBtrfs, true, "/home/x/repo", ""},
		{"nfs warns about network", magicNFS, false, "/net/home/x/repo", "network filesystem"},
		{"cifs warns about network", magicCIFS, false, "/mnt/share/repo", "network filesystem"},
		{"smb2 warns about network", magicSMB2, false, "/mnt/share/repo", "network filesystem"},

		// 9p is only ever a Windows drive, so it must warn even when every
		// WSL heuristic says no. This is the case the statfs check exists for.
		{"9p warns even when WSL detection fails", magic9P, false, "/mnt/c/repo", "Windows drive"},

		// Conversely, ext4 must stay silent on a machine that looks like WSL.
		{"ext4 stays quiet under wsl", testMagicExt4, true, "/mnt/c/repo", ""},

		// Plain FUSE off /mnt (sshfs, gocryptfs) is not our problem.
		{"fuse outside /mnt is not flagged", magicFUSE, true, "/home/x/repo", ""},
		{"fuse without wsl is not flagged", magicFUSE, false, "/mnt/c/repo", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := workspaceAdvice(tc.fsType, tc.wsl, tc.root)
			if tc.want == "" {
				if got != "" {
					t.Fatalf("expected no warning, got:\n%s", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Fatalf("warning missing %q:\n%s", tc.want, got)
			}
			if !strings.Contains(got, tc.root) {
				t.Fatalf("warning should name the path %q:\n%s", tc.root, got)
			}
		})
	}
}
