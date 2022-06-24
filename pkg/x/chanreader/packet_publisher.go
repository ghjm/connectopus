package chanreader

import (
	"context"
	"github.com/ghjm/connectopus/pkg/x/broker"
	log "github.com/sirupsen/logrus"
	"io"
)

type Publisher struct {
	broker broker.Broker[[]byte]
}

func NewPublisher(ctx context.Context, reader io.ReadCloser, mods ...func(chanReader *ChanReader)) *Publisher {
	p := &Publisher{
		broker: broker.New[[]byte](ctx),
	}
	mods = append(mods, WithErrorFunc(func(e error) {
		log.Errorf("packet read error: %s", e)
	}))
	cr := New(ctx, reader, mods...)
	go func() {
		for packet := range cr.ReadChan() {
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
