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

func NewClient(cli *http.Client, baseURL string, interceptors ...clientv2.RequestInterceptor) *Client {
	return &Client{Client: clientv2.NewClient(cli, baseURL, interceptors...)}
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
type SetConfig_UpdateConfig struct {
	MutationID string "json:\"mutationId\" graphql:\"mutationId\""
}
type GetNetns_Netns struct {
	Name string "json:\"name\" graphql:\"name\""
	Pid  int    "json:\"pid\" graphql:\"pid\""
}
type GetStatus_Status_Global struct {
	Domain            string    "json:\"domain\" graphql:\"domain\""
	Subnet            string    "json:\"subnet\" graphql:\"subnet\""
	AuthorizedKeys    []string  "json:\"authorized_keys\" graphql:\"authorized_keys\""
	ConfigLastUpdated time.Time "json:\"config_last_updated\" graphql:\"config_last_updated\""
}
type GetStatus_Status_Nodes_Conns struct {
	Subnet string  "json:\"subnet\" graphql:\"subnet\""
	Cost   float64 "json:\"cost\" graphql:\"cost\""
}
type GetStatus_Status_Nodes struct {
	Name  string                          "json:\"name\" graphql:\"name\""
	Addr  string                          "json:\"addr\" graphql:\"addr\""
	Conns []*GetStatus_Status_Nodes_Conns "json:\"conns\" graphql:\"conns\""
}
type GetStatus_Status_Sessions struct {
	Addr      string "json:\"addr\" graphql:\"addr\""
	Connected bool   "json:\"connected\" graphql:\"connected\""
	ConnStart string "json:\"conn_start\" graphql:\"conn_start\""
}
type GetStatus_Status struct {
	Name     string                       "json:\"name\" graphql:\"name\""
	Addr     string                       "json:\"addr\" graphql:\"addr\""
	Global   GetStatus_Status_Global      "json:\"global\" graphql:\"global\""
	Nodes    []*GetStatus_Status_Nodes    "json:\"nodes\" graphql:\"nodes\""
	Sessions []*GetStatus_Status_Sessions "json:\"sessions\" graphql:\"sessions\""
}
type GetConfig struct {
	Config GetConfig_Config "json:\"config\" graphql:\"config\""
}
type SetConfig struct {
	UpdateConfig SetConfig_UpdateConfig "json:\"updateConfig\" graphql:\"updateConfig\""
}
type GetNetns struct {
	Netns []*GetNetns_Netns "json:\"netns\" graphql:\"netns\""
}
type GetStatus struct {
	Status GetStatus_Status "json:\"status\" graphql:\"status\""
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
		return nil, err
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
		return nil, err
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
		return nil, err
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
		return nil, err
	}

	return &res, nil
}
