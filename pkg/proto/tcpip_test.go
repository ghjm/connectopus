package proto

import (
	"net"
	"testing"
)

/*
Functions to test:

func ParseCIDR(s string) (IP, Subnet, error)
func (a IP) String() string
func (a IP) Equal(b IP) bool
func (a IP) MarshalYAML() (interface{}, error)
func (a *IP) UnmarshalYAML(unmarshal func(interface{}) error) error
func CIDRMask(ones int, bits int) Mask
func (m Mask) String() string
func (m Mask) Prefix() int
func NewSubnet(a IP, m Mask) Subnet
func NewHostOnlySubnet(a IP) Subnet
func (s Subnet) String() string
func (s Subnet) IP() IP
func (s Subnet) Mask() Mask
func (s Subnet) AsIPNet() *net.IPNet
func (s Subnet) Contains(a IP) bool
func (s Subnet) Prefix() int
func (s Subnet) MarshalYAML() (interface{}, error)
func (s *Subnet) UnmarshalYAML(unmarshal func(interface{}) error) error
*/

var testIPv4 = "192.168.0.1"
var testIPv6 = "fd00::1:2:3"

func TestTcpip(t *testing.T) {
	if !net.ParseIP(testIPv4).Equal(net.IP(ParseIP(testIPv4))) {
		t.Fail()
	}
	if !net.ParseIP(testIPv6).Equal(net.IP(ParseIP(testIPv6))) {
		t.Fail()
	}
}
