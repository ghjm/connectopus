// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package cpctl

type ConfigResult struct {
	Yaml      string `json:"yaml"`
	Signature string `json:"signature"`
}

type ConfigUpdateInput struct {
	Yaml      string `json:"yaml"`
	Signature string `json:"signature"`
}

type ConfigUpdateResult struct {
	MutationID string `json:"mutationId"`
}

type NetnsFilter struct {
	Name *string `json:"name"`
}

type NetnsResult struct {
	Name string `json:"name"`
	Pid  int    `json:"pid"`
}

type Status struct {
	Name     string           `json:"name"`
	Addr     string           `json:"addr"`
	Global   *StatusGlobal    `json:"global"`
	Nodes    []*StatusNode    `json:"nodes"`
	Sessions []*StatusSession `json:"sessions"`
}

type StatusGlobal struct {
	Domain         string   `json:"domain"`
	Subnet         string   `json:"subnet"`
	AuthorizedKeys []string `json:"authorized_keys"`
}

type StatusNode struct {
	Name  string            `json:"name"`
	Addr  string            `json:"addr"`
	Conns []*StatusNodeConn `json:"conns"`
}

type StatusNodeConn struct {
	Subnet string  `json:"subnet"`
	Cost   float64 `json:"cost"`
}

type StatusSession struct {
	Addr      string `json:"addr"`
	Connected bool   `json:"connected"`
	ConnStart string `json:"conn_start"`
}
