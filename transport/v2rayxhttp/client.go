package v2rayxhttp

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/tls"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/transport/v2rayhttpupgrade"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"github.com/gofrs/uuid/v5"
)

var _ adapter.V2RayClientTransport = (*Client)(nil)

type Client struct {
	wrapper          adapter.V2RayClientTransport
	xmuxManager      *xmuxManager
	behavior         requestBehavior
	mode             string
	sessionPlacement string
	sessionKey       string
	seqPlacement     string
	seqKey           string
	uplinkPlacement  string
	httpClient       *http.Client
	downloadURL      string
	downloadHost     string
	downloadHeaders  http.Header
	http2            bool
	requestURL       string
	host             string
	headers          http.Header
	seq              atomic.Int64
}

func NewClient(ctx context.Context, dialer N.Dialer, serverAddr M.Socksaddr, options option.V2RayXHTTPOptions, tlsConfig tls.Config) (*Client, error) {
	_ = ctx
	mode, err := normalizeMode(options.Mode)
	if err != nil {
		return nil, err
	}
	if mode == ModeStreamOne && !wantsHTTP3(tlsConfig) {
		transport, err := v2rayhttpupgrade.NewClient(ctx, dialer, serverAddr, option.V2RayHTTPUpgradeOptions{
			Host:    options.Host,
			Path:    options.Path,
			Headers: options.Headers,
		}, tlsConfig)
		if err != nil {
			return nil, err
		}
		return &Client{wrapper: transport, mode: mode}, nil
	}
	sessionPlacement, err := normalizePlacement("session placement", options.SessionPlacement)
	if err != nil {
		return nil, err
	}
	seqPlacement, err := normalizePlacement("seq placement", options.SeqPlacement)
	if err != nil {
		return nil, err
	}
	uplinkPlacement, err := normalizeDataPlacement(options.UplinkDataPlacement)
	if err != nil {
		return nil, err
	}
	behavior, err := newRequestBehavior(mode, uplinkPlacement, options)
	if err != nil {
		return nil, err
	}
	transport, requestURL, host, err := newHTTPTransport(dialer, serverAddr, options, tlsConfig)
	if err != nil {
		return nil, err
	}
	downloadURL, downloadHost, downloadHeaders, err := newDownloadSettings(requestURL, host, options)
	if err != nil {
		return nil, err
	}
	return &Client{
		wrapper:          nil,
		xmuxManager:      newXMuxManager(options.XMux, func() xmuxConn { return &xmuxReusableConn{} }),
		behavior:         behavior,
		mode:             mode,
		sessionPlacement: sessionPlacement,
		sessionKey:       options.SessionKey,
		seqPlacement:     seqPlacement,
		seqKey:           options.SeqKey,
		uplinkPlacement:  uplinkPlacement,
		httpClient:       &http.Client{Transport: transport},
		downloadURL:      downloadURL,
		downloadHost:     downloadHost,
		downloadHeaders:  downloadHeaders,
		http2:            strings.HasPrefix(requestURL, "https://") && !containsHTTP1Only(tlsConfig),
		requestURL:       requestURL,
		host:             host,
		headers:          options.Headers.Build(),
	}, nil
}

func (c *Client) DialContext(ctx context.Context) (net.Conn, error) {
	if c.wrapper != nil {
		return c.wrapper.DialContext(ctx)
	}
	var err error
	sessionID := ""
	if c.mode != ModeStreamOne {
		var sessionUUID uuid.UUID
		sessionUUID, err = uuid.NewV4()
		if err != nil {
			return nil, err
		}
		sessionID = sessionUUID.String()
	}
	baseURL, err := applySessionToURL(c.requestURL, c.sessionPlacement, c.sessionKey, sessionID)
	if err != nil {
		return nil, err
	}
	seq := &atomic.Int64{}
	downloadBaseURL, err := applySessionToURL(c.downloadURL, c.sessionPlacement, c.sessionKey, sessionID)
	if err != nil {
		return nil, err
	}
	if c.xmuxManager != nil {
		client := c.xmuxManager.getClient(ctx)
		client.openUsage.Add(1)
		defer client.openUsage.Add(-1)
	}
	conn := newConn(ctx, c.httpClient, c.mode, c.behavior, c.sessionPlacement, c.sessionKey, c.seqPlacement, c.seqKey, c.uplinkPlacement, c.http2, sessionID, baseURL, downloadBaseURL, c.host, c.downloadHost, c.headers.Clone(), c.downloadHeaders.Clone(), seq)
	if err = conn.start(); err != nil {
		return nil, err
	}
	return conn, nil
}

func (c *Client) Close() error {
	if c.wrapper != nil {
		return c.wrapper.Close()
	}
	closeIdleConnections(c.httpClient.Transport)
	return nil
}

type conn struct {
	ctx              context.Context
	cancel           context.CancelFunc
	httpClient       *http.Client
	mode             string
	behavior         requestBehavior
	sessionPlacement string
	sessionKey       string
	seqPlacement     string
	seqKey           string
	uplinkPlacement  string
	http2            bool
	sessionID        string
	baseURL          string
	downloadURL      string
	host             string
	downloadHost     string
	headers          http.Header
	downloadHeaders  http.Header
	seq              *atomic.Int64
	reader           io.ReadCloser
	upload           io.WriteCloser
	uploadQueue      chan []byte
	uploadDone       chan struct{}
	uploadErr        atomic.Pointer[error]
	downloadErr      atomic.Pointer[error]
	remoteAddr       net.Addr
	localAddr        net.Addr
	closed           atomic.Bool
}

type xmuxReusableConn struct {
	closed atomic.Bool
}

func (c *xmuxReusableConn) IsClosed() bool {
	return c.closed.Load()
}

func newConn(ctx context.Context, httpClient *http.Client, mode string, behavior requestBehavior, sessionPlacement string, sessionKey string, seqPlacement string, seqKey string, uplinkPlacement string, http2 bool, sessionID string, baseURL string, downloadURL string, host string, downloadHost string, headers http.Header, downloadHeaders http.Header, seq *atomic.Int64) *conn {
	connCtx, cancel := context.WithCancel(ctx)
	return &conn{
		ctx:              connCtx,
		cancel:           cancel,
		httpClient:       httpClient,
		mode:             mode,
		behavior:         behavior,
		sessionPlacement: sessionPlacement,
		sessionKey:       sessionKey,
		seqPlacement:     seqPlacement,
		seqKey:           seqKey,
		uplinkPlacement:  uplinkPlacement,
		http2:            http2,
		sessionID:        sessionID,
		baseURL:          baseURL,
		downloadURL:      downloadURL,
		host:             host,
		downloadHost:     downloadHost,
		headers:          headers,
		downloadHeaders:  downloadHeaders,
		seq:              seq,
		uploadQueue:      make(chan []byte, 32),
		uploadDone:       make(chan struct{}),
		remoteAddr:       M.ParseSocksaddr(baseURL),
		localAddr:        M.Socksaddr{},
	}
}

func (c *conn) start() error {
	if c.mode == ModeStreamOne {
		return c.startSingleStream()
	}
	if err := c.startDownload(); err != nil {
		return err
	}
	if c.mode == ModeStreamUp {
		if err := c.startUploadStream(); err != nil {
			_ = c.reader.Close()
			return err
		}
	} else {
		go c.runPacketUploadLoop()
	}
	return nil
}

func (c *conn) startSingleStream() error {
	defer close(c.uploadDone)
	reader, writer := io.Pipe()
	request, err := http.NewRequestWithContext(c.ctx, c.behavior.uplinkHTTPMethod, c.baseURL, reader)
	if err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return err
	}
	request.Host = c.host
	request.Header = c.headers.Clone()
	if err = fillStreamRequestWithKeys(request, c.sessionPlacement, c.seqPlacement, c.sessionKey, c.seqKey, c.sessionID, c.behavior); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return err
	}
	request.ContentLength = -1
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			c.remoteAddr = info.Conn.RemoteAddr()
			c.localAddr = info.Conn.LocalAddr()
		},
	}))
	c.upload = writer
	wrc := &waitReadCloser{Wait: make(chan struct{})}
	c.reader = wrc
	go func() {
		response, err := c.httpClient.Do(request)
		if err != nil {
			c.storeUploadErr(err)
			c.storeDownloadErr(err)
			_ = writer.CloseWithError(err)
			wrc.Close()
			return
		}
		if response.StatusCode != http.StatusOK {
			err = newHTTPStatusError("xhttp stream-one", response.Status)
			c.storeUploadErr(err)
			c.storeDownloadErr(err)
			_ = writer.CloseWithError(err)
			response.Body.Close()
			wrc.Close()
			return
		}
		wrc.Set(response.Body)
	}()
	return nil
}

func (c *conn) startDownload() error {
	request, err := http.NewRequestWithContext(c.ctx, http.MethodGet, c.downloadURL, nil)
	if err != nil {
		return err
	}
	request.Host = c.downloadHost
	request.Header = c.downloadHeaders.Clone()
	if err = fillStreamRequestWithKeys(request, c.sessionPlacement, c.seqPlacement, c.sessionKey, c.seqKey, c.sessionID, c.behavior); err != nil {
		return err
	}
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			c.remoteAddr = info.Conn.RemoteAddr()
			c.localAddr = info.Conn.LocalAddr()
		},
	}))
	wrc := &waitReadCloser{Wait: make(chan struct{})}
	go func() {
		response, err := c.httpClient.Do(request)
		if err != nil {
			c.storeDownloadErr(err)
			wrc.Close()
			return
		}
		if response.StatusCode != http.StatusOK {
			err = newHTTPStatusError("xhttp download", response.Status)
			c.storeDownloadErr(err)
			response.Body.Close()
			wrc.Close()
			return
		}
		wrc.Set(response.Body)
	}()
	c.reader = wrc
	return nil
}

func (c *conn) startUploadStream() error {
	reader, writer := io.Pipe()
	request, err := http.NewRequestWithContext(c.ctx, c.behavior.uplinkHTTPMethod, c.baseURL, reader)
	if err != nil {
		reader.Close()
		writer.Close()
		return err
	}
	request.Host = c.host
	request.Header = c.headers.Clone()
	if err = fillStreamRequestWithKeys(request, c.sessionPlacement, c.seqPlacement, c.sessionKey, c.seqKey, c.sessionID, c.behavior); err != nil {
		reader.Close()
		writer.Close()
		return err
	}
	request.ContentLength = -1
	c.upload = writer
	go func() {
		response, err := c.httpClient.Do(request)
		if err != nil {
			c.storeUploadErr(err)
			_ = writer.CloseWithError(err)
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			err = newHTTPStatusError("xhttp upload", response.Status)
			c.storeUploadErr(err)
			_ = writer.CloseWithError(err)
			return
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = writer.Close()
	}()
	return nil
}

func (c *conn) Read(p []byte) (int, error) {
	if c.reader == nil {
		return 0, io.ErrClosedPipe
	}
	n, err := c.reader.Read(p)
	if err != nil {
		if stored := c.loadDownloadErr(); stored != nil && (err == io.ErrClosedPipe || err == io.EOF) {
			return n, *stored
		}
	}
	return n, err
}

func (c *conn) Write(p []byte) (int, error) {
	if c.mode == ModeStreamUp || c.mode == ModeStreamOne {
		if c.upload == nil {
			return 0, io.ErrClosedPipe
		}
		return c.upload.Write(p)
	}
	select {
	case <-c.ctx.Done():
		return 0, c.ctx.Err()
	case c.uploadQueue <- append([]byte(nil), p...):
		return len(p), nil
	}
}

func (c *conn) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	c.cancel()
	if c.upload != nil {
		_ = c.upload.Close()
	}
	close(c.uploadQueue)
	<-c.uploadDone
	if c.reader != nil {
		_ = c.reader.Close()
	}
	return nil
}

func (c *conn) runPacketUploadLoop() {
	defer close(c.uploadDone)
	var lastWrite time.Time
	for payload := range c.uploadQueue {
		maxUploadSize := int(c.behavior.scMaxEachPostBytes.rand())
		if maxUploadSize <= 0 {
			maxUploadSize = len(payload)
		}
		for offset := 0; offset < len(payload); {
			if !lastWrite.IsZero() {
				wait := time.Duration(c.behavior.scMinPostsIntervalMs.rand())*time.Millisecond - time.Since(lastWrite)
				time.Sleep(maxDuration(0, wait))
			}
			end := offset + maxUploadSize
			if end > len(payload) {
				end = len(payload)
			}
			chunk := payload[offset:end]
			offset = end
			seq := c.seq.Add(1) - 1
			requestURL, err := applySeqToURL(c.baseURL, c.seqPlacement, c.seqKey, int64String(seq))
			if err != nil {
				c.storeUploadErr(err)
				c.cancel()
				return
			}
			request, err := http.NewRequestWithContext(c.ctx, c.behavior.uplinkHTTPMethod, requestURL, nil)
			if err != nil {
				c.storeUploadErr(err)
				c.cancel()
				return
			}
			request.Host = c.host
			request.Header = c.headers.Clone()
			if err = fillPacketRequestWithKeys(request, c.sessionPlacement, c.seqPlacement, c.sessionKey, c.seqKey, c.uplinkPlacement, c.sessionID, int64String(seq), chunk, c.behavior); err != nil {
				c.storeUploadErr(err)
				c.cancel()
				return
			}
			response, err := c.httpClient.Do(request)
			if err != nil {
				c.storeUploadErr(err)
				c.cancel()
				return
			}
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				err = newHTTPStatusError("xhttp upload", response.Status)
				c.storeUploadErr(err)
				c.cancel()
				return
			}
			lastWrite = time.Now()
		}
	}
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func (c *conn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *conn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *conn) SetDeadline(t time.Time) error {
	_ = t
	return nil
}

func (c *conn) SetReadDeadline(t time.Time) error {
	_ = t
	return nil
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	_ = t
	return nil
}

var _ net.Conn = (*conn)(nil)

func newHTTPStatusError(action string, status string) error {
	return E.New(action, ": unexpected status: ", status)
}

type bytesReader []byte

func newBytesReader(p []byte) *bytesReader {
	b := bytesReader(append([]byte(nil), p...))
	return &b
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if len(*r) == 0 {
		return 0, io.EOF
	}
	n := copy(p, *r)
	*r = (*r)[n:]
	return n, nil
}

func (r *bytesReader) Close() error {
	return nil
}

func int64String(value int64) string {
	return strconv.FormatInt(value, 10)
}

func containsHTTP1Only(tlsConfig tls.Config) bool {
	if tlsConfig == nil {
		return false
	}
	nextProtos := tlsConfig.NextProtos()
	return len(nextProtos) == 1 && nextProtos[0] == "http/1.1"
}

func (c *conn) storeUploadErr(err error) {
	if err == nil {
		return
	}
	value := err
	c.uploadErr.CompareAndSwap(nil, &value)
}

func (c *conn) storeDownloadErr(err error) {
	if err == nil {
		return
	}
	value := err
	c.downloadErr.CompareAndSwap(nil, &value)
}

func (c *conn) loadDownloadErr() *error {
	return c.downloadErr.Load()
}

type waitReadCloser struct {
	Wait chan struct{}
	io.ReadCloser
}

func (w *waitReadCloser) Set(rc io.ReadCloser) {
	w.ReadCloser = rc
	defer func() {
		if recover() != nil {
			rc.Close()
		}
	}()
	close(w.Wait)
}

func (w *waitReadCloser) Read(b []byte) (int, error) {
	if w.ReadCloser == nil {
		if <-w.Wait; w.ReadCloser == nil {
			return 0, io.ErrClosedPipe
		}
	}
	return w.ReadCloser.Read(b)
}

func (w *waitReadCloser) Close() error {
	if w.ReadCloser != nil {
		return w.ReadCloser.Close()
	}
	defer func() {
		if recover() != nil && w.ReadCloser != nil {
			w.ReadCloser.Close()
		}
	}()
	close(w.Wait)
	return nil
}
