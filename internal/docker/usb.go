package docker

import (
	"github.com/junikimm717/dev106/internal/host"
	"github.com/moby/moby/api/types/container"
)

// applyUSB binds the whole bus rather than the board node, so a replug (which
// changes the device number) does not need a new container.
func applyUSB(hostConfig *container.HostConfig, u host.USBDevices) {
	if !u.Supported || !u.BusDir {
		return
	}

	hostConfig.Binds = append(hostConfig.Binds, host.USBBusDir+":"+host.USBBusDir)
	hostConfig.DeviceCgroupRules = append(hostConfig.DeviceCgroupRules,
		host.USBCgroupRule, host.SerialCgroupRule)

	// On a shared VM the ttyUSB nodes live in the daemon's /dev, which we cannot
	// enumerate from here and which is empty until the board is attached. Bind
	// the directory so nodes appearing later are simply there, rather than
	// naming devices that do not exist yet. u.Serial is macOS cu.usbserial
	// paths in this mode and is empty whenever the board is actually passed
	// through -- attaching it to the VM is what takes it away from macOS.
	if u.Mode == host.USBSharedVM {
		hostConfig.Binds = append(hostConfig.Binds, host.DevDir+":"+host.DevDir)
	}

	// Serial nodes sit outside /dev/bus/usb, so they need mapping in.
	for _, tty := range u.Serial {
		hostConfig.Devices = append(hostConfig.Devices, container.DeviceMapping{
			PathOnHost:        tty,
			PathInContainer:   tty,
			CgroupPermissions: "rwm",
		})
	}

	hostConfig.GroupAdd = append(hostConfig.GroupAdd, u.GroupIDs...)
}
