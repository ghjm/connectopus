package packet_publisher

import (
	"context"
	"github.com/ghjm/connectopus/pkg/x/broker"
	log "github.com/sirupsen/logrus"
	"io"
)

type Publisher struct {
	broker broker.Broker[[]byte]
}

func New(ctx context.Context, reader io.ReadCloser, mtu int) *Publisher {
	p := &Publisher{
		broker: broker.New[[]byte](ctx),
	}
	go func() {
		<-ctx.Done()
		_ = reader.Close()
	}()
	go func() {
		for {
			packet := make([]byte, mtu)
			n, err := reader.Read(packet)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				log.Errorf("error reading from stack endpoint fd: %s", err)
			}
			packet = packet[:n]
			p.broker.Publish(packet)
		}
	}()
	return p
}

func (p *Publisher) SubscribePackets() <-chan []byte {
	return p.broker.Subscribe()
}

func (p *Publisher) UnsubscribePackets(pktCh <-chan []byte) {
	p.broker.Unsubscribe(pktCh)
}
