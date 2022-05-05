package config

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"net"
	"strconv"
)

type Config struct {
	Global Global          `yaml:"global"`
	Nodes  map[string]Node `yaml:"nodes"`
}

type Global struct {
	Subnet IPNet  `yaml:"subnet"`
	Domain string `yaml:"domain"`
}

type Node struct {
	Address  IP        `yaml:"address"`
	Tun      Tun       `yaml:"tun"`
	Backends []Backend `yaml:"backends"`
	Plugins  []Plugin  `yaml:"plugins"`
}

type Tun struct {
	Name    string `yaml:"name"`
	Address IP     `yaml:"address"`
}

type Backend struct {
	BackendType string `yaml:"type"`
	Params      Params `yaml:"params"`
}

type Plugin struct {
	File   string `yaml:"file"`
	Params Params `yaml:"params"`
}

type Params map[string]string

type IP net.IP

type IPNet net.IPNet

func LoadConfig(filename string) (*Config, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	config := &Config{}
	err = yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func (p Params) GetPort(name string) (uint16, error) {
	valStr, ok := p[name]
	if !ok {
		return 0, fmt.Errorf("missing parameter: %s", name)
	}
	val, err := strconv.ParseUint(valStr, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("error parsing %s: %w", name, err)
	}
	return uint16(val), nil
}

func (p Params) GetHostPort(name string) (net.IP, uint16, error) {
	peer, ok := p[name]
	if !ok {
		return nil, 0, fmt.Errorf("missing parameter: %s", name)
	}
	host, portStr, err := net.SplitHostPort(peer)
	if err != nil {
		return nil, 0, fmt.Errorf("error parsing %s: %w", name, err)
	}
	var port uint64
	port, err = strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, 0, fmt.Errorf("error parsing %s: %w", name, err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		var ips []net.IP
		ips, err = net.LookupIP(host)
		if err != nil {
			return nil, 0, fmt.Errorf("hostname lookup error in %s: %w", name, err)
		}
		if len(ips) == 0 {
			return nil, 0, fmt.Errorf("hostname did not resolve to any IP addresses in %s", name)
		}
		ip = ips[0]
	}
	return ip, uint16(port), nil
}

// UnmarshalYAML unmarshals an IP address from YAML.
func (i *IP) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	err := unmarshal(&s)
	if err != nil {
		return err
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return fmt.Errorf("unmarshal YAML error: invalid IP address: %s", s)
	}
	*i = IP(ip)
	return nil
}

// MarshalYAML marshals an IP address to YAML.
func (i IP) MarshalYAML() (interface{}, error) {
	if i == nil {
		return nil, nil
	} else {
		return net.IP(i).String(), nil
	}
}

// UnmarshalYAML unmarshals an IP subnet from YAML.
func (ipnet *IPNet) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	err := unmarshal(&s)
	if err != nil {
		return err
	}
	var netIpNet *net.IPNet
	_, netIpNet, err = net.ParseCIDR(s)
	if err != nil {
		return fmt.Errorf("unmarshal YAML error: invalid CIDR network: %s", s)
	}
	*ipnet = IPNet(*netIpNet)
	return nil
}

// MarshalYAML marshals an IP subnet to YAML.
func (ipnet *IPNet) MarshalYAML() (interface{}, error) {
	if ipnet == nil {
		return nil, nil
	} else {
		ipn := net.IPNet(*ipnet)
		return ipn.String(), nil
	}
}
