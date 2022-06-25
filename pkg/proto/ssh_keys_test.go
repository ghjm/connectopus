package proto

import (
	"encoding/json"
	"gopkg.in/yaml.v3"
	"testing"
)

var testKeys = []string{
	"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAAAgQCoaE5MgvCMJJPuu1pVokckW41DN/OOqvS5MPHAO7c2TRXTKJ0GkyZjosXSCNiqBHRYoQzZkmndJ+Wek22gjELdcs3iu/Qn+LM3aVYpe2EGauVGQ80G1qNb1JUWYEbadJbWP4FV3MF2b3IL4ZzWy0oPoMqaxGRrWz8TaT1uBxBBww== test key RSA",
	"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAID1yAQanMcWEnwBnw2K8xlTlN+3NtMWkT125vLYyquLo test key ed25519",
}

func TestSSHKeyMarshaling(t *testing.T) {
	for _, keyStr := range testKeys {
		// roundtrip through regular parse
		pk, err := ParseAuthorizedKey(keyStr)
		if err != nil {
			t.Fatal(err)
		}
		if pk.String() != keyStr {
			t.Fatal("keys did not match")
		}

		// roundtrip through JSON
		keyJSON, err := json.Marshal(pk)
		if err != nil {
			t.Fatal(err)
		}
		jk := &MarshalablePublicKey{}
		err = json.Unmarshal(keyJSON, jk)
		if err != nil {
			t.Fatal(err)
		}
		if jk.String() != keyStr {
			t.Fatal("keys did not match")
		}

		// roundtrip through YAML
		keyYAML, err := yaml.Marshal(pk)
		if err != nil {
			t.Fatal(err)
		}
		yk := MarshalablePublicKey{}
		err = yaml.Unmarshal(keyYAML, &yk)
		if err != nil {
			t.Fatal(err)
		}
		if yk.String() != keyStr {
			t.Fatal("keys did not match")
		}

	}
}
