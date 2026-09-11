package docker

import (
	"context"
	"github.com/junikimm717/dev106/internal/config"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/junikimm717/dev106/internal/host"
	"github.com/moby/moby/api/types/container"
)

func TestApplyUSB(t *testing.T) {
	t.Run("unsupported host changes nothing", func(t *testing.T) {
		hc := &container.HostConfig{Binds: []string{"/repo:/workspace:rw"}}
		applyUSB(hc, host.USBDevices{Mode: host.USBUnavailable, Supported: false})
		if len(hc.Binds) != 1 || len(hc.Devices) != 0 || len(hc.DeviceCgroupRules) != 0 {
			t.Fatalf("host config was modified: %+v", hc)
		}
	})

	t.Run("no bus dir changes nothing", func(t *testing.T) {
		hc := &container.HostConfig{}
		applyUSB(hc, host.USBDevices{Mode: host.USBHostDevices, Supported: true, BusDir: false, GroupIDs: []string{"46"}})
		if len(hc.Binds) != 0 || len(hc.GroupAdd) != 0 {
			t.Fatalf("host config was modified: %+v", hc)
		}
	})

	t.Run("full passthrough", func(t *testing.T) {
		hc := &container.HostConfig{Binds: []string{"/repo:/workspace:rw"}}
		applyUSB(hc, host.USBDevices{
			Mode:       host.USBHostDevices,
			Supported:  true,
			BusDir:     true,
			DeviceNode: "/dev/bus/usb/001/007",
			DeviceGID:  46,
			Serial:     []string{"/dev/ttyUSB0", "/dev/ttyUSB1"},
			GroupIDs:   []string{"20", "46"},
		})

		if !containsString(hc.Binds, "/dev/bus/usb:/dev/bus/usb") {
			t.Fatalf("usb bus not bind mounted: %v", hc.Binds)
		}
		if !containsString(hc.Binds, "/repo:/workspace:rw") {
			t.Fatalf("existing binds were dropped: %v", hc.Binds)
		}
		if !containsString(hc.DeviceCgroupRules, host.USBCgroupRule) {
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

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// A daemon we cannot enumerate still gets the bus bind; that is the whole
// point of trusting the VM rather than scanning for a board.
func TestApplyUSBSharedVM(t *testing.T) {
	hc := &container.HostConfig{}
	applyUSB(hc, host.Detect(host.DaemonIdentity{OperatingSystem: "OrbStack"}, host.FPGAProfile))

	if !containsString(hc.Binds, "/dev/bus/usb:/dev/bus/usb") {
		t.Fatalf("shared VM should still get the bus bind: %v", hc.Binds)
	}
	if !containsString(hc.DeviceCgroupRules, host.USBCgroupRule) {
		t.Fatalf("shared VM should still get the cgroup rule: %v", hc.DeviceCgroupRules)
	}
}

// 6.106 and 6.181 never pass a device through, so USB config problems must
// stay silent for them -- a warning about a key they do not use is noise.
func TestUSBConfigWarningsOnlyWhenUSBIsOn(t *testing.T) {
	broken := &config.DevConfig{
		Image:      "ghcr.io/junikimm717/dev106/mit_6181:latest",
		USBIDs:     []string{"garbage"},
		USBProfile: "nonsense",
	}

	for _, tc := range []struct {
		name string
		usb  bool
		want bool
	}{
		{name: "usb off stays silent", usb: false, want: false},
		{name: "usb on reports", usb: true, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			broken.USB = tc.usb
			stderr := os.Stderr
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			os.Stderr = w
			_, newErr := New(context.Background(), broken)
			w.Close()
			os.Stderr = stderr

			out, _ := io.ReadAll(r)
			if newErr != nil {
				t.Skipf("no docker daemon here: %v", newErr)
			}
			if got := strings.Contains(string(out), "usb"); got != tc.want {
				t.Fatalf("usb=%t produced %q, wanted warnings=%t", tc.usb, out, tc.want)
			}
		})
	}
}
