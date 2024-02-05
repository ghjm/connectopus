// Code generated by github.com/Yamashou/gqlgenc, DO NOT EDIT.

package localui

import (
	"net/http"

	"github.com/Yamashou/gqlgenc/clientv2"
)

type Client struct {
	Client *clientv2.Client
}

func NewClient(cli *http.Client, baseURL string, options *clientv2.Options, interceptors ...clientv2.RequestInterceptor) *Client {
	return &Client{Client: clientv2.NewClient(cli, baseURL, options, interceptors...)}
}

type Query struct {
	AvailableNodes AvailableNodesResult "json:\"availableNodes\" graphql:\"availableNodes\""
}

type Mutation struct {
	SelectNode SelectNodeResult "json:\"selectNode\" graphql:\"selectNode\""
}

var DocumentOperationNames = map[string]string{}
