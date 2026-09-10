package cli

import (
	"fmt"
	"strings"

	"github.com/moby/moby/api/types/container"
)

// The Urbana board's FTDI FT2232H. run_6205.sh matches the same pair.
const (
	fpgaVendorID  = "0403"
	fpgaProductID = "6010"
)

// usbMajor is the device-node major for /dev/bus/usb. The container gets a
// cgroup rule for the whole major rather than a rule per device, so flashing
// survives a power cycle or a replug without recreating the container.
const usbCgroupRule = "c 189:* rmw"

// Overridden by tests to point at a fake device tree.
var usbBusDir = "/dev/bus/usb"

// USBDevices is what the host can offer the container. Everything is empty on
// a host that cannot pass USB through at all.
type USBDevices struct {
	// Supported is false where the daemon has no access to host USB, i.e.
	// Docker Desktop's VM on macOS and Windows.
	Supported bool

	// BusDir is true when /dev/bus/usb exists and can be bind mounted.
	BusDir bool

	// BoardNode is the /dev/bus/usb/BBB/DDD node of the FPGA, empty when no
	// board is plugged in.
	BoardNode string

	// BoardGID owns BoardNode. Zero means root-only, which openFPGALoader
	// cannot open as the unprivileged container user.
	BoardGID int

	// Serial holds /dev/ttyUSB* and /dev/ttyACM* nodes for the UART labs.
	Serial []string

	// GroupIDs are the host groups owning the nodes above, minus root.
	GroupIDs []string
}

func (u USBDevices) BoardFound() bool { return u.BoardNode != "" }

// BoardRootOnly reports a board that is present but owned by root, which is
// what you get on a host missing the openFPGALoader udev rule.
func (u USBDevices) BoardRootOnly() bool { return u.BoardFound() && u.BoardGID == 0 }

// usbAdvice explains why flashing will not work, or returns "" when the board
// is ready. Simulation and `lab-bc build` never need USB, so this stays a
// warning: it is only the flashing half of the course that is blocked.
func usbAdvice(u USBDevices, wsl bool, platform string) string {
	if !u.Supported {
		return fmt.Sprintf(`warning: %s cannot pass USB devices into containers.

  Simulation (iverilog, cocotb) and remote builds (lab-bc) work normally.
  Flashing the board and reading its UART do not, and there is no setting
  that changes this: the daemon runs in a VM with no access to your USB bus.

  Build the bitstream here, then flash from a Linux host or VM:
      openFPGALoader -b urbana build/obj/final.bit

`, platform)
	}

	if u.BoardRootOnly() {
		return fmt.Sprintf(`warning: %s is owned by root, so openFPGALoader cannot open it.

  Install the udev rule on the host and replug the board:
      sudo curl -fsSL -o /etc/udev/rules.d/99-openfpgaloader.rules \
        https://raw.githubusercontent.com/trabucayre/openFPGALoader/master/99-openfpgaloader.rules
      sudo udevadm control --reload-rules && sudo udevadm trigger

`, u.BoardNode)
	}

	if !u.BoardFound() {
		fix := "  Plug the board in, then run `dev106 restart`.\n"
		if wsl {
			fix = `  WSL2 does not see USB devices until you attach them from Windows.
  In an admin PowerShell:
      usbipd list
      usbipd bind   --hardware-id 0403:6010
      usbipd attach --wsl --hardware-id 0403:6010
  Then run ` + "`dev106 restart`" + `.
`
		}
		return fmt.Sprintf(`warning: no FPGA board (%s:%s) is visible to dev106.

  Simulation and `+"`lab-bc build`"+` work fine. Flashing will not.

%s
`, fpgaVendorID, fpgaProductID, fix)
	}

	return ""
}

// staleContainerAdvice fires when the board showed up after the container was
// created. The device list is fixed at creation, so attaching to the old
// container silently has no board in it.
func staleContainerAdvice(u USBDevices, containerHasUSB bool) string {
	if !u.Supported || !u.BoardFound() || containerHasUSB {
		return ""
	}
	return `warning: this container was created without the FPGA board attached.

  The board is plugged in now, but a container's devices are fixed when it
  is created. Pick it up with:
      dev106 restart

`
}

// containerHasUSBPassthrough reports whether an existing container was created
// with the USB bus mounted.
func containerHasUSBPassthrough(binds []string) bool {
	for _, bind := range binds {
		if strings.HasPrefix(bind, usbBusDir+":") {
			return true
		}
	}
	return false
}

// applyUSB gives the container the host's USB bus. The whole bus is bind
// mounted rather than the single board node so that replugging the board,
// which changes its device number, does not need a new container; the cgroup
// rule is what makes the new node usable when that happens.
func applyUSB(hostConfig *container.HostConfig, u USBDevices) {
	if !u.Supported || !u.BusDir {
		return
	}

	hostConfig.Binds = append(hostConfig.Binds, usbBusDir+":"+usbBusDir)
	hostConfig.DeviceCgroupRules = append(hostConfig.DeviceCgroupRules, usbCgroupRule)

	// Serial nodes live outside /dev/bus/usb and are not covered by the bind,
	// so they are mapped individually. They only exist while the board is
	// plugged in, which is why a power cycle needs `dev106 restart`.
	for _, tty := range u.Serial {
		hostConfig.Devices = append(hostConfig.Devices, container.DeviceMapping{
			PathOnHost:        tty,
			PathInContainer:   tty,
			CgroupPermissions: "rwm",
		})
	}

	hostConfig.GroupAdd = append(hostConfig.GroupAdd, u.GroupIDs...)
}
