// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command ceritifier is a small HTTP server that generates SSL certificates
// for clients. The generated certificates are signed by a certificate
// authority in PEM format whose path is specified by the --ca-cert-path and
// --ca-cert-key flags.
//
// The client may then present the certificate to v.io on demand. For example,
// https://maven.v.io/ is our repository for Java binaries. In order to fetch
// the binaries, the client must present a certificate created by Certifier.
//
// This binary should be placed behind an authenticating reverse proxy (e.g. an
// OAuth proxy) so that requests are received only from users authorized to
// make them. Also, this server will deliver the client private key via HTTP,
// so secure connections should be used between the client and the reverse
// proxy and between the reverse proxy and this server.
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net/http"
	"time"
)

var (
	caCertPath      = flag.String("ca-cert-path", "", "(required) Path to the PEM encoded CA public key to include with the signed certificates")
	caKeyPath       = flag.String("ca-key-path", "", "(required) Path to the PEM encoded CA private key to use for signing the client certificates")
	rsaBits         = flag.Int("rsa-bits", 2048, "Size of RSA key to generate.")
	address         = flag.String("address", ":8080", "The address on which to listen")
	emailHeaderName = flag.String("email-header-name", "X-Forwarded-Email", "The name of the HTTP header to use to determine the user's email address.")
)

// clientCertificateSigner represents a signer of client certificates. In fact,
// the signer is also responsible for generating the certificate.
type clientCertificateSigner struct {
	// signingKey is the private key used for signing.
	signingKey *rsa.PrivateKey

	// signingCertificate is the signing certificate to attach to the
	// signed client certificate.
	signingCertificate *x509.Certificate

	// keyLength is the RSA key length to use for generated client
	// certificates (in bits, e.g. 2048).
	keyLength int

	// emailHeaderName is the header containing the user's email address.
	// It is expected that the reverse proxy sitting in front of the
	// certifier will provide this header.
	emailHeaderName string
}

func newSigner(certificatePemBytes []byte, keyPemBytes []byte, keyLength int, emailHeaderName string) (*clientCertificateSigner, error) {
	cert, err := tls.X509KeyPair(certificatePemBytes, keyPemBytes)
	if err != nil {
		return nil, err
	}
	privKey, ok := (cert.PrivateKey).(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("Only RSA private keys are supported, got %T", privKey)
	}
	if len(cert.Certificate) != 1 {
		return nil, fmt.Errorf("Expected exactly one signing certificate, got %d", len(cert.Certificate))
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, err
	}
	signer := &clientCertificateSigner{
		keyLength:          keyLength,
		signingKey:         privKey,
		signingCertificate: leaf,
		emailHeaderName:    emailHeaderName,
	}
	return signer, nil
}

func (c clientCertificateSigner) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	emailAddresses, ok := req.Header[c.emailHeaderName]
	if !ok || emailAddresses == nil || emailAddresses[0] == "" {
		http.Error(w, fmt.Sprintf("Header %s was not specified or was empty, could not determine user's email address.", c.emailHeaderName), http.StatusBadRequest)
		return
	}
	emailAddress := emailAddresses[0]
	// Generate a new RSA private key with the specified length.
	priv, err := rsa.GenerateKey(rand.Reader, c.keyLength)
	if err != nil {
		http.Error(w, fmt.Sprintf("%v", err), http.StatusInternalServerError)
		return
	}
	// Create a new client certificate with the signing certificate as its
	// parent.
	clientCertTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: emailAddress,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * 365 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, clientCertTemplate, c.signingCertificate, &priv.PublicKey, c.signingKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("%v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-type", "application/x-pem-file")
	w.Header().Add("Content-Disposition", `attachment; filename="client-certificate.pem"`)
	var clientCertPem bytes.Buffer
	if err := pem.Encode(&clientCertPem, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		http.Error(w, fmt.Sprintf("%v", err), http.StatusInternalServerError)
		return
	}
	// Emit the email address and public part of the certificate. In the
	// event of a misbehaving client, we can use this information to revoke
	// the user's certificate and prevent them from accessing the protected
	// resource.
	log.Printf("Issuing cert to %s\n%s", emailAddress, clientCertPem.String())
	if _, err := w.Write(clientCertPem.Bytes()); err != nil {
		http.Error(w, fmt.Sprintf("%v", err), http.StatusInternalServerError)
		return
	}
	if err := pem.Encode(w, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		http.Error(w, fmt.Sprintf("%v", err), http.StatusInternalServerError)
		return
	}
}

func main() {
	flag.Parse()
	if *caCertPath == "" {
		log.Fatalf("--caCertPath is required")
	}
	if *caKeyPath == "" {
		log.Fatalf("--caKeyPath is required")
	}
	certificateBytes, err := ioutil.ReadFile(*caCertPath)
	if err != nil {
		log.Fatalf("ReadFile(%q) failed: %v", *caCertPath, err)
	}
	keyBytes, err := ioutil.ReadFile(*caKeyPath)
	if err != nil {
		log.Fatalf("ReadFile(%q) failed: %v", *caKeyPath, err)
	}
	signer, err := newSigner(certificateBytes, keyBytes, *rsaBits, *emailHeaderName)
	if err != nil {
		log.Fatalf("Could not create signer: %v", err)
	}
	http.Handle("/", signer)
	log.Printf("listening on %s", *address)
	if err := http.ListenAndServe(*address, nil); err != nil {
		log.Fatalf("ListenAndServe(%q) failed: %v", *address, err)
	}
}
