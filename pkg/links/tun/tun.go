package tun

import (
	"context"
	"fmt"
	"github.com/ghjm/connectopus/pkg/x/packet_publisher"
	"io"
)

var ErrNotImplemented = fmt.Errorf("not implemented on this platform")

type Link struct {
	packet_publisher.Publisher
	ctx    context.Context
	tunRWC io.ReadWriteCloser
}

func (l *Link) SendPacket(packet []byte) error {
	n, err := l.tunRWC.Write(packet)
	if err != nil {
		return err
	}
	if n != len(packet) {
		return fmt.Errorf("tun device only wrote %d bytes of %d", n, len(packet))
	}
	return nil
}
