//go:build linux

package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

// fakeHost is a sysfs/dev tree DetectUSB can walk without real hardware.
type fakeHost struct {
	sysfs string
	dev   string
}

func newFakeHost(t *testing.T) *fakeHost {
	t.Helper()
	base := t.TempDir()
	h := &fakeHost{
		sysfs: filepath.Join(base, "sys"),
		dev:   filepath.Join(base, "dev"),
	}
	if err := os.MkdirAll(h.sysfs, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(h.dev, "bus", "usb"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", base)

	oldSysfs, oldBus, oldDev, oldGroup := sysfsUSBDir, USBBusDir, devDir, groupFile
	sysfsUSBDir = h.sysfs
	USBBusDir = filepath.Join(h.dev, "bus", "usb")
	devDir = h.dev
	groupFile = filepath.Join(base, "group")
	if err := os.WriteFile(groupFile, []byte("root:x:0:\nplugdev:x:46:\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sysfsUSBDir, USBBusDir, devDir, groupFile = oldSysfs, oldBus, oldDev, oldGroup
	})
	return h
}

// addUSBDevice writes one device's sysfs attributes and its /dev node.
func (h *fakeHost) addUSBDevice(t *testing.T, name, vendor, product string, bus, dev int) string {
	t.Helper()
	dir := filepath.Join(h.sysfs, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for file, value := range map[string]string{
		"idVendor":  vendor,
		"idProduct": product,
		"busnum":    strconv.Itoa(bus),
		"devnum":    strconv.Itoa(dev),
	} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(value+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	node := filepath.Join(h.dev, "bus", "usb", fmt.Sprintf("%03d", bus), fmt.Sprintf("%03d", dev))
	if err := os.MkdirAll(filepath.Dir(node), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(node, nil, 0660); err != nil {
		t.Fatal(err)
	}
	return node
}

func (h *fakeHost) addSerial(t *testing.T, name string) string {
	t.Helper()
	node := filepath.Join(h.dev, name)
	if err := os.WriteFile(node, nil, 0660); err != nil {
		t.Fatal(err)
	}
	return node
}

func mustProfile(t *testing.T, name, label string, ids []USBID) DeviceProfile {
	t.Helper()
	p, err := Profile(name, label, ids)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func currentGID(t *testing.T) int {
	t.Helper()
	return syscall.Getgid()
}

// A board from the default list, so these tests stay meaningful if the
// default changes.
var (
	boardVendor  = DefaultUSBIDs[0].Vendor
	boardProduct = DefaultUSBIDs[0].Product
)

// Digilent HS2/HS3 and JTAG-SMT2 are 0403:6014. Pinning a single ID reported
// them missing on Linux exactly as it did on OrbStack.
func TestDetectUSBFindsNonDefaultProgrammer(t *testing.T) {
	h := newFakeHost(t)
	want := h.addUSBDevice(t, "1-4", "0403", "6014", 1, 9)

	got := DetectUSB(USBHostDevices, DaemonIdentity{OperatingSystem: "Ubuntu 24.04.1 LTS"}, FPGAProfile)

	if got.DeviceNode != want {
		t.Fatalf("DeviceNode = %q, want %q (an FT232H is a programmer too)", got.DeviceNode, want)
	}
}

// A programmer nobody anticipated works once usb_ids names it.
func TestDetectUSBHonoursConfiguredIDs(t *testing.T) {
	h := newFakeHost(t)
	want := h.addUSBDevice(t, "1-5", "1d50", "6018", 1, 11)

	got := DetectUSB(USBHostDevices, DaemonIdentity{OperatingSystem: "Ubuntu 24.04.1 LTS"},
		mustProfile(t, "", "", []USBID{{"1d50", "6018"}}))

	if got.DeviceNode != want {
		t.Fatalf("DeviceNode = %q, want %q", got.DeviceNode, want)
	}
}

// A plain USB-serial adapter must not be mistaken for a board.
func TestDetectUSBIgnoresUartOnlyFTDI(t *testing.T) {
	h := newFakeHost(t)
	h.addUSBDevice(t, "1-6", "0403", "6001", 1, 13)

	got := DetectUSB(USBHostDevices, DaemonIdentity{OperatingSystem: "Ubuntu 24.04.1 LTS"}, FPGAProfile)

	if got.DeviceFound() {
		t.Fatalf("an FT232RL is not an FPGA board: %q", got.DeviceNode)
	}
}

func TestDetectUSBFindsBoard(t *testing.T) {
	h := newFakeHost(t)
	// Only the programmer should match.
	h.addUSBDevice(t, "usb1", "1d6b", "0002", 1, 1)
	h.addUSBDevice(t, "1-2", "046d", "c52b", 1, 4)
	want := h.addUSBDevice(t, "1-3", boardVendor, boardProduct, 1, 7)

	got := DetectUSB(USBHostDevices, DaemonIdentity{OperatingSystem: "Ubuntu 24.04.1 LTS"}, FPGAProfile)

	if !got.Supported || !got.BusDir || !got.Enumerable() {
		t.Fatalf("linux host should support passthrough: %+v", got)
	}
	if got.DeviceNode != want {
		t.Fatalf("DeviceNode = %q, want %q", got.DeviceNode, want)
	}
	if !got.DeviceFound() {
		t.Fatal("DeviceFound() should be true")
	}
	if got.DeviceGID != currentGID(t) {
		t.Fatalf("DeviceGID = %d, want %d", got.DeviceGID, currentGID(t))
	}
}

// Zero padded in /dev but not in sysfs, so string concat would miss the node.
func TestDetectUSBPadsBusAndDeviceNumbers(t *testing.T) {
	h := newFakeHost(t)
	want := h.addUSBDevice(t, "3-1", boardVendor, boardProduct, 3, 12)

	got := DetectUSB(USBHostDevices, DaemonIdentity{OperatingSystem: "Ubuntu 24.04.1 LTS"}, FPGAProfile)
	if got.DeviceNode != want {
		t.Fatalf("DeviceNode = %q, want %q", got.DeviceNode, want)
	}
	if filepath.Base(got.DeviceNode) != "012" {
		t.Fatalf("device number not zero padded: %q", got.DeviceNode)
	}
}

func TestDetectUSBNoBoardFallsBackToPlugdev(t *testing.T) {
	h := newFakeHost(t)
	h.addUSBDevice(t, "1-2", "046d", "c52b", 1, 4)

	got := DetectUSB(USBHostDevices, DaemonIdentity{OperatingSystem: "Ubuntu 24.04.1 LTS"}, FPGAProfile)

	if got.DeviceFound() {
		t.Fatalf("no board should have matched: %+v", got)
	}
	if !containsString(got.GroupIDs, "46") {
		t.Fatalf("should have joined plugdev, got %v", got.GroupIDs)
	}
	if warning := USBWarning(got); warning == "" {
		t.Fatal("a missing board should warn")
	}
}

func TestDetectUSBCollectsSerialNodes(t *testing.T) {
	h := newFakeHost(t)
	h.addUSBDevice(t, "1-3", boardVendor, boardProduct, 1, 7)
	usb0 := h.addSerial(t, "ttyUSB0")
	acm0 := h.addSerial(t, "ttyACM0")
	h.addSerial(t, "ttyS0") // a plain UART, not ours

	got := DetectUSB(USBHostDevices, DaemonIdentity{OperatingSystem: "Ubuntu 24.04.1 LTS"}, FPGAProfile)

	if len(got.Serial) != 2 {
		t.Fatalf("Serial = %v, want ttyUSB0 and ttyACM0", got.Serial)
	}
	if !containsString(got.Serial, usb0) || !containsString(got.Serial, acm0) {
		t.Fatalf("Serial = %v", got.Serial)
	}
	for _, s := range got.Serial {
		if filepath.Base(s) == "ttyS0" {
			t.Fatal("ttyS0 is not a USB serial adapter")
		}
	}
}

// Adding gid 0 would be a privilege grant, not a convenience.
func TestDetectUSBNeverAddsRootGroup(t *testing.T) {
	h := newFakeHost(t)
	h.addUSBDevice(t, "1-3", boardVendor, boardProduct, 1, 7)

	for _, gid := range DetectUSB(USBHostDevices, DaemonIdentity{OperatingSystem: "Ubuntu 24.04.1 LTS"}, FPGAProfile).GroupIDs {
		if gid == "0" {
			t.Fatal("gid 0 must never be added")
		}
	}
}

func TestDetectUSBMissingSysfsIsHarmless(t *testing.T) {
	h := newFakeHost(t)
	if err := os.RemoveAll(h.sysfs); err != nil {
		t.Fatal(err)
	}

	got := DetectUSB(USBHostDevices, DaemonIdentity{OperatingSystem: "Ubuntu 24.04.1 LTS"}, FPGAProfile)
	if got.DeviceFound() {
		t.Fatal("no sysfs means no board")
	}
	if !got.Supported {
		t.Fatal("the host is still linux")
	}
}
