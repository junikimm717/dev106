package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
)

func TestUSBAdvice(t *testing.T) {
	ready := USBDevices{Mode: USBHostDevices, Supported: true, BusDir: true, BoardNode: "/dev/bus/usb/001/007", BoardGID: 46}

	cases := []struct {
		name     string
		devices  USBDevices
		wsl      bool
		platform string
		want     []string
		quiet    bool
	}{
		{
			name:     "board ready is silent",
			devices:  ready,
			platform: "this host",
			quiet:    true,
		},
		{
			name:     "unknown unsupported daemon stays generic",
			devices:  USBDevices{Mode: USBUnavailable, Supported: false},
			platform: "Docker on macOS",
			want:     []string{"Docker on macOS", "lab-bc", "flash from a Linux host"},
		},
		{
			name: "docker desktop names usb/ip and points at orbstack",
			devices: USBDevices{
				Mode:     USBUnavailable,
				Identity: DaemonIdentity{OperatingSystem: "Docker Desktop"},
			},
			platform: "Docker on macOS",
			want:     []string{"USB/IP", "OrbStack", "lab-bc"},
		},
		{
			name: "remote daemon says where the board has to be",
			devices: USBDevices{
				Mode:     USBUnavailable,
				Identity: DaemonIdentity{Remote: true},
			},
			platform: "this host",
			want:     []string{"not on this machine", "lab-bc"},
		},
		{
			name: "orbstack points at orb usb attach rather than claiming failure",
			devices: USBDevices{
				Mode:      USBSharedVM,
				Supported: true,
				BusDir:    true,
				Identity:  DaemonIdentity{OperatingSystem: "OrbStack"},
			},
			platform: "Docker on macOS",
			want:     []string{"orb usb attach", "orb usb list"},
		},
		{
			name:     "root owned node points at the udev rule",
			devices:  USBDevices{Mode: USBHostDevices, Supported: true, BusDir: true, BoardNode: "/dev/bus/usb/001/007", BoardGID: 0},
			platform: "this host",
			want:     []string{"owned by root", "99-openfpgaloader.rules", "udevadm"},
		},
		{
			name:     "missing board on plain linux says plug it in",
			devices:  USBDevices{Mode: USBHostDevices, Supported: true, BusDir: true},
			platform: "this host",
			want:     []string{"0403:6010", "dev106 restart"},
		},
		{
			name:     "missing board under wsl explains usbipd",
			devices:  USBDevices{Mode: USBHostDevices, Supported: true, BusDir: true},
			wsl:      true,
			platform: "this host",
			want:     []string{"usbipd attach", "0403:6010"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := usbAdvice(tc.devices, tc.wsl, tc.platform)
			if tc.quiet {
				if got != "" {
					t.Fatalf("expected silence, got:\n%s", got)
				}
				return
			}
			if got == "" {
				t.Fatal("expected a warning, got none")
			}
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("warning missing %q:\n%s", want, got)
				}
			}
		})
	}
}

// Simulation and lab-bc builds are the bulk of the course and need no board,
// so a missing board must never be phrased as a hard stop.
func TestUSBAdviceNeverClaimsEverythingIsBroken(t *testing.T) {
	for _, devices := range []USBDevices{
		{Mode: USBUnavailable, Supported: false},
		{Mode: USBUnavailable, Identity: DaemonIdentity{OperatingSystem: "Docker Desktop"}},
		{Mode: USBUnavailable, Identity: DaemonIdentity{Remote: true}},
		{Mode: USBHostDevices, Supported: true, BusDir: true},
	} {
		got := usbAdvice(devices, false, "this host")
		if !strings.HasPrefix(got, "warning:") {
			t.Fatalf("advice should be a warning, not an error:\n%s", got)
		}
		if !strings.Contains(got, "lab-bc") {
			t.Fatalf("advice should say remote builds still work:\n%s", got)
		}
	}
}

// The OrbStack case is not a failure, so it must not be phrased as a warning.
func TestSharedVMAdviceIsANoteNotAWarning(t *testing.T) {
	got := usbAdvice(USBDevices{Mode: USBSharedVM, Supported: true, BusDir: true}, false, "Docker on macOS")
	if !strings.HasPrefix(got, "note:") {
		t.Fatalf("shared-VM advice should be a note:\n%s", got)
	}
	if strings.Contains(got, "cannot") {
		t.Fatalf("OrbStack can pass USB through; advice must not say otherwise:\n%s", got)
	}
}

func TestDaemonUSBMode(t *testing.T) {
	cases := []struct {
		name        string
		id          DaemonIdentity
		clientLinux bool
		want        DaemonUSBMode
	}{
		{
			name: "orbstack by operating system",
			id:   DaemonIdentity{OperatingSystem: "OrbStack"},
			want: USBSharedVM,
		},
		{
			// The kernel string is the backstop if the product name changes.
			name: "orbstack by kernel version",
			id:   DaemonIdentity{OperatingSystem: "", KernelVersion: "7.0.14-orbstack-00380-ga7e0a2dc9535"},
			want: USBSharedVM,
		},
		{
			name: "docker desktop on macOS has no path",
			id:   DaemonIdentity{OperatingSystem: "Docker Desktop"},
			want: USBUnavailable,
		},
		{
			// WSL2 shares one kernel across distros, so a usbipd-attached
			// device reaches the docker-desktop distro too.
			name:        "docker desktop under wsl uses host devices",
			id:          DaemonIdentity{OperatingSystem: "Docker Desktop"},
			clientLinux: true,
			want:        USBHostDevices,
		},
		{
			name:        "native linux daemon",
			id:          DaemonIdentity{OperatingSystem: "Ubuntu 24.04.1 LTS"},
			clientLinux: true,
			want:        USBHostDevices,
		},
		{
			// A local scan would describe the wrong machine entirely.
			name:        "remote daemon beats everything else",
			id:          DaemonIdentity{OperatingSystem: "Ubuntu 24.04.1 LTS", Remote: true},
			clientLinux: true,
			want:        USBUnavailable,
		},
		{
			name: "unknown daemon on a mac",
			id:   DaemonIdentity{OperatingSystem: "Alpine Linux v3.20"},
			want: USBUnavailable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := daemonUSBMode(tc.id, tc.clientLinux); got != tc.want {
				t.Fatalf("daemonUSBMode() = %v, want %v", got, tc.want)
			}
		})
	}
}

// A quiet scan on a daemon we cannot enumerate must never be reported as
// "no board" -- that was the bug this whole mode split exists to prevent.
func TestSharedVMNeverReportsMissingBoard(t *testing.T) {
	u := DetectUSB(USBSharedVM, DaemonIdentity{OperatingSystem: "OrbStack"})

	if u.Enumerable() {
		t.Fatal("a shared-VM daemon is not enumerable from here")
	}
	if !u.BusDir {
		t.Fatal("the VM's bus should still be bound")
	}
	if got := usbAdvice(u, false, "Docker on macOS"); strings.Contains(got, "no FPGA board") {
		t.Fatalf("must not claim the board is missing:\n%s", got)
	}

	hc := &container.HostConfig{}
	applyUSB(hc, u)
	if !containsString(hc.Binds, "/dev/bus/usb:/dev/bus/usb") {
		t.Fatalf("shared VM should still get the bus bind: %v", hc.Binds)
	}
	if !containsString(hc.DeviceCgroupRules, usbCgroupRule) {
		t.Fatalf("shared VM should still get the cgroup rule: %v", hc.DeviceCgroupRules)
	}
}

func TestDetectUSBUnavailableTouchesNothing(t *testing.T) {
	u := DetectUSB(USBUnavailable, DaemonIdentity{OperatingSystem: "Docker Desktop"})
	if u.Supported || u.BusDir || u.BoardFound() || len(u.Serial) != 0 {
		t.Fatalf("unavailable mode should be empty: %+v", u)
	}
}

func TestApplyUSB(t *testing.T) {
	t.Run("unsupported host changes nothing", func(t *testing.T) {
		hc := &container.HostConfig{Binds: []string{"/repo:/workspace:rw"}}
		applyUSB(hc, USBDevices{Mode: USBUnavailable, Supported: false})
		if len(hc.Binds) != 1 || len(hc.Devices) != 0 || len(hc.DeviceCgroupRules) != 0 {
			t.Fatalf("host config was modified: %+v", hc)
		}
	})

	t.Run("no bus dir changes nothing", func(t *testing.T) {
		hc := &container.HostConfig{}
		applyUSB(hc, USBDevices{Mode: USBHostDevices, Supported: true, BusDir: false, GroupIDs: []string{"46"}})
		if len(hc.Binds) != 0 || len(hc.GroupAdd) != 0 {
			t.Fatalf("host config was modified: %+v", hc)
		}
	})

	t.Run("full passthrough", func(t *testing.T) {
		hc := &container.HostConfig{Binds: []string{"/repo:/workspace:rw"}}
		applyUSB(hc, USBDevices{
			Mode:      USBHostDevices,
			Supported: true,
			BusDir:    true,
			BoardNode: "/dev/bus/usb/001/007",
			BoardGID:  46,
			Serial:    []string{"/dev/ttyUSB0", "/dev/ttyUSB1"},
			GroupIDs:  []string{"20", "46"},
		})

		if !containsString(hc.Binds, "/dev/bus/usb:/dev/bus/usb") {
			t.Fatalf("usb bus not bind mounted: %v", hc.Binds)
		}
		if !containsString(hc.Binds, "/repo:/workspace:rw") {
			t.Fatalf("existing binds were dropped: %v", hc.Binds)
		}
		if !containsString(hc.DeviceCgroupRules, usbCgroupRule) {
			t.Fatalf("missing cgroup rule: %v", hc.DeviceCgroupRules)
		}
		if len(hc.Devices) != 2 {
			t.Fatalf("expected both serial nodes, got %+v", hc.Devices)
		}
		for _, d := range hc.Devices {
			if d.PathOnHost != d.PathInContainer {
				t.Fatalf("serial node path changed: %+v", d)
			}
		}
		if len(hc.GroupAdd) != 2 {
			t.Fatalf("expected both gids, got %v", hc.GroupAdd)
		}
	})
}

func TestStaleContainerAdvice(t *testing.T) {
	board := USBDevices{Mode: USBHostDevices, Supported: true, BusDir: true, BoardNode: "/dev/bus/usb/001/007", BoardGID: 46}

	if got := staleContainerAdvice(board, false); !strings.Contains(got, "dev106 restart") {
		t.Fatalf("board plugged in after creation should suggest restart, got %q", got)
	}
	if got := staleContainerAdvice(board, true); got != "" {
		t.Fatalf("container already has usb, expected silence, got %q", got)
	}
	if got := staleContainerAdvice(USBDevices{Mode: USBHostDevices, Supported: true, BusDir: true}, false); got != "" {
		t.Fatalf("no board means the plain warning already fired, got %q", got)
	}
	if got := staleContainerAdvice(USBDevices{Mode: USBUnavailable, Supported: false}, false); got != "" {
		t.Fatalf("unsupported host, expected silence, got %q", got)
	}
}

func TestContainerHasUSBPassthrough(t *testing.T) {
	if !containerHasUSBPassthrough([]string{"/repo:/workspace:rw", "/dev/bus/usb:/dev/bus/usb"}) {
		t.Fatal("should have detected the usb bind")
	}
	if containerHasUSBPassthrough([]string{"/repo:/workspace:rw"}) {
		t.Fatal("false positive on a plain bind list")
	}
	// A repo that happens to be named like the bus must not count.
	if containerHasUSBPassthrough([]string{"/home/x/dev/bus/usbthing:/workspace:rw"}) {
		t.Fatal("false positive on a lookalike path")
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func TestOrbSerialPorts(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "orb")
	// Real `orb serial list` output, plus an FTDI and a CDC port.
	body := `#!/bin/sh
cat <<'OUT'
cu.Bluetooth-Incoming-Port   callout  /dev/cu.Bluetooth-Incoming-Port
cu.debug-console             callout  /dev/cu.debug-console
cu.usbserial-FT9AB2CD        callout  /dev/cu.usbserial-FT9AB2CD
tty.Bluetooth-Incoming-Port  tty      /dev/tty.Bluetooth-Incoming-Port
tty.debug-console            tty      /dev/tty.debug-console
tty.usbserial-FT9AB2CD       tty      /dev/tty.usbserial-FT9AB2CD
tty.usbmodem1101             tty      /dev/tty.usbmodem1101
OUT
`
	if err := os.WriteFile(script, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	// dir first so the fake wins; /bin so the script's own tools resolve.
	t.Setenv("PATH", dir+":/bin:/usr/bin")

	got := orbSerialPorts()

	want := []string{"/dev/tty.usbserial-FT9AB2CD", "/dev/tty.usbmodem1101"}
	if len(got) != len(want) {
		t.Fatalf("orbSerialPorts() = %v, want %v", got, want)
	}
	for _, w := range want {
		if !containsString(got, w) {
			t.Fatalf("orbSerialPorts() = %v, missing %s", got, w)
		}
	}
	for _, g := range got {
		// /dev/cu.* is the same port again; Bluetooth is not a board.
		if strings.Contains(g, "/dev/cu.") || strings.Contains(g, "Bluetooth") {
			t.Fatalf("should have been filtered out: %s", g)
		}
	}
}

func TestOrbSerialPortsWithoutOrb(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if got := orbSerialPorts(); got != nil {
		t.Fatalf("no orb binary should yield nothing, got %v", got)
	}
}
