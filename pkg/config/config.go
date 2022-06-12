package config

import (
	"fmt"
	"github.com/ghjm/connectopus/pkg/proto"
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
	Domain string       `yaml:"domain"`
	Subnet proto.Subnet `yaml:"subnet"`
}

type Node struct {
	Address    proto.IP    `yaml:"address"`
	Tun        Tun         `yaml:"tun,omitempty"`
	Backends   []Backend   `yaml:"backends,omitempty"`
	Services   []Service   `yaml:"services,omitempty"`
	Namespaces []Namespace `yaml:"namespaces,omitempty"`
	Cpctl      Cpctl       `yaml:"cpctl,omitempty"`
}

type Tun struct {
	Name    string   `yaml:"name"`
	Address proto.IP `yaml:"address"`
}

type Backend struct {
	BackendType string `yaml:"type"`
	Params      Params `yaml:"params"`
}

type Service struct {
	Port       int    `yaml:"port"`
	Command    string `yaml:"command"`
	WinCommand string `yaml:"win_command"`
}

type Namespace struct {
	Name    string   `yaml:"name"`
	Address proto.IP `yaml:"address"`
}

type Cpctl struct {
	Enable     bool   `yaml:"enable"`
	SocketFile string `yaml:"socket_file"`
	Port       int    `yaml:"port"`
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
