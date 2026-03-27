package v2rayxhttp

import (
	"context"
	"io"
	"net"
	"net/http"

	"github.com/sagernet/quic-go"
	"github.com/sagernet/quic-go/http3"
	"github.com/sagernet/sing-box/common/tls"
	C "github.com/sagernet/sing-box/constant"
	qtls "github.com/sagernet/sing-quic"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

type managedRoundTripper struct {
	http.RoundTripper
	packetConn net.PacketConn
}

func (m *managedRoundTripper) CloseIdleConnections() {
	if pool, ok := m.RoundTripper.(interface{ CloseIdleConnections() }); ok {
		pool.CloseIdleConnections()
	}
}

func (m *managedRoundTripper) Close() error {
	var err error
	if closer, ok := m.RoundTripper.(io.Closer); ok {
		err = closer.Close()
	}
	if m.packetConn != nil {
		if closeErr := m.packetConn.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

func wantsHTTP3(config interface{ NextProtos() []string }) bool {
	if config == nil {
		return false
	}
	nextProtos := config.NextProtos()
	return len(nextProtos) > 0 && nextProtos[0] == http3.NextProtoH3
}

func newHTTP3Transport(dialer N.Dialer, serverAddr M.Socksaddr, tlsConfig tls.Config) (http.RoundTripper, string, error) {
	udpConn, err := dialer.DialContext(context.Background(), N.NetworkUDP, serverAddr)
	if err != nil {
		return nil, "", err
	}
	packetConn := bufio.NewUnbindPacketConn(udpConn)
	quicConfig := &quic.Config{
		DisablePathMTUDiscovery: !C.IsLinux && !C.IsWindows,
	}
	var quicConn *quic.Conn
	transport, err := qtls.CreateTransport(packetConn, &quicConn, serverAddr, tlsConfig, quicConfig)
	if err != nil {
		_ = packetConn.Close()
		return nil, "", err
	}
	return &managedRoundTripper{RoundTripper: transport, packetConn: packetConn}, "https", nil
}
