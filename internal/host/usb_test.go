package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUSBAdvice(t *testing.T) {
	ready := USBDevices{Mode: USBHostDevices, Supported: true, BusDir: true, DeviceNode: "/dev/bus/usb/001/007", DeviceGID: 46}

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
				Mode:           USBSharedVM,
				Supported:      true,
				BusDir:         true,
				AttachKnown:    true,
				DeviceAttached: true,
				DeviceID:       "00100000",
				Identity:       DaemonIdentity{OperatingSystem: "OrbStack"},
			},
			platform: "Docker on macOS",
			quiet:    true,
		},
		{
			name: "orbstack with a detached board names the exact command",
			devices: USBDevices{
				Mode:         USBSharedVM,
				Supported:    true,
				BusDir:       true,
				AttachKnown:  true,
				DeviceID:     "00100000",
				DeviceVidPID: "0403:6010",
				Identity:     DaemonIdentity{OperatingSystem: "OrbStack"},
			},
			platform: "Docker on macOS",
			want:     []string{"orb usb attach 00100000", "0403:6010", "lab-bc"},
		},
		{
			name:     "root owned node points at the udev rule",
			devices:  USBDevices{Mode: USBHostDevices, Supported: true, BusDir: true, DeviceNode: "/dev/bus/usb/001/007", DeviceGID: 0},
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

	u := DetectUSB(USBSharedVM, DaemonIdentity{OperatingSystem: "OrbStack"}, FPGAProfile)

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
	u := DetectUSB(USBUnavailable, DaemonIdentity{OperatingSystem: "Docker Desktop"}, FPGAProfile)
	if u.Supported || u.BusDir || u.DeviceFound() || len(u.Serial) != 0 {
		t.Fatalf("unavailable mode should be empty: %+v", u)
	}
}

func TestStaleContainerAdvice(t *testing.T) {
	board := USBDevices{Mode: USBHostDevices, Supported: true, BusDir: true, DeviceNode: "/dev/bus/usb/001/007", DeviceGID: 46}

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
		ids          []USBID
		wantID       string
		wantVidPID   string
		wantAttached bool
		wantKnown    bool
	}{
		{
			name:         "attached board",
			out:          "00100000  0403:6010  Xilinx JTAG+Serial  attached",
			wantID:       "00100000",
			wantVidPID:   "0403:6010",
			wantAttached: true,
			wantKnown:    true,
		},
		{
			name:       "detached board has a blank state column",
			out:        "00100000  0403:6010  Xilinx JTAG+Serial  ",
			wantID:     "00100000",
			wantVidPID: "0403:6010",
			wantKnown:  true,
		},
		{
			// Digilent HS2/HS3 and JTAG-SMT2 are 6014, not 6010. Pinning one
			// ID called these boards missing.
			name:         "ft232h programmer is recognised too",
			out:          "00300000  0403:6014  Digilent USB Device  attached",
			wantID:       "00300000",
			wantVidPID:   "0403:6014",
			wantAttached: true,
			wantKnown:    true,
		},
		{
			// Nothing we know is not the same as nothing plugged in, so we
			// must not claim to know. Saying "detached" here would name an
			// `orb usb attach` that cannot help.
			name: "unrecognised devices leave us unknown, not detached",
			out:  "00200000  05ac:8103  Apple Keyboard  attached",
		},
		{
			name: "empty list tells us nothing either",
			out:  "",
		},
		{
			// A UART-only FTDI part must not be mistaken for a programmer.
			name: "plain serial adapter is not a board",
			out:  "00400000  0403:6001  USB Serial  attached",
		},
		{
			name:         "board among others",
			out:          "00200000  05ac:8103  Apple Keyboard  attached\n00100000  0403:6010  Xilinx JTAG+Serial  attached",
			wantID:       "00100000",
			wantVidPID:   "0403:6010",
			wantAttached: true,
			wantKnown:    true,
		},
		{
			// An unusual programmer works once it is named in the config.
			name:         "configured id overrides the defaults",
			out:          "00500000  1d50:6018  Some Other Probe  attached",
			ids:          []USBID{{"1d50", "6018"}},
			wantID:       "00500000",
			wantVidPID:   "1d50:6018",
			wantAttached: true,
			wantKnown:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeOrb(t, tc.out)
			ids := tc.ids
			if ids == nil {
				ids = DefaultUSBIDs
			}
			id, vidPID, attached, known := orbDevice(ids)
			if id != tc.wantID || vidPID != tc.wantVidPID || attached != tc.wantAttached || known != tc.wantKnown {
				t.Fatalf("orbDevice() = (%q, %q, %v, %v), want (%q, %q, %v, %v)",
					id, vidPID, attached, known, tc.wantID, tc.wantVidPID, tc.wantAttached, tc.wantKnown)
			}
		})
	}
}

func TestParseUSBIDs(t *testing.T) {
	ids, bad := ParseUSBIDs([]string{"0403:6010", "0x0403:0x6014", " 1D50:6018 ", "nope", "0403:", ""})

	want := []USBID{{"0403", "6010"}, {"0403", "6014"}, {"1d50", "6018"}}
	if len(ids) != len(want) {
		t.Fatalf("ParseUSBIDs() = %v, want %v", ids, want)
	}
	for i, w := range want {
		if ids[i] != w {
			t.Fatalf("ParseUSBIDs()[%d] = %v, want %v", i, ids[i], w)
		}
	}
	if len(bad) != 3 {
		t.Fatalf("expected 3 malformed entries, got %v", bad)
	}
}

// A board that is plugged in but not one we know must not be announced as
// detached, because `orb usb attach` would not fix it.
func TestUnknownProgrammerDoesNotClaimDetached(t *testing.T) {
	fakeOrb(t, "00200000  05ac:8103  Apple Keyboard  attached")

	u := DetectUSB(USBSharedVM, DaemonIdentity{OperatingSystem: "OrbStack"}, FPGAProfile)
	if u.DeviceDetached() {
		t.Fatal("an unrecognised device list is not evidence the board is detached")
	}
	got := usbAdvice(u, false, "Docker on macOS")
	if !strings.HasPrefix(got, "note:") {
		t.Fatalf("should fall back to the hedged note:\n%s", got)
	}
}

// Without orb we know nothing, which must not be mistaken for "detached".
func TestOrbBoardWithoutOrb(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if id, vidPID, attached, known := orbDevice(DefaultUSBIDs); id != "" || vidPID != "" || attached || known {
		t.Fatalf("orbDevice() = (%q, %q, %v, %v), want empty and unknown", id, vidPID, attached, known)
	}
	u := DetectUSB(USBSharedVM, DaemonIdentity{OperatingSystem: "OrbStack"}, FPGAProfile)
	if u.DeviceDetached() {
		t.Fatal("not being able to ask is not the same as detached")
	}
}

// The VM's node is root:root 0660, so without the root group the container
// user cannot open the board at all -- passthrough was wired up and then
// silently blocked on permissions.
func TestSharedVMJoinsRootGroup(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	u := DetectUSB(USBSharedVM, DaemonIdentity{OperatingSystem: "OrbStack"}, FPGAProfile)
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

// The point of the profile: an Arduino must not be described as an FPGA, and
// must not be handed openFPGALoader's udev rules or a bitstream command.
func TestNonFPGAProfileDropsFPGAVocabulary(t *testing.T) {
	arduino := Profile("Arduino", []USBID{{"2341", "0043"}})

	cases := []USBDevices{
		{Mode: USBHostDevices, Supported: true, BusDir: true, Profile: arduino},
		{Mode: USBHostDevices, Supported: true, BusDir: true, Profile: arduino,
			DeviceNode: "/dev/bus/usb/001/007", DeviceGID: 0},
		{Mode: USBUnavailable, Profile: arduino,
			Identity: DaemonIdentity{OperatingSystem: "Docker Desktop"}},
		{Mode: USBUnavailable, Profile: arduino},
		{Mode: USBUnavailable, Profile: arduino, Identity: DaemonIdentity{Remote: true}},
		{Mode: USBSharedVM, Supported: true, BusDir: true, Profile: arduino,
			AttachKnown: true, DeviceID: "00100000", DeviceVidPID: "2341:0043"},
	}

	for _, u := range cases {
		got := usbAdvice(u, false, "Docker on macOS")
		if got == "" {
			continue
		}
		for _, banned := range []string{"FPGA", "openFPGALoader", "bitstream", "lab-bc", "iverilog", "cocotb"} {
			if strings.Contains(got, banned) {
				t.Fatalf("advice for an Arduino mentions %q:\n%s", banned, got)
			}
		}
		if !strings.Contains(got, "Arduino") {
			t.Fatalf("advice should name the device:\n%s", got)
		}
	}
}

// Overriding ids alone is the "my programmer is not in the list" case, so the
// FPGA wording and the openFPGALoader udev link should survive.
func TestIDOverrideAloneKeepsFPGAProfile(t *testing.T) {
	p := Profile("", []USBID{{"1d50", "6018"}})

	if p.Label != FPGAProfile.Label {
		t.Fatalf("Label = %q, want the FPGA default", p.Label)
	}
	if p.UdevRules == "" || p.FlashExample == "" {
		t.Fatal("openFPGALoader hints should survive an id-only override")
	}
	if len(p.IDs) != 1 || p.IDs[0].String() != "1d50:6018" {
		t.Fatalf("IDs = %v, want only the override", p.IDs)
	}
}

// A custom label means different hardware, so FPGA-specific hints must go.
func TestLabelOverrideDropsFPGAHints(t *testing.T) {
	p := Profile("Arduino", nil)

	if p.UdevRules != "" || p.FlashExample != "" {
		t.Fatal("openFPGALoader hints must not follow a relabelled device")
	}
	if len(p.IDs) == 0 {
		t.Fatal("a label alone should keep the default ids")
	}
}

// A zero profile is an uninitialised struct, not a device with no name.
func TestZeroProfileFallsBackWholesale(t *testing.T) {
	got := usbAdvice(USBDevices{Mode: USBHostDevices, Supported: true, BusDir: true,
		DeviceNode: "/dev/bus/usb/001/007", DeviceGID: 0}, false, "this host")

	if !strings.Contains(got, "99-openfpgaloader.rules") {
		t.Fatalf("an unset profile should behave exactly like FPGAProfile:\n%s", got)
	}
}
