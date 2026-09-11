package host

import (
	"os/exec"
	"runtime"
	"strings"
)

// The Urbana board's FTDI FT2232H.
const (
	fpgaVendorID  = "0403"
	fpgaProductID = "6010"
)

// Whole USB major, so a replug does not need a new container.
const USBCgroupRule = "c 189:* rmw"

// Overridden by tests to point at a fake device tree.
var USBBusDir = "/dev/bus/usb"

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

func ContainerHasUSBPassthrough(binds []string) bool {
	for _, bind := range binds {
		if strings.HasPrefix(bind, USBBusDir+":") {
			return true
		}
	}
	return false
}

// Detect resolves how this daemon reaches USB, then probes accordingly.
func Detect(id DaemonIdentity) USBDevices {
	return DetectUSB(daemonUSBMode(id, runtime.GOOS == "linux"), id)
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
