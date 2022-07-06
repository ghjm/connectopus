//go:build !windows

package ssh_jwt

import (
	"fmt"
	"io"
)

func pageantNewConn() (io.ReadWriteCloser, error) {
	return nil, fmt.Errorf("not implemented")
}
