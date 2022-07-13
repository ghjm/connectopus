package ssh_jwt

import (
	"encoding/base64"
	"fmt"
	"github.com/42wim/sshsig"
	"golang.org/x/crypto/ssh/agent"
	"strings"
)

func SignSSH(signingString, namespace, authorizedKey string, a agent.Agent) (string, error) {
	if a == nil {
		return "", fmt.Errorf("no SSH agent")
	}
	sig, err := sshsig.SignWithAgent([]byte(authorizedKey), a, strings.NewReader(signingString), namespace)
	if err != nil {
		return "", err
	}
	sigStr := strings.ReplaceAll(string(sig), "\n", "")
	sigStr = strings.TrimPrefix(sigStr, "-----BEGIN SSH SIGNATURE-----")
	sigStr = strings.TrimSuffix(sigStr, "-----END SSH SIGNATURE-----")
	var sigDec []byte
	sigDec, err = base64.StdEncoding.DecodeString(sigStr)
	if err != nil {
		return "", err
	}
	sigEnc := base64.RawURLEncoding.EncodeToString(sigDec)
	return sigEnc, nil
}

func VerifySSHSignature(signingString, signature, namespace, authorizedKey string) error {
	sigBytes, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return err
	}
	sigStr := "-----BEGIN SSH SIGNATURE-----\n" + base64.StdEncoding.EncodeToString(sigBytes) + "\n-----END SSH SIGNATURE-----\n"
	return sshsig.Verify(strings.NewReader(signingString), []byte(sigStr), []byte(authorizedKey), namespace)
}
