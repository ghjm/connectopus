package config

import (
	"github.com/ghjm/connectopus/pkg/proto"
	"gopkg.in/yaml.v3"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
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
	Address    proto.IP             `yaml:"address"`
	Backends   map[string]Params    `yaml:"backends,omitempty"`
	Services   map[string]Service   `yaml:"services,omitempty"`
	TunDevs    map[string]TunDev    `yaml:"tun_devs,omitempty"`
	Namespaces map[string]Namespace `yaml:"namespaces,omitempty"`
	Cpctl      Cpctl                `yaml:"cpctl,omitempty"`
}

type Service struct {
	Port       int    `yaml:"port"`
	Command    string `yaml:"command"`
	WinCommand string `yaml:"win_command"`
}

type TunDev struct {
	DeviceName string   `yaml:"device"`
	Address    proto.IP `yaml:"address"`
	Cost       float32  `yaml:"cost"`
}

type Namespace struct {
	Address proto.IP `yaml:"address"`
	Cost    float32  `yaml:"cost"`
}

type Cpctl struct {
	SocketFile string `yaml:"socket_file"`
	NoSocket   bool   `yaml:"no_socket"`
	Port       int    `yaml:"port"`
}

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

func ParseConfig(data []byte) (*Config, error) {
	config := &Config{}
	err := yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func LoadConfig(filename string) (*Config, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return ParseConfig(data)
}
