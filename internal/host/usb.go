package host

import (
	"os/exec"
	"runtime"
	"strings"
)

// USBID is a USB vendor:product pair, lowercase hex without the 0x.
type USBID struct{ Vendor, Product string }

func (id USBID) String() string { return id.Vendor + ":" + id.Product }

// DefaultUSBIDs are the FTDI parts openFPGALoader drives as JTAG bridges.
// One ID is not enough: Digilent alone spans two, so pinning 6010 would call
// an HS2 or a JTAG-SMT2 board missing.
//
// The UART-only FT232RL (6001) and FT231X (6015) are left out deliberately.
// openFPGALoader can bit-bang JTAG over them, but they are also the most
// common plain serial chips in existence, and telling someone their Arduino
// is a detached FPGA is a worse failure than not recognising a rare cable.
// Add those explicitly with usb_ids if you have such a programmer.
var DefaultUSBIDs = []USBID{
	{"0403", "6010"}, // FT2232H: Urbana, Arty, most Digilent boards
	{"0403", "6011"}, // FT4232H
	{"0403", "6014"}, // FT232H: Digilent HS2/HS3, JTAG-SMT2
	{"0403", "6043"}, // FT4232HP
}

// ParseUSBIDs reads "vid:pid" strings from config. Malformed entries are
// returned rather than rejected, so one typo cannot stop a shell opening.
func ParseUSBIDs(raw []string) (ids []USBID, bad []string) {
	for _, entry := range raw {
		vendor, product, found := strings.Cut(strings.TrimSpace(entry), ":")
		vendor = strings.TrimPrefix(strings.ToLower(vendor), "0x")
		product = strings.TrimPrefix(strings.ToLower(product), "0x")
		if !found || !isHex(vendor) || !isHex(product) {
			bad = append(bad, entry)
			continue
		}
		ids = append(ids, USBID{vendor, product})
	}
	return ids, bad
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// usbIDsOrDefault keeps a caller that passes nothing on the default list.
func usbIDsOrDefault(ids []USBID) []USBID {
	if len(ids) == 0 {
		return DefaultUSBIDs
	}
	return ids
}

func matchesID(vidPID string, ids []USBID) bool {
	for _, id := range ids {
		if strings.EqualFold(vidPID, id.String()) {
			return true
		}
	}
	return false
}

// FormatUSBIDs renders a list for a warning.
func FormatUSBIDs(ids []USBID) string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return strings.Join(out, ", ")
}

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

	// AttachKnown records that we got a straight answer about the shared VM;
	// false means we could not ask, which is not the same as "no board".
	AttachKnown   bool
	BoardAttached bool
	// BoardID identifies the board to `orb usb attach`.
	BoardID string
	// BoardVidPID is the ID we actually matched, not the one we assumed.
	BoardVidPID string

	// IDs is what we searched for, so a warning can say so.
	IDs []USBID
}

func (u USBDevices) BoardFound() bool { return u.BoardNode != "" }

// BoardDetached is the shared-VM counterpart of a missing board: the Mac has
// it, the daemon's VM does not, and one command fixes that.
func (u USBDevices) BoardDetached() bool {
	return u.Mode == USBSharedVM && u.AttachKnown && !u.BoardAttached
}

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
func Detect(id DaemonIdentity, ids []USBID) USBDevices {
	return DetectUSB(daemonUSBMode(id, runtime.GOOS == "linux"), id, ids)
}

// DetectUSB reports what the daemon can hand a container.
func DetectUSB(mode DaemonUSBMode, id DaemonIdentity, ids []USBID) USBDevices {
	ids = usbIDsOrDefault(ids)
	u := USBDevices{Mode: mode, Identity: id, Supported: mode != USBUnavailable, IDs: ids}
	switch mode {
	case USBHostDevices:
		scanHostUSB(&u)
	case USBSharedVM:
		// Cannot stat the VM's bus from here; the daemon resolves the bind.
		u.BusDir = true
		u.Serial = orbSerialPorts()
		u.BoardID, u.BoardVidPID, u.BoardAttached, u.AttachKnown = orbBoard(ids)

		// The node is root:root 0660 inside the VM. No udev rule of ours runs
		// there, and a chown at startup would not survive a replug (the device
		// number changes), so the root group is the only durable way for the
		// container user to open it. scanHostUSB drops gid 0 on purpose; there
		// a udev rule is the right answer and this would be a needless grant.
		u.GroupIDs = append(u.GroupIDs, "0")
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

// orbBoard asks OrbStack whether the FPGA is attached to its VM. `orb serial
// list` cannot answer this: the FT2232H's JTAG channel is not a serial port,
// so macOS creates no cu.usbserial node and the board never appears there.
//
// Lines are "ID  VID:PID  NAME  STATE", NAME is multi-word, and STATE is blank
// when detached, so the last field is the only reliable place to look.
func orbBoard(ids []USBID) (id, vidPID string, attached, known bool) {
	orb, err := exec.LookPath("orb")
	if err != nil {
		return "", "", false, false
	}
	out, err := exec.Command(orb, "usb", "list").Output()
	if err != nil {
		return "", "", false, false
	}

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || !matchesID(fields[1], ids) {
			continue
		}
		return fields[0], fields[1], fields[len(fields)-1] == "attached", true
	}

	// Nothing we recognise. That is NOT "your board is detached" -- it may be
	// a programmer that is not in our list at all, and saying otherwise names
	// a command that cannot help. Stay unknown and let the hedged note run.
	return "", "", false, false
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
