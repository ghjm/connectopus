package proto

import (
	"encoding/json"
	"gopkg.in/yaml.v3"
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

func TestIPMarshaling(t *testing.T) {
	for _, ipStr := range []string{testIPv4, testIPv6} {
		// roundtrip through regular parse
		ip := ParseIP(ipStr)
		if ip.String() != ipStr {
			t.Fatal("did not match")
		}

		// roundtrip through JSON
		ipJSON, err := json.Marshal(ip)
		if err != nil {
			t.Fatal(err)
		}
		jip := new(IP)
		err = json.Unmarshal(ipJSON, &jip)
		if err != nil {
			t.Fatal(err)
		}
		if jip.String() != ipStr {
			t.Fatal("did not match")
		}

		// roundtrip through YAML
		ipYAML, err := yaml.Marshal(ip)
		if err != nil {
			t.Fatal(err)
		}
		yip := new(IP)
		err = yaml.Unmarshal(ipYAML, &yip)
		if err != nil {
			t.Fatal(err)
		}
		if yip.String() != ipStr {
			t.Fatal("did not match")
		}
	}
}

func TestSubnetMarshaling(t *testing.T) {
	for _, ipStr := range []string{testIPv4, testIPv6} {
		ip := ParseIP(ipStr)
		subnet := NewSubnet(ip, CIDRMask(16, len(ip)*8))
		subnetStr := subnet.String()

		// roundtrip through regular parse
		_, subnet2, err := ParseCIDR(subnetStr)
		if err != nil {
			t.Fatal(err)
		}
		if subnet2.String() != subnetStr {
			t.Fatal("did not match")
		}

		// roundtrip through JSON
		sJSON, err := json.Marshal(subnet)
		if err != nil {
			t.Fatal(err)
		}
		js := new(Subnet)
		err = json.Unmarshal(sJSON, &js)
		if err != nil {
			t.Fatal(err)
		}
		if js.String() != subnetStr {
			t.Fatal("did not match")
		}

		// roundtrip through YAML
		sYAML, err := yaml.Marshal(subnet)
		if err != nil {
			t.Fatal(err)
		}
		ys := new(Subnet)
		err = yaml.Unmarshal(sYAML, &ys)
		if err != nil {
			t.Fatal(err)
		}
		if ys.String() != subnetStr {
			t.Fatal("did not match")
		}
	}
}
