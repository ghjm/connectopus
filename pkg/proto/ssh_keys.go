package proto

import (
	"encoding/json"
	"golang.org/x/crypto/ssh"
	"strings"
)

type MarshalablePublicKey struct {
	ssh.PublicKey
	Comment string
}

// String returns the SSH authorized_keys formatted string of the public key.
//goland:noinspection GoMixedReceiverTypes
func (mpk *MarshalablePublicKey) String() string {
	str := string(ssh.MarshalAuthorizedKey(mpk))
	str = strings.Trim(str, "\n")
	if mpk.Comment != "" {
		str += " " + mpk.Comment
	}
	return str
}

func ParseAuthorizedKey(key string) (*MarshalablePublicKey, error) {
	mpk := MarshalablePublicKey{}
	err := parseAuthorizedKey(key, &mpk)
	if err != nil {
		return nil, err
	}
	return &mpk, nil
}

// parseAuthorizedKey parses an SSH public key in authorized_keys format to an already-allocated MarshalablePublicKey
func parseAuthorizedKey(data string, mpk *MarshalablePublicKey) error {
	key, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(data))
	if err != nil {
		return err
	}
	*mpk = MarshalablePublicKey{
		PublicKey: key,
		Comment:   comment,
	}
	return nil
}

// MarshalJSON marshals an SSH public key to JSON.
//goland:noinspection GoMixedReceiverTypes
func (mpk MarshalablePublicKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(mpk.String())
}

// UnmarshalJSON unmarshals an SSH public key from JSON.
//goland:noinspection GoMixedReceiverTypes
func (mpk *MarshalablePublicKey) UnmarshalJSON(data []byte) error {
	var str string
	err := json.Unmarshal(data, &str)
	if err != nil {
		return err
	}
	return parseAuthorizedKey(str, mpk)
}

// MarshalYAML marshals an SSH public key to YAML.
//goland:noinspection GoMixedReceiverTypes
func (mpk MarshalablePublicKey) MarshalYAML() (interface{}, error) {
	return mpk.String(), nil
}

// UnmarshalYAML unmarshals an SSH public key from YAML.
//goland:noinspection GoMixedReceiverTypes
func (mpk *MarshalablePublicKey) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var str string
	err := unmarshal(&str)
	if err != nil {
		return err
	}
	return parseAuthorizedKey(str, mpk)
}
