//go:build linux

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// Overridden by tests to point at a fake device tree.
var (
	sysfsUSBDir = "/sys/bus/usb/devices"
	groupFile   = "/etc/group"
	devDir      = "/dev"
)

func nodeGID(path string) (int, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Gid), true
}

func sysfsField(dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// findBoard walks sysfs for the FT2232H and turns its bus/device numbers into
// the /dev/bus/usb node. Reading sysfs rather than shelling out to lsusb keeps
// this working in a stripped-down host.
func findBoard() string {
	entries, err := os.ReadDir(sysfsUSBDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		dir := filepath.Join(sysfsUSBDir, entry.Name())
		if sysfsField(dir, "idVendor") != fpgaVendorID || sysfsField(dir, "idProduct") != fpgaProductID {
			continue
		}
		// These are decimal in sysfs but zero padded in /dev, and busnum can
		// carry a leading zero, so parse rather than concatenate.
		bus, err := strconv.Atoi(sysfsField(dir, "busnum"))
		if err != nil {
			continue
		}
		dev, err := strconv.Atoi(sysfsField(dir, "devnum"))
		if err != nil {
			continue
		}
		node := fmt.Sprintf("%s/%03d/%03d", usbBusDir, bus, dev)
		if _, err := os.Stat(node); err == nil {
			return node
		}
	}
	return ""
}

func serialNodes() []string {
	var out []string
	for _, pattern := range []string{"ttyUSB*", "ttyACM*"} {
		matches, err := filepath.Glob(filepath.Join(devDir, pattern))
		if err != nil {
			continue
		}
		out = append(out, matches...)
	}
	sort.Strings(out)
	return out
}

// DetectUSB inspects the host for the FPGA board and any serial adapters.
func DetectUSB() USBDevices {
	u := USBDevices{Supported: true}

	if info, err := os.Stat(usbBusDir); err == nil && info.IsDir() {
		u.BusDir = true
	}

	gids := map[int]bool{}

	if node := findBoard(); node != "" {
		u.BoardNode = node
		if gid, ok := nodeGID(node); ok {
			u.BoardGID = gid
			gids[gid] = true
		}
	}

	for _, tty := range serialNodes() {
		u.Serial = append(u.Serial, tty)
		if gid, ok := nodeGID(tty); ok {
			gids[gid] = true
		}
	}

	// Without a board we still join plugdev, so a board plugged in later is
	// readable through the bind mounted bus without recreating the container.
	if !u.BoardFound() {
		if gid, ok := plugdevGID(); ok {
			gids[gid] = true
		}
	}

	// Root is already implied and adding it would be a privilege grant, not a
	// convenience.
	delete(gids, 0)
	for gid := range gids {
		u.GroupIDs = append(u.GroupIDs, strconv.Itoa(gid))
	}
	sort.Strings(u.GroupIDs)

	return u
}

func plugdevGID() (int, bool) {
	b, err := os.ReadFile(groupFile)
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 3 || fields[0] != "plugdev" {
			continue
		}
		gid, err := strconv.Atoi(fields[2])
		if err != nil {
			return 0, false
		}
		return gid, true
	}
	return 0, false
}

// USBWarning returns the advice to print for the current host, or "".
func USBWarning(u USBDevices) string {
	return usbAdvice(u, IsWSL(), "this host")
}
