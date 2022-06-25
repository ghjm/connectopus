//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || zos

package termios

import (
	"fmt"
	"golang.org/x/sys/unix"
	"syscall"
)

func SaveTermios() (any, error) {
	termios, err := unix.IoctlGetTermios(syscall.Stdin, ioctlReadTermios)
	if err != nil {
		return nil, err
	}
	return termios, nil
}

func RestoreTermios(termios any) error {
	termiosValue, ok := termios.(*unix.Termios)
	if !ok {
		return fmt.Errorf("invalid data type for termios")
	}
	return unix.IoctlSetTermios(syscall.Stdin, ioctlWriteTermios, termiosValue)
}
