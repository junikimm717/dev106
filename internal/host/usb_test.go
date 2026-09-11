package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
			// The board is there and usable, so nagging about attaching it
			// was the bug: the note fired on every single start.
			name: "orbstack with the board attached is silent",
			devices: USBDevices{
				Mode:          USBSharedVM,
				Supported:     true,
				BusDir:        true,
				AttachKnown:   true,
				BoardAttached: true,
				BoardID:       "00100000",
				Identity:      DaemonIdentity{OperatingSystem: "OrbStack"},
			},
			platform: "Docker on macOS",
			quiet:    true,
		},
		{
			name: "orbstack with a detached board names the exact command",
			devices: USBDevices{
				Mode:        USBSharedVM,
				Supported:   true,
				BusDir:      true,
				AttachKnown: true,
				BoardID:     "00100000",
				Identity:    DaemonIdentity{OperatingSystem: "OrbStack"},
			},
			platform: "Docker on macOS",
			want:     []string{"orb usb attach 00100000", "0403:6010", "lab-bc"},
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
	// No orb on PATH, so this is the "cannot ask" case rather than whatever
	// the developer happens to have plugged in.
	t.Setenv("PATH", t.TempDir())

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
}

func TestDetectUSBUnavailableTouchesNothing(t *testing.T) {
	u := DetectUSB(USBUnavailable, DaemonIdentity{OperatingSystem: "Docker Desktop"})
	if u.Supported || u.BusDir || u.BoardFound() || len(u.Serial) != 0 {
		t.Fatalf("unavailable mode should be empty: %+v", u)
	}
}

func TestStaleContainerAdvice(t *testing.T) {
	board := USBDevices{Mode: USBHostDevices, Supported: true, BusDir: true, BoardNode: "/dev/bus/usb/001/007", BoardGID: 46}

	if got := StaleContainerAdvice(board, false); !strings.Contains(got, "dev106 restart") {
		t.Fatalf("board plugged in after creation should suggest restart, got %q", got)
	}
	if got := StaleContainerAdvice(board, true); got != "" {
		t.Fatalf("container already has usb, expected silence, got %q", got)
	}
	if got := StaleContainerAdvice(USBDevices{Mode: USBHostDevices, Supported: true, BusDir: true}, false); got != "" {
		t.Fatalf("no board means the plain warning already fired, got %q", got)
	}
	if got := StaleContainerAdvice(USBDevices{Mode: USBUnavailable, Supported: false}, false); got != "" {
		t.Fatalf("unsupported host, expected silence, got %q", got)
	}
}

func TestContainerHasUSBPassthrough(t *testing.T) {
	if !ContainerHasUSBPassthrough([]string{"/repo:/workspace:rw", "/dev/bus/usb:/dev/bus/usb"}) {
		t.Fatal("should have detected the usb bind")
	}
	if ContainerHasUSBPassthrough([]string{"/repo:/workspace:rw"}) {
		t.Fatal("false positive on a plain bind list")
	}
	// A repo that happens to be named like the bus must not count.
	if ContainerHasUSBPassthrough([]string{"/home/x/dev/bus/usbthing:/workspace:rw"}) {
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

// fakeOrb puts a stub `orb` on PATH that prints body for any subcommand.
func fakeOrb(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\ncat <<'OUT'\n" + body + "\nOUT\n"
	if err := os.WriteFile(filepath.Join(dir, "orb"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/bin:/usr/bin")
}

func TestOrbBoard(t *testing.T) {
	// Real `orb usb list` output. Detached leaves the state column blank, and
	// the board name is two words, so neither field count nor position alone
	// identifies the state.
	cases := []struct {
		name         string
		out          string
		wantID       string
		wantAttached bool
		wantKnown    bool
	}{
		{
			name:         "attached board",
			out:          "00100000  0403:6010  Xilinx JTAG+Serial  attached",
			wantID:       "00100000",
			wantAttached: true,
			wantKnown:    true,
		},
		{
			name:      "detached board has a blank state column",
			out:       "00100000  0403:6010  Xilinx JTAG+Serial  ",
			wantID:    "00100000",
			wantKnown: true,
		},
		{
			// orb answered, so "not listed" really does mean unplugged.
			name:      "board absent but other devices present",
			out:       "00200000  05ac:8103  Apple Keyboard  attached",
			wantKnown: true,
		},
		{
			name:      "empty list is still an answer",
			out:       "",
			wantKnown: true,
		},
		{
			name:         "board among others",
			out:          "00200000  05ac:8103  Apple Keyboard  attached\n00100000  0403:6010  Xilinx JTAG+Serial  attached",
			wantID:       "00100000",
			wantAttached: true,
			wantKnown:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeOrb(t, tc.out)
			id, attached, known := orbBoard()
			if id != tc.wantID || attached != tc.wantAttached || known != tc.wantKnown {
				t.Fatalf("orbBoard() = (%q, %v, %v), want (%q, %v, %v)",
					id, attached, known, tc.wantID, tc.wantAttached, tc.wantKnown)
			}
		})
	}
}

// Without orb we know nothing, which must not be mistaken for "detached".
func TestOrbBoardWithoutOrb(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if id, attached, known := orbBoard(); id != "" || attached || known {
		t.Fatalf("orbBoard() = (%q, %v, %v), want empty and unknown", id, attached, known)
	}
	u := DetectUSB(USBSharedVM, DaemonIdentity{OperatingSystem: "OrbStack"})
	if u.BoardDetached() {
		t.Fatal("not being able to ask is not the same as detached")
	}
}

// The VM's node is root:root 0660, so without the root group the container
// user cannot open the board at all -- passthrough was wired up and then
// silently blocked on permissions.
func TestSharedVMJoinsRootGroup(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	u := DetectUSB(USBSharedVM, DaemonIdentity{OperatingSystem: "OrbStack"})
	if !containsString(u.GroupIDs, "0") {
		t.Fatalf("shared VM should join the root group, got %v", u.GroupIDs)
	}
}

func TestOrbSerialPortsWithoutOrb(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if got := orbSerialPorts(); got != nil {
		t.Fatalf("no orb binary should yield nothing, got %v", got)
	}
}
