package netns

import "github.com/ghjm/connectopus/pkg/netopus/link/packet_publisher"

// Link implements link.Link for a local private network namespace
type Link struct {
	packet_publisher.Publisher
	shimFd int
	pid    int
}
