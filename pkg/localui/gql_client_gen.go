// Code generated by github.com/Yamashou/gqlgenc, DO NOT EDIT.

package localui

import (
	"context"
	"net/http"

	"github.com/Yamashou/gqlgenc/clientv2"
)

type Client struct {
	Client *clientv2.Client
}

func NewClient(cli *http.Client, baseURL string, interceptors ...clientv2.RequestInterceptor) *Client {
	return &Client{Client: clientv2.NewClient(cli, baseURL, interceptors...)}
}

type Query struct {
	SSHKeys []*SSHKeyResult "json:\"sshKeys\" graphql:\"sshKeys\""
}
type Mutation struct {
	Authenticate AuthenticateResult "json:\"authenticate\" graphql:\"authenticate\""
}
type GetSSHKeys_SSHKeys struct {
	AuthorizedKeys []string "json:\"authorizedKeys\" graphql:\"authorizedKeys\""
}
type GetSSHKeys struct {
	SSHKeys []*GetSSHKeys_SSHKeys "json:\"sshKeys\" graphql:\"sshKeys\""
}

const GetSSHKeysDocument = `query GetSSHKeys {
	sshKeys {
		authorizedKeys
	}
}
`

func (c *Client) GetSSHKeys(ctx context.Context, interceptors ...clientv2.RequestInterceptor) (*GetSSHKeys, error) {
	vars := map[string]interface{}{}

	var res GetSSHKeys
	if err := c.Client.Post(ctx, "GetSSHKeys", GetSSHKeysDocument, &res, vars, interceptors...); err != nil {
		return nil, err
	}

	return &res, nil
}