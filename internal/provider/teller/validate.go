package teller

import (
	"crypto/tls"
	"fmt"
)

// ValidateCredentials checks that the certificate and private key files
// can be loaded as a valid X.509 key pair.
func ValidateCredentials(certPath, keyPath string) error {
	_, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return fmt.Errorf("invalid teller certificate: %w", err)
	}
	return nil
}

// ValidateCredentialsPEM checks that PEM-encoded certificate and private key
// bytes form a valid X.509 key pair.
func ValidateCredentialsPEM(certPEM, keyPEM []byte) error {
	_, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("invalid teller certificate PEM: %w", err)
	}
	return nil
}
