// Code generated by github.com/Yamashou/gqlgenc, DO NOT EDIT.

package cpctl

import (
	"context"
	"net/http"
	"time"

	"github.com/Yamashou/gqlgenc/clientv2"
)

type Client struct {
	Client *clientv2.Client
}

func NewClient(cli *http.Client, baseURL string, options *clientv2.Options, interceptors ...clientv2.RequestInterceptor) *Client {
	return &Client{Client: clientv2.NewClient(cli, baseURL, options, interceptors...)}
}

type Query struct {
	Netns  []*NetnsResult "json:\"netns\" graphql:\"netns\""
	Status Status         "json:\"status\" graphql:\"status\""
	Config ConfigResult   "json:\"config\" graphql:\"config\""
}
type Mutation struct {
	UpdateConfig ConfigUpdateResult "json:\"updateConfig\" graphql:\"updateConfig\""
}
type GetConfig_Config struct {
	Yaml      string "json:\"yaml\" graphql:\"yaml\""
	Signature string "json:\"signature\" graphql:\"signature\""
}

func (t *GetConfig_Config) GetYaml() string {
	if t == nil {
		t = &GetConfig_Config{}
	}
	return t.Yaml
}
func (t *GetConfig_Config) GetSignature() string {
	if t == nil {
		t = &GetConfig_Config{}
	}
	return t.Signature
}

type SetConfig_UpdateConfig struct {
	MutationID string "json:\"mutationId\" graphql:\"mutationId\""
}

func (t *SetConfig_UpdateConfig) GetMutationID() string {
	if t == nil {
		t = &SetConfig_UpdateConfig{}
	}
	return t.MutationID
}

type GetNetns_Netns struct {
	Name string "json:\"name\" graphql:\"name\""
	Pid  int    "json:\"pid\" graphql:\"pid\""
}

func (t *GetNetns_Netns) GetName() string {
	if t == nil {
		t = &GetNetns_Netns{}
	}
	return t.Name
}
func (t *GetNetns_Netns) GetPid() int {
	if t == nil {
		t = &GetNetns_Netns{}
	}
	return t.Pid
}

type GetStatus_Status_Global struct {
	Domain            string    "json:\"domain\" graphql:\"domain\""
	Subnet            string    "json:\"subnet\" graphql:\"subnet\""
	AuthorizedKeys    []string  "json:\"authorized_keys\" graphql:\"authorized_keys\""
	ConfigLastUpdated time.Time "json:\"config_last_updated\" graphql:\"config_last_updated\""
}

func (t *GetStatus_Status_Global) GetDomain() string {
	if t == nil {
		t = &GetStatus_Status_Global{}
	}
	return t.Domain
}
func (t *GetStatus_Status_Global) GetSubnet() string {
	if t == nil {
		t = &GetStatus_Status_Global{}
	}
	return t.Subnet
}
func (t *GetStatus_Status_Global) GetAuthorizedKeys() []string {
	if t == nil {
		t = &GetStatus_Status_Global{}
	}
	return t.AuthorizedKeys
}
func (t *GetStatus_Status_Global) GetConfigLastUpdated() *time.Time {
	if t == nil {
		t = &GetStatus_Status_Global{}
	}
	return &t.ConfigLastUpdated
}

type GetStatus_Status_Nodes_Conns struct {
	Subnet string  "json:\"subnet\" graphql:\"subnet\""
	Cost   float64 "json:\"cost\" graphql:\"cost\""
}

func (t *GetStatus_Status_Nodes_Conns) GetSubnet() string {
	if t == nil {
		t = &GetStatus_Status_Nodes_Conns{}
	}
	return t.Subnet
}
func (t *GetStatus_Status_Nodes_Conns) GetCost() float64 {
	if t == nil {
		t = &GetStatus_Status_Nodes_Conns{}
	}
	return t.Cost
}

type GetStatus_Status_Nodes struct {
	Name  string                          "json:\"name\" graphql:\"name\""
	Addr  string                          "json:\"addr\" graphql:\"addr\""
	Conns []*GetStatus_Status_Nodes_Conns "json:\"conns\" graphql:\"conns\""
}

func (t *GetStatus_Status_Nodes) GetName() string {
	if t == nil {
		t = &GetStatus_Status_Nodes{}
	}
	return t.Name
}
func (t *GetStatus_Status_Nodes) GetAddr() string {
	if t == nil {
		t = &GetStatus_Status_Nodes{}
	}
	return t.Addr
}
func (t *GetStatus_Status_Nodes) GetConns() []*GetStatus_Status_Nodes_Conns {
	if t == nil {
		t = &GetStatus_Status_Nodes{}
	}
	return t.Conns
}

type GetStatus_Status_Sessions struct {
	Addr      string "json:\"addr\" graphql:\"addr\""
	Connected bool   "json:\"connected\" graphql:\"connected\""
	ConnStart string "json:\"conn_start\" graphql:\"conn_start\""
}

func (t *GetStatus_Status_Sessions) GetAddr() string {
	if t == nil {
		t = &GetStatus_Status_Sessions{}
	}
	return t.Addr
}
func (t *GetStatus_Status_Sessions) GetConnected() bool {
	if t == nil {
		t = &GetStatus_Status_Sessions{}
	}
	return t.Connected
}
func (t *GetStatus_Status_Sessions) GetConnStart() string {
	if t == nil {
		t = &GetStatus_Status_Sessions{}
	}
	return t.ConnStart
}

type GetStatus_Status struct {
	Name     string                       "json:\"name\" graphql:\"name\""
	Addr     string                       "json:\"addr\" graphql:\"addr\""
	Global   GetStatus_Status_Global      "json:\"global\" graphql:\"global\""
	Nodes    []*GetStatus_Status_Nodes    "json:\"nodes\" graphql:\"nodes\""
	Sessions []*GetStatus_Status_Sessions "json:\"sessions\" graphql:\"sessions\""
}

func (t *GetStatus_Status) GetName() string {
	if t == nil {
		t = &GetStatus_Status{}
	}
	return t.Name
}
func (t *GetStatus_Status) GetAddr() string {
	if t == nil {
		t = &GetStatus_Status{}
	}
	return t.Addr
}
func (t *GetStatus_Status) GetGlobal() *GetStatus_Status_Global {
	if t == nil {
		t = &GetStatus_Status{}
	}
	return &t.Global
}
func (t *GetStatus_Status) GetNodes() []*GetStatus_Status_Nodes {
	if t == nil {
		t = &GetStatus_Status{}
	}
	return t.Nodes
}
func (t *GetStatus_Status) GetSessions() []*GetStatus_Status_Sessions {
	if t == nil {
		t = &GetStatus_Status{}
	}
	return t.Sessions
}

type GetConfig struct {
	Config GetConfig_Config "json:\"config\" graphql:\"config\""
}

func (t *GetConfig) GetConfig() *GetConfig_Config {
	if t == nil {
		t = &GetConfig{}
	}
	return &t.Config
}

type SetConfig struct {
	UpdateConfig SetConfig_UpdateConfig "json:\"updateConfig\" graphql:\"updateConfig\""
}

func (t *SetConfig) GetUpdateConfig() *SetConfig_UpdateConfig {
	if t == nil {
		t = &SetConfig{}
	}
	return &t.UpdateConfig
}

type GetNetns struct {
	Netns []*GetNetns_Netns "json:\"netns\" graphql:\"netns\""
}

func (t *GetNetns) GetNetns() []*GetNetns_Netns {
	if t == nil {
		t = &GetNetns{}
	}
	return t.Netns
}

type GetStatus struct {
	Status GetStatus_Status "json:\"status\" graphql:\"status\""
}

func (t *GetStatus) GetStatus() *GetStatus_Status {
	if t == nil {
		t = &GetStatus{}
	}
	return &t.Status
}

const GetConfigDocument = `query GetConfig {
	config {
		yaml
		signature
	}
}
`

func (c *Client) GetConfig(ctx context.Context, interceptors ...clientv2.RequestInterceptor) (*GetConfig, error) {
	vars := map[string]interface{}{}

	var res GetConfig
	if err := c.Client.Post(ctx, "GetConfig", GetConfigDocument, &res, vars, interceptors...); err != nil {
		return &res, err
	}

	return &res, nil
}

const SetConfigDocument = `mutation SetConfig ($input: ConfigUpdateInput!) {
	updateConfig(config: $input) {
		mutationId
	}
}
`

func (c *Client) SetConfig(ctx context.Context, input ConfigUpdateInput, interceptors ...clientv2.RequestInterceptor) (*SetConfig, error) {
	vars := map[string]interface{}{
		"input": input,
	}

	var res SetConfig
	if err := c.Client.Post(ctx, "SetConfig", SetConfigDocument, &res, vars, interceptors...); err != nil {
		return &res, err
	}

	return &res, nil
}

const GetNetnsDocument = `query GetNetns ($name: String) {
	netns(filter: {name:$name}) {
		name
		pid
	}
}
`

func (c *Client) GetNetns(ctx context.Context, name *string, interceptors ...clientv2.RequestInterceptor) (*GetNetns, error) {
	vars := map[string]interface{}{
		"name": name,
	}

	var res GetNetns
	if err := c.Client.Post(ctx, "GetNetns", GetNetnsDocument, &res, vars, interceptors...); err != nil {
		return &res, err
	}

	return &res, nil
}

const GetStatusDocument = `query GetStatus {
	status {
		name
		addr
		global {
			domain
			subnet
			authorized_keys
			config_last_updated
		}
		nodes {
			name
			addr
			conns {
				subnet
				cost
			}
		}
		sessions {
			addr
			connected
			conn_start
		}
	}
}
`

func (c *Client) GetStatus(ctx context.Context, interceptors ...clientv2.RequestInterceptor) (*GetStatus, error) {
	vars := map[string]interface{}{}

	var res GetStatus
	if err := c.Client.Post(ctx, "GetStatus", GetStatusDocument, &res, vars, interceptors...); err != nil {
		return &res, err
	}

	return &res, nil
}
