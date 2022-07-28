package proto

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"lukechampine.com/uint128"
	"math/bits"
	"net"
)

// This package attempts to produce "best of both worlds" types representing TCP/IP addresses and subnets,
// combining the desirable qualities from Golang's net package and gVisor's tcpip package.  Specifically,
// the base type of an IP is a string rather than a []byte, meaning IP addresses can be map keys.

// IP represents an IP address (IPv4 or IPv6)
type IP string

// Mask represents a netmask (IPv4 or IPv6)
type Mask string

// Subnet represents an IP subnet (IPv4 or IPv6)
type Subnet string

func isZeros(p []byte) bool {
	for i := 0; i < len(p); i++ {
		if p[i] != 0 {
			return false
		}
	}
	return true
}

// ParseIP parses a human-readable address and returns an IP.  Unlike net.IP, if the address is ipv4, the returned
// IP will be in ipv4 format (as if .To4() had been called on it).
func ParseIP(s string) IP {
	ip := []byte(net.ParseIP(s))
	if len(ip) == net.IPv6len &&
		isZeros(ip[0:10]) &&
		ip[10] == 0xff &&
		ip[11] == 0xff {
		ip = ip[12:16]
	}
	return IP(ip)
}

// ParseCIDR parses an IP and subnet in CIDR notation.
func ParseCIDR(s string) (IP, Subnet, error) {
	netIP, netSubnet, err := net.ParseCIDR(s)
	if err != nil {
		return "", "", err
	}
	ip := IP(netIP)
	subnet := Subnet(string(netSubnet.IP) + string(netSubnet.Mask))
	return ip, subnet, nil
}

// RandomSubnet returns a subnet with an address starting with keepBits bits of IP and random for the rest of prefixBits
func RandomSubnet(ip IP, keepBits uint, prefixBits uint) Subnet {
	ipb := []byte(ip)
	kv := uint128.New(
		binary.BigEndian.Uint64(ipb[8:]),
		binary.BigEndian.Uint64(ipb[:8]),
	)
	km := uint128.Max.Rsh(keepBits)
	kv = kv.And(km.Xor(uint128.Max))

	randb := make([]byte, 16)
	_, _ = rand.Read(randb)
	rv := uint128.FromBytes(randb)
	rm := uint128.Max.Lsh(128 - prefixBits)
	rv = rv.And(rm).And(km)

	ipv := kv.Add(rv)

	ipBytes := make([]byte, 16)
	binary.BigEndian.PutUint64(ipBytes[8:], ipv.Lo)
	binary.BigEndian.PutUint64(ipBytes[:8], ipv.Hi)

	return NewSubnet(IP(ipBytes), CIDRMask(int(prefixBits), 128))
}

// String returns the human-readable string representation of this address.
//goland:noinspection GoMixedReceiverTypes
func (a IP) String() string {
	return net.IP(a).String()
}

// DebugString returns a representation to show in debuggers.  Gor compatibility with GoLand this function must
// be able to be statically evaluated, so it cannot call other functions.
//goland:noinspection GoMixedReceiverTypes
func (a IP) DebugString() string {
	return fmt.Sprintf("IP %x", string(a))
}

// Equal returns true if the two IPs are equivalent, including IPv6 address equivalencies to IPv4.
//goland:noinspection GoMixedReceiverTypes
func (a IP) Equal(b IP) bool {
	return net.IP(a).Equal(net.IP(b))
}

// MarshalJSON marshals an IP address to JSON.
//goland:noinspection GoMixedReceiverTypes
func (a IP) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

// UnmarshalJSON unmarshals an IP address from JSON.
//goland:noinspection GoMixedReceiverTypes
func (a *IP) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	ip := ParseIP(s)
	if ip == "" {
		return fmt.Errorf("invalid IP address")
	}
	*a = ip
	return nil
}

// MarshalYAML marshals an IP address to YAML.
//goland:noinspection GoMixedReceiverTypes
func (a IP) MarshalYAML() (interface{}, error) {
	if a == "" {
		return nil, nil
	}
	return a.String(), nil
}

// UnmarshalYAML unmarshals an IP address from YAML.
//goland:noinspection GoMixedReceiverTypes
func (a *IP) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	err := unmarshal(&s)
	if err != nil {
		return err
	}
	ip := ParseIP(s)
	if ip == "" {
		return fmt.Errorf("unmarshal YAML error: invalid IP address")
	}
	*a = ip
	return nil
}

func CIDRMask(ones int, bits int) Mask {
	return Mask(net.CIDRMask(ones, bits))
}

// String returns the human-readable string representation of this netmask.
func (m Mask) String() string {
	return net.IPMask(m).String()
}

// DebugString returns a representation to show in debuggers.  Gor compatibility with GoLand this function must
// be able to be statically evaluated, so it cannot call other functions.
//goland:noinspection GoMixedReceiverTypes
func (m Mask) DebugString() string {
	return fmt.Sprintf("Mask %x", string(m))
}

// Prefix returns the number of leading 1 bits in the netmask
func (m Mask) Prefix() int {
	p := 0
	for _, b := range []byte(m) {
		p += bits.LeadingZeros8(^b)
	}
	return p
}

// NewSubnet creates a subnet from an address and mask, masking the host bits of the address.  It is
// up to the caller to ensure a and m are the same length - if they are not, an invalid subnet will be returned.
func NewSubnet(a IP, m Mask) Subnet {
	if len(a) != len(m) {
		return ""
	}
	netAddr := make([]byte, len(a))
	for i := 0; i < len(a); i++ {
		netAddr[i] = a[i] & m[i]
	}
	return Subnet(string(netAddr) + string(m))
}

// NewHostOnlySubnet creates a subnet containing only a single IP address (ie, a /32 for IPv4 or a /128 for IPv6).
func NewHostOnlySubnet(a IP) Subnet {
	return Subnet(string(a) + string(net.CIDRMask(len(a)*8, len(a)*8)))
}

// String returns the human-readable string representation of this subnet.
//goland:noinspection GoMixedReceiverTypes
func (s Subnet) String() string {
	if len(s) == 0 {
		return "<nil>"
	}
	if len(s) != 2*net.IPv4len && len(s) != 2*net.IPv6len {
		return "<invalid>"
	}
	return s.AsIPNet().String()
}

// DebugString returns a representation to show in debuggers.  Gor compatibility with GoLand this function must
// be able to be statically evaluated, so it cannot call other functions.
//goland:noinspection GoMixedReceiverTypes
func (s Subnet) DebugString() string {
	return fmt.Sprintf("Subnet %x", string(s))
}

// IP returns the IP address (network ID) of the subnet
//goland:noinspection GoMixedReceiverTypes
func (s Subnet) IP() IP {
	if len(s) != 2*net.IPv4len && len(s) != 2*net.IPv6len {
		return ""
	}
	return IP(s[:len(s)/2])
}

// Mask returns the netmask of the subnet
//goland:noinspection GoMixedReceiverTypes
func (s Subnet) Mask() Mask {
	if len(s) != 2*net.IPv4len && len(s) != 2*net.IPv6len {
		return ""
	}
	return Mask(s[len(s)/2:])
}

// AsIPNet returns the subnet as a *net.IPNet
//goland:noinspection GoMixedReceiverTypes
func (s Subnet) AsIPNet() *net.IPNet {
	return &net.IPNet{
		IP:   net.IP(s.IP()),
		Mask: net.IPMask(s.Mask()),
	}
}

// Contains returns true if the IP is contained within the subnet
//goland:noinspection GoMixedReceiverTypes
func (s Subnet) Contains(a IP) bool {
	return s.AsIPNet().Contains(net.IP(a))
}

// Prefix returns the prefix length of the subnet's netmask
//goland:noinspection GoMixedReceiverTypes
func (s Subnet) Prefix() int {
	return s.Mask().Prefix()
}

// MarshalJSON marshals an IP subnet to JSON.
//goland:noinspection GoMixedReceiverTypes
func (s Subnet) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON unmarshals an IP subnet from JSON.
//goland:noinspection GoMixedReceiverTypes
func (s *Subnet) UnmarshalJSON(data []byte) error {
	var str string
	err := json.Unmarshal(data, &str)
	if err != nil {
		return err
	}
	var subnet Subnet
	_, subnet, err = ParseCIDR(str)
	if err != nil {
		return fmt.Errorf("unmarshal JSON error: invalid subnet")
	}
	*s = subnet
	return nil
}

// MarshalYAML marshals an IP subnet to YAML.
//goland:noinspection GoMixedReceiverTypes
func (s Subnet) MarshalYAML() (interface{}, error) {
	if s == "" {
		return nil, nil
	}
	return s.String(), nil
}

// UnmarshalYAML unmarshals a subnet from YAML.
//goland:noinspection GoMixedReceiverTypes
func (s *Subnet) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	err := unmarshal(&str)
	if err != nil {
		return err
	}
	var subnet Subnet
	_, subnet, err = ParseCIDR(str)
	if err != nil {
		return fmt.Errorf("unmarshal YAML error: invalid subnet")
	}
	*s = subnet
	return nil
}
