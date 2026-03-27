package v2rayxhttp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	boxTLS "github.com/sagernet/sing-box/common/tls"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/rw"

	"github.com/stretchr/testify/require"
)

type echoHandler struct{}

func (echoHandler) NewConnectionEx(ctx context.Context, conn net.Conn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	go func() {
		defer func() {
			_ = conn.Close()
			if onClose != nil {
				onClose(nil)
			}
		}()
		_, _ = io.Copy(conn, conn)
	}()
}

func TestV2RayXHTTPHTTP3StreamOne(t *testing.T) {
	t.Parallel()

	caPem, certPem, keyPem := createSelfSignedCertificate(t, "example.org")
	logger := log.NewNOPFactory().Logger()

	serverTLS, err := boxTLS.NewServer(context.Background(), logger, option.InboundTLSOptions{
		Enabled:         true,
		ServerName:      "example.org",
		ALPN:            []string{"h3"},
		CertificatePath: certPem,
		KeyPath:         keyPem,
	})
	require.NoError(t, err)
	require.NoError(t, serverTLS.Start())
	defer serverTLS.Close()

	server, err := NewServer(context.Background(), logger, option.V2RayXHTTPOptions{
		Mode: "stream-one",
		Path: "/xhttp-h3",
	}, serverTLS, echoHandler{})
	require.NoError(t, err)
	defer server.Close()

	packetListener, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer packetListener.Close()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ServePacket(packetListener)
	}()

	clientTLS, err := boxTLS.NewClient(context.Background(), logger, "example.org", option.OutboundTLSOptions{
		Enabled:         true,
		ServerName:      "example.org",
		ALPN:            []string{"h3"},
		CertificatePath: caPem,
	})
	require.NoError(t, err)

	client, err := NewClient(context.Background(), N.SystemDialer, M.ParseSocksaddr(packetListener.LocalAddr().String()), option.V2RayXHTTPOptions{
		Mode: "stream-one",
		Path: "/xhttp-h3",
	}, clientTLS)
	require.NoError(t, err)
	defer client.Close()

	conn, err := client.DialContext(context.Background())
	require.NoError(t, err)
	defer conn.Close()

	payload := []byte("hello-http3-xhttp")
	_, err = conn.Write(payload)
	require.NoError(t, err)

	result := make(chan error, 1)
	go func() {
		received := make([]byte, len(payload))
		_, readErr := io.ReadFull(conn, received)
		if readErr != nil {
			result <- readErr
			return
		}
		if string(received) != string(payload) {
			result <- io.ErrUnexpectedEOF
			return
		}
		result <- nil
	}()

	select {
	case err = <-result:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for h3 xhttp echo")
	}
}

func createSelfSignedCertificate(t *testing.T, domain string) (caPem, certPem, keyPem string) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "sing-box-xhttp-h3")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	spkiASN1, err := x509.MarshalPKIXPublicKey(caKey.Public())
	require.NoError(t, err)

	var spki struct {
		Algorithm        pkix.AlgorithmIdentifier
		SubjectPublicKey asn1.BitString
	}
	_, err = asn1.Unmarshal(spkiASN1, &spki)
	require.NoError(t, err)

	skid := sha1.Sum(spki.SubjectPublicKey.Bytes)
	caTpl := &x509.Certificate{
		SerialNumber: randomSerialNumber(t),
		Subject: pkix.Name{
			CommonName: "sing-box xhttp h3 test ca",
		},
		SubjectKeyId:          skid[:],
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	caCert, err := x509.CreateCertificate(rand.Reader, caTpl, caTpl, caKey.Public(), caKey)
	require.NoError(t, err)

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	serverTpl := &x509.Certificate{
		SerialNumber: randomSerialNumber(t),
		Subject: pkix.Name{
			CommonName: domain,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(0, 1, 0),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{domain},
	}
	serverCert, err := x509.CreateCertificate(rand.Reader, serverTpl, caTpl, serverKey.Public(), caKey)
	require.NoError(t, err)

	caPath := filepath.Join(tempDir, "ca.pem")
	certPath := filepath.Join(tempDir, "cert.pem")
	keyPath := filepath.Join(tempDir, "key.pem")

	require.NoError(t, rw.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert})))
	require.NoError(t, rw.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCert})))

	serverKeyDER, err := x509.MarshalPKCS8PrivateKey(serverKey)
	require.NoError(t, err)
	require.NoError(t, rw.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: serverKeyDER})))

	return caPath, certPath, keyPath
}

func randomSerialNumber(t *testing.T) *big.Int {
	t.Helper()

	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, limit)
	require.NoError(t, err)
	return serialNumber
}
