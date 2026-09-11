package cli

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/moby/moby/api/types/container"
)

// The Urbana board's FTDI FT2232H.
const (
	fpgaVendorID  = "0403"
	fpgaProductID = "6010"
)

// Whole USB major, so a replug does not need a new container.
const usbCgroupRule = "c 189:* rmw"

// Overridden by tests to point at a fake device tree.
var usbBusDir = "/dev/bus/usb"

// DaemonUSBMode is a property of the daemon, not of our GOOS: the devices a
// container sees are the daemon's.
type DaemonUSBMode int

const (
	// No path. Docker Desktop reaches USB only over USB/IP, which needs a
	// privileged helper container and does not present /dev/bus/usb.
	USBUnavailable DaemonUSBMode = iota

	// The daemon shares our /dev, so a host scan is meaningful.
	USBHostDevices

	// The daemon's own VM has a real USB controller (OrbStack). The bind is
	// still right, since the daemon resolves it; our sysfs is not.
	USBSharedVM
)

// DaemonIdentity is the part of `docker info` that bears on USB.
type DaemonIdentity struct {
	OperatingSystem string
	KernelVersion   string
	Remote          bool
}

func (d DaemonIdentity) isOrbStack() bool {
	return containsFold(d.OperatingSystem, "orbstack") || containsFold(d.KernelVersion, "orbstack")
}

func (d DaemonIdentity) isDockerDesktop() bool {
	return containsFold(d.OperatingSystem, "docker desktop") ||
		containsFold(d.KernelVersion, "linuxkit")
}

func daemonUSBMode(d DaemonIdentity, clientLinux bool) DaemonUSBMode {
	if d.Remote {
		return USBUnavailable
	}
	if d.isOrbStack() {
		return USBSharedVM
	}
	if d.isDockerDesktop() {
		// WSL2 distros share one kernel, so a usbipd-attached device reaches
		// the docker-desktop distro too.
		if clientLinux {
			return USBHostDevices
		}
		return USBUnavailable
	}
	if clientLinux {
		return USBHostDevices
	}
	return USBUnavailable
}

// USBDevices is what the daemon can offer the container.
type USBDevices struct {
	Mode      DaemonUSBMode
	Identity  DaemonIdentity
	Supported bool
	BusDir    bool

	// BoardNode is the /dev/bus/usb/BBB/DDD node, empty when unplugged.
	BoardNode string
	// BoardGID 0 means root-only, which the container user cannot open.
	BoardGID int
	Serial   []string
	// GroupIDs own the nodes above, minus root.
	GroupIDs []string
}

func (u USBDevices) BoardFound() bool { return u.BoardNode != "" }

// Enumerable reports whether a host scan describes the container's devices.
// It does not on a shared-VM daemon, so a quiet scan is not "no board".
func (u USBDevices) Enumerable() bool { return u.Mode == USBHostDevices }

// BoardRootOnly means the udev rule is missing.
func (u USBDevices) BoardRootOnly() bool { return u.BoardFound() && u.BoardGID == 0 }

// usbAdvice explains why flashing will not work, or "" when the board is
// ready. Only flashing is ever blocked, so these stay warnings.
func usbAdvice(u USBDevices, wsl bool, platform string) string {
	if !u.Supported {
		return unavailableAdvice(u, platform)
	}

	// We cannot see the VM's devices, so this is a pointer, not a diagnosis.
	if u.Mode == USBSharedVM {
		return `note: OrbStack passes USB through, but the board has to be attached first.

  Check that it is, and attach it if not:
      orb usb list
      orb usb attach <id>

  Attached devices are visible to every container, so this is a one-time
  step per plug-in. Serial adapters are forwarded automatically.

`
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

func unavailableAdvice(u USBDevices, platform string) string {
	switch {
	case u.Identity.Remote:
		return `warning: your Docker daemon is not on this machine, so it cannot see
  a board plugged in here.

  Simulation and lab-bc build work normally. For flashing, the board has to
  be plugged into whatever machine runs the daemon.

`
	case u.Identity.isDockerDesktop():
		return `warning: Docker Desktop cannot pass USB devices into containers directly.

  Simulation (iverilog, cocotb) and remote builds (lab-bc) work normally.
  Flashing and UART do not: Docker Desktop reaches USB only over USB/IP,
  which needs a privileged helper container held open for the session and
  does not present the board at /dev/bus/usb. dev106 does not drive that.

  Options, roughly in order of how much trouble they are:
    * Use OrbStack instead, which passes USB through natively.
    * Flash from a Linux host or VM after building the bitstream here.

`
	default:
		return fmt.Sprintf(`warning: %s cannot pass USB devices into containers.

  Simulation (iverilog, cocotb) and remote builds (lab-bc) work normally.
  Flashing the board and reading its UART do not.

  Build the bitstream here, then flash from a Linux host or VM:
      openFPGALoader -b urbana build/obj/final.bit

`, platform)
	}
}

// staleContainerAdvice fires when the board showed up after creation, since
// the device list is fixed then.
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

func containerHasUSBPassthrough(binds []string) bool {
	for _, bind := range binds {
		if strings.HasPrefix(bind, usbBusDir+":") {
			return true
		}
	}
	return false
}

// DetectUSB reports what the daemon can hand a container.
func DetectUSB(mode DaemonUSBMode, id DaemonIdentity) USBDevices {
	u := USBDevices{Mode: mode, Identity: id, Supported: mode != USBUnavailable}
	switch mode {
	case USBHostDevices:
		scanHostUSB(&u)
	case USBSharedVM:
		// Cannot stat the VM's bus from here; the daemon resolves the bind.
		u.BusDir = true
		u.Serial = orbSerialPorts()
	}
	return u
}

func USBWarning(u USBDevices) string {
	return usbAdvice(u, IsWSL(), platformName())
}

func platformName() string {
	switch runtime.GOOS {
	case "darwin":
		return "Docker on macOS"
	case "windows":
		return "Docker on Windows"
	default:
		return "this host"
	}
}

// orbSerialPorts lists the macOS serial ports OrbStack forwards into the VM.
// They keep their macOS names there, so the ttyUSB*/ttyACM* glob misses them.
func orbSerialPorts() []string {
	orb, err := exec.LookPath("orb")
	if err != nil {
		return nil
	}
	out, err := exec.Command(orb, "serial", "list").Output()
	if err != nil {
		return nil
	}

	var ports []string
	for _, line := range strings.Split(string(out), "\n") {
		// name, type, path. "callout" (/dev/cu.*) is the same port twice.
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[1] != "tty" {
			continue
		}
		path := fields[2]
		if !containsFold(path, "usbserial") && !containsFold(path, "usbmodem") {
			continue
		}
		ports = append(ports, path)
	}
	return ports
}

// applyUSB binds the whole bus rather than the board node, so a replug (which
// changes the device number) does not need a new container.
func applyUSB(hostConfig *container.HostConfig, u USBDevices) {
	if !u.Supported || !u.BusDir {
		return
	}

	hostConfig.Binds = append(hostConfig.Binds, usbBusDir+":"+usbBusDir)
	hostConfig.DeviceCgroupRules = append(hostConfig.DeviceCgroupRules, usbCgroupRule)

	// Serial nodes sit outside /dev/bus/usb, so they need mapping in.
	for _, tty := range u.Serial {
		hostConfig.Devices = append(hostConfig.Devices, container.DeviceMapping{
			PathOnHost:        tty,
			PathInContainer:   tty,
			CgroupPermissions: "rwm",
		})
	}

	hostConfig.GroupAdd = append(hostConfig.GroupAdd, u.GroupIDs...)
}
