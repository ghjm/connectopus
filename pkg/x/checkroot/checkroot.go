//go:build linux

package checkroot

import (
	"github.com/syndtr/gocapability/capability"
	"os"
)

func CheckNetAdmin() bool {
	c, err := capability.NewPid2(0)
	if err != nil {
		return false
	}
	return c.Get(capability.EFFECTIVE, capability.CAP_NET_ADMIN)
}

func CheckRoot() bool {
	return os.Geteuid() == 0
}
