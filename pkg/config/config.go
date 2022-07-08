package config

import (
	"fmt"
	"github.com/ghjm/connectopus/pkg/proto"
	"gopkg.in/yaml.v3"
	"io/fs"
	"io/ioutil"
	"net"
	"os"
	"path"
	"strconv"
)

type Config struct {
	Global Global          `yaml:"global"`
	Nodes  map[string]Node `yaml:"nodes"`
}

type Global struct {
	Domain         string                       `yaml:"domain"`
	Subnet         proto.Subnet                 `yaml:"subnet"`
	AuthorizedKeys []proto.MarshalablePublicKey `yaml:"authorized_keys"`
}

type Node struct {
	Address    proto.IP    `yaml:"address"`
	MTU        uint16      `yaml:"mtu,omitempty"`
	Backends   []Backend   `yaml:"backends,omitempty"`
	Services   []Service   `yaml:"services,omitempty"`
	TunDevs    []TunDev    `yaml:"tun_devs,omitempty"`
	Namespaces []Namespace `yaml:"namespaces,omitempty"`
	Cpctl      Cpctl       `yaml:"cpctl,omitempty"`
}

type Backend struct {
	BackendType string  `yaml:"type"`
	Cost        float32 `yaml:"cost"`
	Params      Params  `yaml:"params"`
}

type Service struct {
	Port       int    `yaml:"port"`
	Command    string `yaml:"command"`
	WinCommand string `yaml:"win_command"`
}

type TunDev struct {
	Name       string   `yaml:"name"`
	DeviceName string   `yaml:"device"`
	Address    proto.IP `yaml:"address"`
	Cost       float32  `yaml:"cost"`
}

type Namespace struct {
	Name    string   `yaml:"name"`
	Address proto.IP `yaml:"address"`
	Cost    float32  `yaml:"cost"`
}

type Cpctl struct {
	SocketFile string `yaml:"socket_file"`
	NoSocket   bool   `yaml:"no_socket"`
	Port       int    `yaml:"port"`
}

type Params map[string]string

func ExpandFilename(nodeID, filename string) (string, error) {
	if path.IsAbs(filename) {
		return filename, nil
	}
	if filename == "" {
		filename = "cpctl.sock"
	}
	ucd, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return path.Join(ucd, "connectopus", nodeID, filename), nil
}

func FindSockets(socketNode string) ([]string, error) {
	ucd, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	basePath := path.Join(ucd, "connectopus")
	if socketNode != "" {
		return []string{path.Join(basePath, socketNode, "cpctl.sock")}, nil
	}
	var files []fs.FileInfo
	files, err = ioutil.ReadDir(basePath)
	if err != nil {
		return nil, err
	}
	var fileList []string
	for _, file := range files {
		if file.IsDir() {
			fullPath := path.Join(basePath, file.Name(), "cpctl.sock")
			_, err = os.Stat(fullPath)
			if err != nil {
				continue
			}
			fileList = append(fileList, fullPath)
		}
	}
	return fileList, nil
}

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

func (p Params) GetIP(name string) (net.IP, error) {
	host, ok := p[name]
	if !ok {
		return nil, fmt.Errorf("missing parameter: %s", name)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address: %s", host)
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

func (p Params) GetBool(name string, defaultValue bool) (bool, error) {
	valStr, ok := p[name]
	if !ok {
		return defaultValue, nil
	}
	val, err := strconv.ParseBool(valStr)
	if err != nil {
		return false, fmt.Errorf("error parsing %s: %w", name, err)
	}
	return val, nil
}
