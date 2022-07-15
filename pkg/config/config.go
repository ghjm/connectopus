package config

import (
	"fmt"
	"github.com/ghjm/connectopus/pkg/proto"
	"gopkg.in/yaml.v3"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"time"
)

type Config struct {
	Global Global          `yaml:"global"`
	Nodes  map[string]Node `yaml:"nodes"`
}

type Global struct {
	Domain         string                       `yaml:"domain"`
	Subnet         proto.Subnet                 `yaml:"subnet"`
	AuthorizedKeys []proto.MarshalablePublicKey `yaml:"authorized_keys"`
	LastUpdated    time.Time                    `yaml:"last_updated,omitempty"`
}

type Node struct {
	Address    proto.IP             `yaml:"address"`
	Backends   map[string]Params    `yaml:"backends,omitempty"`
	Services   map[string]Service   `yaml:"services,omitempty"`
	TunDevs    map[string]TunDev    `yaml:"tun_devs,omitempty"`
	Namespaces map[string]Namespace `yaml:"namespaces,omitempty"`
	Cpctl      Cpctl                `yaml:"cpctl,omitempty"`
	Dns        Dns                  `yaml:"dns,omitempty"`
}

type Service struct {
	Port    int    `yaml:"port"`
	Command string `yaml:"command"`
}

type TunDev struct {
	DeviceName string   `yaml:"device"`
	Address    proto.IP `yaml:"address"`
	Cost       float32  `yaml:"cost,omitempty"`
}

type Namespace struct {
	Address  proto.IP                    `yaml:"address"`
	Cost     float32                     `yaml:"cost,omitempty"`
	Services map[string]NamespaceService `yaml:"services,omitempty"`
}

type NamespaceService struct {
	Command string `yaml:"command"`
}

type Cpctl struct {
	SocketFile string `yaml:"socket_file,omitempty"`
	NoSocket   bool   `yaml:"no_socket,omitempty"`
	Port       int    `yaml:"port,omitempty"`
}

type Dns struct {
	Disable bool `yaml:"disable,omitempty"`
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

func FindDataDirs(identity string) ([]string, error) {
	ucd, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	basePath := path.Join(ucd, "connectopus")
	if identity != "" {
		dirName := path.Join(basePath, identity)
		_, err = os.Stat(dirName)
		if err != nil {
			return nil, fmt.Errorf("could not open data dir: %w", err)
		}
		return []string{dirName}, nil
	}
	var files []fs.FileInfo
	files, err = ioutil.ReadDir(basePath)
	if err != nil {
		return nil, err
	}
	var fileList []string
	for _, file := range files {
		if file.IsDir() {
			fileList = append(fileList, path.Join(basePath, file.Name()))
		}
	}
	return fileList, nil
}

func (c *Config) Unmarshal(data []byte) error {
	return yaml.Unmarshal(data, c)
}

func (c *Config) Marshal() ([]byte, error) {
	return yaml.Marshal(c)
}

func (c *Config) Load(filename string) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	return c.Unmarshal(data)
}

func (c *Config) Save(filename string) error {
	data, err := c.Marshal()
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filename, data, 0600)
}
