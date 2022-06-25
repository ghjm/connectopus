//go:build aix || linux || solaris || zos

package termios

import "golang.org/x/sys/unix"

const ioctlReadTermios = unix.TCGETS
const ioctlWriteTermios = unix.TCSETS
