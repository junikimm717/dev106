//go:build !linux

package host

// The 9p/virtiofs Windows-drive problem is specific to WSL.

func IsWSL() bool { return false }

func WorkspaceWarning(root string) string { return "" }
