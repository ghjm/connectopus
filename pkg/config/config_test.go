package config

import (
	"io/ioutil"
	"os"
	"reflect"
	"testing"
)

var testYaml = `---
global:
  subnet: FD00::0/8

nodes:

  foo:
    address: FD00::1
    backends:
      - type: dtls-dialer
        params:
          peer: localhost:4444

  bar:
    address: FD00::2
    backends:
      - type: dtls-listener
        params:
          port: 4444

  baz:
    address: FD00::3
    backends:
      - type: dtls-dialer
        params:
          peer: localhost:4444

`

var correctConfig = Config{
	Global: Global{
		Subnet: "FD00::0/8",
	},
	Nodes: map[string]Node{
		"foo": {
			Address: "FD00::1",
			Backends: []Backend{
				{
					BackendType: "dtls-dialer",
					Params: map[string]string{
						"peer": "localhost:4444",
					},
				},
			},
		},
		"bar": {
			Address: "FD00::2",
			Backends: []Backend{
				{
					BackendType: "dtls-listener",
					Params: map[string]string{
						"port": "4444",
					},
				},
			},
		},
		"baz": {
			Address: "FD00::3",
			Backends: []Backend{
				{
					BackendType: "dtls-dialer",
					Params: map[string]string{
						"peer": "localhost:4444",
					},
				},
			},
		},
	},
}

func TestConfig(t *testing.T) {
	configFile, err := ioutil.TempFile("", "configtest")
	if err != nil {
		t.Fatalf("error creating tempfile: %s", err)
	}
	defer func() {
		err := os.Remove(configFile.Name())
		if err != nil {
			t.Fatalf("error deleting tempfile: %s", err)
		}
	}()
	_, err = configFile.WriteString(testYaml)
	if err != nil {
		t.Fatalf("error writing to tempfile: %s", err)
	}
	err = configFile.Close()
	if err != nil {
		t.Fatalf("error closing tempfile: %s", err)
	}
	var config *Config
	config, err = LoadConfig(configFile.Name())
	if err != nil {
		t.Fatalf("error loading tempfile: %s", err)
	}
	if !reflect.DeepEqual(config, &correctConfig) {
		t.Fatalf("config loaded incorrectly")
	}
}
