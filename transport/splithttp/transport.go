package splithttp

import (
	"context"
	"io"
	"net"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metacubex/http"
	"github.com/metacubex/mihomo/common/buf"
	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/common/utils"
	"github.com/metacubex/mihomo/component/ech"
	tlsC "github.com/metacubex/mihomo/component/tls"
	shareTLS "github.com/metacubex/mihomo/component/transport/tls"
	"github.com/metacubex/quic-go"
	"github.com/metacubex/quic-go/http3"
	"github.com/metacubex/sing/common/bufio"
	"github.com/metacubex/tls"
)

type pooledBodyReader struct {
	mu  sync.Mutex
	buf *buf.Buffer
}

func (r *pooledBodyReader) Read(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.buf == nil || r.buf.IsEmpty() {
		return 0, io.EOF
	}
	n, err = r.buf.Read(p)
	if r.buf.IsEmpty() {
		err = io.EOF
	}
	return
}

func (r *pooledBodyReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.buf != nil {
		r.buf.Release()
		r.buf = nil
	}
	return nil
}

type TransportWrap struct {
	ctx            context.Context
	cancel         context.CancelFunc
	config         *Config
	tlsConfig      *tls.Config
	realityConfig  *tlsC.RealityConfig
	dialFn         func(context.Context, string, string) (net.Conn, error)
	listenPacketFn func(context.Context) (net.PacketConn, net.Addr, error)
	xmuxManager    *XmuxManager
	httpVersion    string
}

func NewTransport(dialFn func(context.Context, string, string) (net.Conn, error), lpFn func(context.Context) (net.PacketConn, net.Addr, error), tlsCfg *tls.Config, cfg *Config, fp, authCert, authKey string, echCfg *ech.Config, realityCfg *tlsC.RealityConfig) *TransportWrap {
	ctx, cancel := context.WithCancel(context.Background())
	httpVersion := "2"
	if tlsCfg != nil && len(tlsCfg.NextProtos) > 0 && tlsCfg.NextProtos[0] == "h3" {
		httpVersion = "3"
	}
	tw := &TransportWrap{ctx: ctx, cancel: cancel, config: cfg, tlsConfig: tlsCfg, realityConfig: realityCfg, listenPacketFn: lpFn, httpVersion: httpVersion}
	tw.dialFn = func(ctxI context.Context, network, addr string) (net.Conn, error) {
		pconn, err := dialFn(ctxI, network, addr)
		if err != nil {
			return nil, err
		}
		if tlsCfg == nil {
			return pconn, nil
		}
		return shareTLS.StreamTLSConn(ctxI, pconn, &shareTLS.Config{
			Host: tlsCfg.ServerName, SkipCertVerify: tlsCfg.InsecureSkipVerify, NextProtos: tlsCfg.NextProtos,
			ClientFingerprint: fp, Certificate: authCert, PrivateKey: authKey, ECH: echCfg, Reality: realityCfg,
		})
	}
	var xConf XmuxConfig
	if cfg.Xmux != nil {
		xConf = *cfg.Xmux
	}
	tw.xmuxManager = NewXmuxManager(xConf, func() XmuxConn { return tw.createHTTPClient() })
	return tw
}

func (tw *TransportWrap) HTTPVersion() string { return tw.httpVersion }
func (tw *TransportWrap) Close() error        { tw.cancel(); return nil }

func (tw *TransportWrap) createHTTPClient() DialerClient {
	var transport http.RoundTripper
	if tw.httpVersion == "3" {
		transport = &http3.Transport{QUICConfig: &quic.Config{MaxIdleTimeout: 90 * time.Second, KeepAlivePeriod: 15 * time.Second}, TLSClientConfig: tw.tlsConfig, Dial: func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
			pconn, rAddr, err := tw.listenPacketFn(ctx)
			if err != nil {
				return nil, err
			}
			return quic.DialEarly(ctx, pconn, rAddr, tlsCfg, cfg)
		}}
	} else {
		transport = &http.Http2Transport{DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return tw.dialFn(ctx, network, addr)
		}, AllowHTTP: true, ReadIdleTimeout: 30 * time.Minute}
	}
	return &DefaultDialerClient{transportConfig: tw.config, client: &http.Client{Transport: transport}}
}

type readOnly struct{ io.Reader }

func (readOnly) Close() error { return nil }

func (tw *TransportWrap) DialContext(ctx context.Context) (net.Conn, error) {
	requestURL := url.URL{Scheme: "http", Path: tw.config.GetNormalizedPath(), RawQuery: tw.config.GetNormalizedQuery()}
	if tw.tlsConfig != nil {
		requestURL.Scheme = "https"
	}
	requestURL.Host = tw.config.Host
	if requestURL.Host == "" && tw.tlsConfig != nil {
		requestURL.Host = tw.tlsConfig.ServerName
	}

	xmuxClient := tw.xmuxManager.GetXmuxClient(ctx)
	httpClient := xmuxClient.XmuxConn.(DialerClient)
	mode := tw.config.Mode
	if mode == "" || mode == "auto" {
		mode = "packet-up"
		if tw.realityConfig != nil {
			mode = "stream-one"
		}
	}
	sessionId := ""
	if mode != "stream-one" {
		sessionId = utils.NewUUIDV4().String()
	}

	p1, p2 := N.Pipe()
	bgCtx, bgCancel := context.WithCancel(context.Background())
	rAddr, lAddr := &LazyAddr{}, &LazyAddr{}
	xmuxClient.OpenUsage.Add(1)

	var closed atomic.Bool
	var wg sync.WaitGroup
	onCloseFunc := func() {
		if closed.CompareAndSwap(false, true) {
			bgCancel()
			xmuxClient.OpenUsage.Add(-1)
			_ = p1.Close()
			_ = p2.Close()
		}
	}

	// 🚦 此时不再需要 hsErrCh 阻塞 Read
	go func() {
		xmuxClient.LeftRequests.Add(-1)
		var respBody io.ReadCloser
		var err error

		if mode == "stream-one" || mode == "stream-up" {
			respBody, err = httpClient.OpenStream(bgCtx, requestURL.String(), sessionId, readOnly{p1}, mode == "stream-up", rAddr, lAddr)
		} else {
			respBody, err = httpClient.OpenStream(bgCtx, requestURL.String(), sessionId, nil, false, rAddr, lAddr)
		}

		if err != nil {
			onCloseFunc()
			return
		}

		if mode == "stream-one" || mode == "stream-up" {
			if respBody != nil {
				defer respBody.Close()
				_, _ = bufio.Copy(p1, respBody)
			}
			onCloseFunc()
			return
		}

		scMaxEach := tw.config.GetNormalizedScMaxEachPostBytes().rand()

		go func() {
			if respBody != nil {
				defer respBody.Close()
				_, _ = bufio.Copy(p1, respBody)
			}
			onCloseFunc()
		}()

		var seq int64
		const maxConcurrency = 16
		semaphore := make(chan struct{}, maxConcurrency)
		for i := 0; i < maxConcurrency; i++ {
			semaphore <- struct{}{}
		}

		defer func() { wg.Wait(); onCloseFunc() }()

		for {
			select {
			case <-semaphore:
			case <-bgCtx.Done():
				return
			}

			payload := buf.NewSize(int(scMaxEach))
			_, err := payload.ReadOnceFrom(p1)

			if payload.IsEmpty() {
				payload.Release()
				semaphore <- struct{}{}
				if err != nil {
					break
				}
				continue
			}

			seqStr := strconv.FormatInt(seq, 10)
			seq++
			curClient := tw.xmuxManager.GetXmuxClient(bgCtx)
			curHTTP := curClient.XmuxConn.(DialerClient)
			curClient.LeftRequests.Add(-1)

			wg.Add(1)
			go func(s string, p *buf.Buffer, cl DialerClient) {
				defer wg.Done()
				defer func() { semaphore <- struct{}{} }()
				bodyReader := &pooledBodyReader{buf: p}
				defer bodyReader.Close()
				if errPost := cl.PostPacket(bgCtx, requestURL.String(), sessionId, s, bodyReader); errPost != nil {
					onCloseFunc()
				}
			}(seqStr, payload, curHTTP)

			if err != nil {
				break
			}
		}
	}()

	wrappedConn := &asyncStreamConn{
		Conn:        N.NewDeadlineConn(p2),
		rAddr:       rAddr,
		lAddr:       lAddr,
		defaultHost: requestURL.Host,
		onClose:     onCloseFunc,
	}

	// 🚀 终极优化：直接返回连接，不再使用 EarlyConn 阻塞 Read 响应头。
	// 这就是全链路 0-RTT 的精髓：相信请求会成功，让首包飞出去。
	return N.NewRefConn(wrappedConn, xmuxClient), nil
}
