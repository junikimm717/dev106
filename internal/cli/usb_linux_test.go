//go:build linux

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
)

// fakeHost builds a sysfs/dev tree that DetectUSB can walk, so the parsing of
// bus/device numbers into a /dev node is covered without a real board.
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

	t.Setenv("HOME", base) // keep anything home-relative inside the sandbox

	oldSysfs, oldBus, oldDev, oldGroup := sysfsUSBDir, usbBusDir, devDir, groupFile
	sysfsUSBDir = h.sysfs
	usbBusDir = filepath.Join(h.dev, "bus", "usb")
	devDir = h.dev
	groupFile = filepath.Join(base, "group")
	if err := os.WriteFile(groupFile, []byte("root:x:0:\nplugdev:x:46:\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sysfsUSBDir, usbBusDir, devDir, groupFile = oldSysfs, oldBus, oldDev, oldGroup
	})
	return h
}

// addUSBDevice writes the sysfs attributes for one device and, when the
// vendor/product match the board, the /dev node it resolves to.
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

func currentGID(t *testing.T) int {
	t.Helper()
	return syscall.Getgid()
}

func TestDetectUSBFindsBoard(t *testing.T) {
	h := newFakeHost(t)
	// A hub and a mouse share the tree; only the FT2232H should match.
	h.addUSBDevice(t, "usb1", "1d6b", "0002", 1, 1)
	h.addUSBDevice(t, "1-2", "046d", "c52b", 1, 4)
	want := h.addUSBDevice(t, "1-3", fpgaVendorID, fpgaProductID, 1, 7)

	got := DetectUSB()

	if !got.Supported || !got.BusDir {
		t.Fatalf("linux host should support passthrough: %+v", got)
	}
	if got.BoardNode != want {
		t.Fatalf("BoardNode = %q, want %q", got.BoardNode, want)
	}
	if !got.BoardFound() {
		t.Fatal("BoardFound() should be true")
	}
	if got.BoardGID != currentGID(t) {
		t.Fatalf("BoardGID = %d, want %d", got.BoardGID, currentGID(t))
	}
}

// The device number is zero padded in /dev but not in sysfs, and a bus number
// can carry a leading zero. Concatenating the strings would miss the node.
func TestDetectUSBPadsBusAndDeviceNumbers(t *testing.T) {
	h := newFakeHost(t)
	want := h.addUSBDevice(t, "3-1", fpgaVendorID, fpgaProductID, 3, 12)

	got := DetectUSB()
	if got.BoardNode != want {
		t.Fatalf("BoardNode = %q, want %q", got.BoardNode, want)
	}
	if filepath.Base(got.BoardNode) != "012" {
		t.Fatalf("device number not zero padded: %q", got.BoardNode)
	}
}

func TestDetectUSBNoBoardFallsBackToPlugdev(t *testing.T) {
	h := newFakeHost(t)
	h.addUSBDevice(t, "1-2", "046d", "c52b", 1, 4)

	got := DetectUSB()

	if got.BoardFound() {
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
	h.addUSBDevice(t, "1-3", fpgaVendorID, fpgaProductID, 1, 7)
	usb0 := h.addSerial(t, "ttyUSB0")
	acm0 := h.addSerial(t, "ttyACM0")
	h.addSerial(t, "ttyS0") // a plain UART, not ours

	got := DetectUSB()

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

// Root is already implied inside the container, so adding gid 0 would be a
// privilege grant rather than a convenience.
func TestDetectUSBNeverAddsRootGroup(t *testing.T) {
	h := newFakeHost(t)
	h.addUSBDevice(t, "1-3", fpgaVendorID, fpgaProductID, 1, 7)

	for _, gid := range DetectUSB().GroupIDs {
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

	got := DetectUSB()
	if got.BoardFound() {
		t.Fatal("no sysfs means no board")
	}
	if !got.Supported {
		t.Fatal("the host is still linux")
	}
}
