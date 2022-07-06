//go:build windows

package ssh_jwt

import (
	"github.com/kbolino/pageant"
	"io"
)

func pageantNewConn() (io.ReadWriteCloser, error) {
	return pageant.NewConn()
}
