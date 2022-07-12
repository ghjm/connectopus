//go:build !windows

package cpctl

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"github.com/ghjm/connectopus/pkg/config"
	"github.com/ghjm/connectopus/pkg/links/netns"
	"github.com/ghjm/connectopus/pkg/proto"
	"github.com/ghjm/connectopus/pkg/x/ssh_jwt"
	"github.com/golang-jwt/jwt/v4"
	"go.uber.org/goleak"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestCpctl(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	f, err := ioutil.TempFile("", "temp_cpctl_*.sock")
	if err != nil {
		t.Fatal(err)
	}
	fn := f.Name()
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		time.Sleep(50 * time.Millisecond)
		_ = os.Remove(fn)
	}()
	var privateKey *rsa.PrivateKey
	privateKey, err = rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	var publicKey ssh.PublicKey
	publicKey, err = ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	a := agent.NewKeyring()
	err = a.Add(agent.AddedKey{PrivateKey: privateKey})
	if err != nil {
		t.Fatal(err)
	}
	mpk := proto.MarshalablePublicKey{
		PublicKey: publicKey,
		Comment:   "",
	}
	var sm jwt.SigningMethod
	sm, err = ssh_jwt.SetupSigningMethod("connectopus", nil)
	if err != nil {
		t.Fatal("error initializing JWT signing method: %w", err)
	}
	r := Server{
		Resolver: Resolver{
			GetConfig: func() *config.Config {
				return &config.Config{
					Global: config.Global{
						AuthorizedKeys: []proto.MarshalablePublicKey{mpk},
					},
				}
			},
			GetNetopus: func() proto.Netopus { return nil },
			GetNsReg:   func() *netns.Registry { return nil },
		},
		SigningMethod: sm,
	}
	err = os.Remove(fn)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.ServeUnix(ctx, fn)
	if err != nil {
		t.Fatal(err)
	}
	var tok string
	tok, err = GenTokenFromAgent(a, mpk.String(), "")
	if err != nil {
		t.Fatal(err)
	}
	var c *Client
	c, err = NewSocketClient(fn, "", tok, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.GetNetns(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
}
