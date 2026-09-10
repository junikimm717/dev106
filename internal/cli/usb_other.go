//go:build !linux

package cli

import "runtime"

// Docker Desktop and OrbStack run the daemon inside a Linux VM that has no
// path to the host's USB bus, so there is nothing to detect and nothing to
// pass through.
func DetectUSB() USBDevices {
	return USBDevices{Supported: false}
}

func USBWarning(u USBDevices) string {
	platform := "Docker on " + runtime.GOOS
	if runtime.GOOS == "darwin" {
		platform = "Docker on macOS"
	}
	return usbAdvice(u, false, platform)
}
