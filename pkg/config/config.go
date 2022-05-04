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
	Subnet string `yaml:"subnet"`
	Domain string `yaml:"domain"`
}

type Node struct {
	Address  string    `yaml:"address"`
	Backends []Backend `yaml:"backends"`
}

type Backend struct {
	BackendType string `yaml:"type"`
	Params      Params `yaml:"params"`
}

type Params map[string]string

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

func (g *Global) SubnetIPNet() (*net.IPNet, error) {
	_, ipnet, err := net.ParseCIDR(g.Subnet)
	if err != nil {
		return nil, err
	}
	return ipnet, nil
}

func (n *Node) AddressIP() (net.IP, error) {
	ip := net.ParseIP(n.Address)
	if ip == nil {
		return nil, fmt.Errorf("error parsing IP address")
	}
	if len(ip) != net.IPv6len {
		return nil, fmt.Errorf("address must be IPv6")
	}
	return ip, nil
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
