package packet_publisher

import (
	"context"
	"github.com/ghjm/connectopus/pkg/x/broker"
	"github.com/ghjm/connectopus/pkg/x/chanreader"
	log "github.com/sirupsen/logrus"
	"io"
)

type Publisher struct {
	broker broker.Broker[[]byte]
}

func New(ctx context.Context, reader io.ReadCloser, mods ...func(chanReader *chanreader.ChanReader)) *Publisher {
	p := &Publisher{
		broker: broker.New[[]byte](ctx),
	}
	mods = append(mods, chanreader.WithErrorFunc(func(e error) {
		log.Errorf("packet read error: %s", e)
	}))
	cr := chanreader.New(ctx, reader, mods...)
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
