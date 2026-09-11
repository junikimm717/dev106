package host

import (
	"fmt"
	"strings"
)

// Every user-facing string the host checks can print. Kept together so the
// wording stays consistent and the probing code stays readable.

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

// StaleContainerAdvice fires when the board showed up after creation, since
// the device list is fixed then.
func StaleContainerAdvice(u USBDevices, containerHasUSB bool) string {
	if !u.Supported || !u.BoardFound() || containerHasUSB {
		return ""
	}
	return `warning: this container was created without the FPGA board attached.

  The board is plugged in now, but a container's devices are fixed when it
  is created. Pick it up with:
      dev106 restart

`
}

// workspaceAdvice keys off the filesystem type rather than the WSL detection,
// which only picks wording: a Windows drive is 9p or virtiofs regardless of
// what /proc/version claims, and the Linux VHD is ext4 regardless.
func workspaceAdvice(fsType int64, wsl bool, root string) string {
	windowsDrive := fsType == magic9P ||
		(fsType == magicFUSE && wsl && isWindowsDrivePath(root))

	if windowsDrive {
		transport := "9p"
		if fsType == magicFUSE {
			transport = "virtiofs"
		}
		return fmt.Sprintf(`warning: %s is on a Windows drive, not your Linux filesystem.

  WSL reaches Windows files over %s, so every open and stat becomes a
  round trip out of the VM. Builds are many times slower, chown does not
  stick (so dev106 cannot match your UID/GID), and file watching is dead.

  Clone into your Linux home instead:
      cd ~ && git clone <url>

`, root, transport)
	}

	switch fsType {
	case magicNFS, magicCIFS, magicSMB2:
		return fmt.Sprintf(`warning: %s is on a network filesystem.

  Builds will be slow and file ownership may not survive, so dev106 may not
  be able to match your UID/GID. A local disk is strongly preferred.

`, root)
	}

	return ""
}

// DockerHostWarning fires when DOCKER_HOST points over TCP under WSL, where
// Docker Desktop's bind mount translation lives behind the distro's unix
// socket and a TCP connection skips it.
func DockerHostWarning(dockerHost string) string {
	if !strings.HasPrefix(dockerHost, "tcp://") || !IsWSL() {
		return ""
	}
	return fmt.Sprintf(`warning: DOCKER_HOST is %s

  On WSL, connecting over TCP bypasses Docker Desktop's bind mount
  translation, so /workspace would come up empty instead of erroring.
  Unless you mean to use a remote daemon, unset it:
      unset DOCKER_HOST

`, dockerHost)
}
