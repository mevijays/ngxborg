package webui

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// certDir holds the web UI's own TLS certificate. A PAM password is going
// over this connection, so it is served over TLS even with nothing else
// configured — self-signed rather than not encrypted at all, the same
// reasoning ngxsetup applies to a freshly created site before a real
// certificate exists.
const certDir = "/etc/ngxborg/certs"

// ensureCert returns a tls.Certificate, generating and persisting a
// self-signed one on first run so restarts do not present a different
// certificate (and a different browser warning) every time.
func ensureCert() (tls.Certificate, error) {
	certPath := filepath.Join(certDir, "web.crt")
	keyPath := filepath.Join(certDir, "web.key")

	if cert, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
		return cert, nil
	}

	cert, key, err := generateSelfSigned()
	if err != nil {
		return tls.Certificate{}, err
	}
	if err := os.MkdirAll(certDir, 0o750); err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(certPath, cert, 0o644); err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(keyPath, key, 0o600); err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(cert, key)
}

func generateSelfSigned() (certPEM, keyPEM []byte, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "ngxborg", Organization: []string{"ngxborg"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("creating certificate: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}
