package tun

type Link interface {
	// SendPacket is called by Netopus to send a package outbound to the tun device
	SendPacket([]byte) error
}
