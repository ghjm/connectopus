//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || zos)

package termios

func SaveTermios() (any, error) {
	return nil, nil
}

func RestoreTermios(_ any) error {
	return nil
}
