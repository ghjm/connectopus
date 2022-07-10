package config

import (
	"fmt"
	"net"
	"strconv"
)

type Params map[string]string

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

func (p Params) GetString(name string, defaultValue string) string {
	valStr, ok := p[name]
	if !ok {
		return defaultValue
	}
	return valStr
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

func (p Params) GetFloat32(name string, defaultValue float32) (float32, error) {
	valStr, ok := p[name]
	if !ok {
		return defaultValue, nil
	}
	val, err := strconv.ParseFloat(valStr, 32)
	if err != nil {
		return 0, fmt.Errorf("error parsing %s: %w", name, err)
	}
	return float32(val), nil
}
