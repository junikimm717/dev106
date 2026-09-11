//go:build !linux

package cli

// Only Linux has host USB devices to scan; DetectUSB never calls this
// otherwise.
func scanHostUSB(u *USBDevices) {}
