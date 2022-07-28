package bridge

import (
	"fmt"
	"io"
	"strings"
)

// NormalBufferSize is the size of buffers used by various processes when copying data between sockets.
const NormalBufferSize = 65536

// RunBridge bridges two connections, like netcat.
func RunBridge(c1, c2 io.ReadWriteCloser) error {
	errChan := make(chan error)
	go func() {
		errChan <- bridgeHalf(c1, c2)
	}()
	go func() {
		errChan <- bridgeHalf(c2, c1)
	}()
	err1 := <-errChan
	err2 := <-errChan
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}

// BridgeHalf bridges the read side of c1 to the write side of c2.
func bridgeHalf(c1 io.Reader, c2 io.WriteCloser) error {
	defer func() {
		_ = c2.Close()
	}()
	buf := make([]byte, NormalBufferSize)
	for {
		n, err := c1.Read(buf)
		if err != nil {
			if err.Error() != "EOF" && !strings.Contains(err.Error(), "use of closed network connection") {
				return fmt.Errorf("connection read error: %w", err)
			}
			return nil
		}
		if n > 0 {
			wn, err := c2.Write(buf[:n])
			if err != nil {
				return fmt.Errorf("connection write error: %w", err)
			}
			if wn != n {
				return fmt.Errorf("not all bytes written")
			}
		}
	}
}
