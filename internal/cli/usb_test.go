package cli

import (
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
)

func TestUSBAdvice(t *testing.T) {
	ready := USBDevices{Supported: true, BusDir: true, BoardNode: "/dev/bus/usb/001/007", BoardGID: 46}

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
			name:     "unsupported host explains the VM",
			devices:  USBDevices{Supported: false},
			platform: "Docker on macOS",
			want:     []string{"Docker on macOS", "lab-bc", "no setting"},
		},
		{
			name:     "root owned node points at the udev rule",
			devices:  USBDevices{Supported: true, BusDir: true, BoardNode: "/dev/bus/usb/001/007", BoardGID: 0},
			platform: "this host",
			want:     []string{"owned by root", "99-openfpgaloader.rules", "udevadm"},
		},
		{
			name:     "missing board on plain linux says plug it in",
			devices:  USBDevices{Supported: true, BusDir: true},
			platform: "this host",
			want:     []string{"0403:6010", "dev106 restart"},
		},
		{
			name:     "missing board under wsl explains usbipd",
			devices:  USBDevices{Supported: true, BusDir: true},
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
		{Supported: false},
		{Supported: true, BusDir: true},
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

func TestApplyUSB(t *testing.T) {
	t.Run("unsupported host changes nothing", func(t *testing.T) {
		hc := &container.HostConfig{Binds: []string{"/repo:/workspace:rw"}}
		applyUSB(hc, USBDevices{Supported: false})
		if len(hc.Binds) != 1 || len(hc.Devices) != 0 || len(hc.DeviceCgroupRules) != 0 {
			t.Fatalf("host config was modified: %+v", hc)
		}
	})

	t.Run("no bus dir changes nothing", func(t *testing.T) {
		hc := &container.HostConfig{}
		applyUSB(hc, USBDevices{Supported: true, BusDir: false, GroupIDs: []string{"46"}})
		if len(hc.Binds) != 0 || len(hc.GroupAdd) != 0 {
			t.Fatalf("host config was modified: %+v", hc)
		}
	})

	t.Run("full passthrough", func(t *testing.T) {
		hc := &container.HostConfig{Binds: []string{"/repo:/workspace:rw"}}
		applyUSB(hc, USBDevices{
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
	board := USBDevices{Supported: true, BusDir: true, BoardNode: "/dev/bus/usb/001/007", BoardGID: 46}

	if got := staleContainerAdvice(board, false); !strings.Contains(got, "dev106 restart") {
		t.Fatalf("board plugged in after creation should suggest restart, got %q", got)
	}
	if got := staleContainerAdvice(board, true); got != "" {
		t.Fatalf("container already has usb, expected silence, got %q", got)
	}
	if got := staleContainerAdvice(USBDevices{Supported: true, BusDir: true}, false); got != "" {
		t.Fatalf("no board means the plain warning already fired, got %q", got)
	}
	if got := staleContainerAdvice(USBDevices{Supported: false}, false); got != "" {
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
