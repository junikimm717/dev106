package host

import (
	"fmt"
	"path"
	"strings"
)

// Every user-facing string the host checks can print. Kept together so the
// wording stays consistent and the probing code stays readable.

// usbAdvice explains why flashing will not work, or "" when the board is
// ready. Only flashing is ever blocked, so these stay warnings.
func usbAdvice(u USBDevices, wsl bool, platform string) string {
	p := profileOrDefault(u.Profile)
	if !u.Supported {
		return unavailableAdvice(u, p, platform)
	}

	if u.Mode == USBSharedVM {
		// orb answered: the board is attached and the container can open it.
		if u.AttachKnown && u.DeviceAttached {
			return ""
		}

		if u.DeviceDetached() {
			target := u.DeviceID
			if target == "" {
				target = "<id>"
			}
			which := u.DeviceVidPID
			if which == "" {
				which = FormatUSBIDs(p.IDs)
			}
			return fmt.Sprintf(`warning: the %s (%s) is not attached to OrbStack's VM,
  so the container cannot see it.

  Attach it:
      orb usb attach %s

  %s

`, p.Label, which, target, p.Fallback)
		}

		// orb did not answer, so this is a pointer, not a diagnosis.
		return fmt.Sprintf(`note: OrbStack passes USB through, but your %s has to be
  attached first.

  Check that it is, and attach it if not:
      orb usb list
      orb usb attach <id>

  Attached devices are visible to every container, so this is a one-time
  step per plug-in. Serial adapters are forwarded automatically.

`, p.Label)
	}

	if u.DeviceRootOnly() {
		fix := `  Give your user access with a udev rule from the vendor, then replug it.
`
		if p.UdevRules != "" {
			fix = fmt.Sprintf(`  Install the udev rule on the host and replug it:
      sudo curl -fsSL -o /etc/udev/rules.d/%s \
        %s
      sudo udevadm control --reload-rules && sudo udevadm trigger
`, path.Base(p.UdevRules), p.UdevRules)
		}
		return fmt.Sprintf(`warning: your %s (%s) is owned by root, so the container
  cannot open it.

%s
`, p.Label, u.DeviceNode, fix)
	}

	if !u.DeviceFound() {
		// usbipd wants one ID, so show the commonest rather than the whole
		// list; the line below says what else would have counted.
		example := p.IDs[0].String()
		fix := fmt.Sprintf("  Plug the %s in, then run `dev106 restart`.\n", p.Label)
		if wsl {
			fix = fmt.Sprintf(`  WSL2 does not see USB devices until you attach them from Windows.
  In an admin PowerShell:
      usbipd list
      usbipd bind   --hardware-id %s
      usbipd attach --wsl --hardware-id %s
  Then run `+"`dev106 restart`"+`.
`, example, example)
		}
		return fmt.Sprintf(`warning: no %s is visible to dev106.

  Looked for %s. If your hardware is not one of those, name it in your
  dev106 config:
      usb_ids   = ["vvvv:pppp"]
      usb_label = "what it is"

  %s

%s
`, p.Label, FormatUSBIDs(p.IDs), p.Fallback, fix)
	}

	return ""
}

func unavailableAdvice(u USBDevices, p DeviceProfile, platform string) string {
	switch {
	case u.Identity.Remote:
		return fmt.Sprintf(`warning: your Docker daemon is not on this machine, so it cannot see
  a %s plugged in here.

  %s The device has to be plugged into whatever
  machine runs the daemon.

`, p.Label, p.Fallback)
	case u.Identity.isDockerDesktop():
		return fmt.Sprintf(`warning: Docker Desktop cannot pass your %s into a container.

  %s
  Flashing and UART do not: Docker Desktop reaches USB only over USB/IP,
  which needs a privileged helper container held open for the session and
  does not present the device at /dev/bus/usb. dev106 does not drive that.

  Options, roughly in order of how much trouble they are:
    * Use OrbStack instead, which passes USB through natively.
    * Build here, then flash from a Linux host or VM.

`, p.Label, p.Fallback)
	default:
		flash := "  Build here, then flash from a Linux host or VM.\n"
		if p.FlashExample != "" {
			flash = fmt.Sprintf(`  Build here, then flash from a Linux host or VM:
      %s
`, p.FlashExample)
		}
		return fmt.Sprintf(`warning: %s cannot pass USB devices into containers.

  %s
  Flashing the %s and reading its UART do not.

%s
`, platform, p.Fallback, p.Label, flash)
	}
}

// StaleContainerAdvice fires when the board showed up after creation, since
// the device list is fixed then.
func StaleContainerAdvice(u USBDevices, containerHasUSB bool) string {
	if !u.Supported || !u.DeviceFound() || containerHasUSB {
		return ""
	}
	return fmt.Sprintf(`warning: this container was created without the %s attached.

  It is plugged in now, but a container's devices are fixed when it is
  created. Pick it up with:
      dev106 restart

`, profileOrDefault(u.Profile).Label)
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
