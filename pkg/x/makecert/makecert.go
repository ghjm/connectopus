package makecert

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"
)

// This package is used to create ad-hoc test CAs and certificates.  It is intended to be used in tests and is
// not a general purpose certificate management library.

type CA struct {
	Certificate *x509.Certificate
	CertPEM     []byte
	Key         *rsa.PrivateKey
	KeyPEM      []byte
	Pool        *x509.CertPool
}

func MakeCA(orgName string, bits int, expireDays int) (*CA, error) {
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{orgName},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, expireDays),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	ca := &CA{}
	var err error
	ca.Key, err = rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, err
	}
	var caBytes []byte
	caBytes, err = x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &ca.Key.PublicKey, ca.Key)
	if err != nil {
		return nil, err
	}
	ca.Certificate, err = x509.ParseCertificate(caBytes)
	if err != nil {
		return nil, err
	}
	caPEM := new(bytes.Buffer)
	err = pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: ca.Certificate.Raw,
	})
	if err != nil {
		return nil, err
	}
	ca.CertPEM = caPEM.Bytes()
	caKeyPEM := new(bytes.Buffer)
	err = pem.Encode(caKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(ca.Key),
	})
	if err != nil {
		return nil, err
	}
	ca.KeyPEM = caKeyPEM.Bytes()
	ca.Pool = x509.NewCertPool()
	ca.Pool.AppendCertsFromPEM(ca.CertPEM)
	return ca, nil
}

type Cert struct {
	Certificate *x509.Certificate
	CertPEM     []byte
	Key         *rsa.PrivateKey
	KeyPEM      []byte
	TLSCert     tls.Certificate
}

func (ca *CA) MakeCert(orgName string, bits int, expireDays int, ipAddresses []net.IP, dnsNames []string) (*Cert, error) {
	certTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{orgName},
		},
		DNSNames:     dnsNames,
		IPAddresses:  ipAddresses,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(0, 0, expireDays),
		SubjectKeyId: []byte{1},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	cert := &Cert{}
	var err error
	cert.Key, err = rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, err
	}
	var certBytes []byte
	certBytes, err = x509.CreateCertificate(rand.Reader, certTemplate, ca.Certificate, &cert.Key.PublicKey, ca.Key)
	if err != nil {
		return nil, err
	}
	cert.Certificate, err = x509.ParseCertificate(certBytes)
	if err != nil {
		return nil, err
	}
	caPEM := new(bytes.Buffer)
	err = pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Certificate.Raw,
	})
	if err != nil {
		return nil, err
	}
	cert.CertPEM = caPEM.Bytes()
	caKeyPEM := new(bytes.Buffer)
	err = pem.Encode(caKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(cert.Key),
	})
	if err != nil {
		return nil, err
	}
	cert.KeyPEM = caKeyPEM.Bytes()
	cert.TLSCert, err = tls.X509KeyPair(cert.CertPEM, cert.KeyPEM)
	if err != nil {
		return nil, err
	}
	return cert, nil
}
