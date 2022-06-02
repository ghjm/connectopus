package link

// Link is a network stack that accepts and produces IPv6 packets
type Link interface {
	// SendPacket injects a single packet into the network stack.  The data must be a valid IPv6 packet.
	SendPacket(packet []byte) error
	// SubscribePackets returns a channel which will receive packets outgoing from the network stack.
	SubscribePackets() <-chan []byte
	// UnsubscribePackets unsubscribes a channel previously subscribed with SubscribePackets.
	UnsubscribePackets(pktCh <-chan []byte)
}
