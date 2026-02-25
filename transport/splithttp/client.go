package splithttp

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"sync/atomic"

	"github.com/metacubex/http"
	"github.com/metacubex/http/httptrace"
)

// 🚀 自定义异步 Reader：消除标准库 io.Pipe 的同步损耗
type asyncBodyReader struct {
	ready chan struct{}
	body  io.ReadCloser
	err   error
}

func (r *asyncBodyReader) Read(p []byte) (int, error) {
	<-r.ready
	if r.err != nil {
		return 0, r.err
	}
	return r.body.Read(p)
}

func (r *asyncBodyReader) Close() error {
	<-r.ready
	if r.body != nil {
		return r.body.Close()
	}
	return nil
}

type LazyAddr struct {
	addr atomic.Pointer[net.Addr]
}

func (l *LazyAddr) Store(addr net.Addr) { l.addr.Store(&addr) }
func (l *LazyAddr) Load() net.Addr {
	if a := l.addr.Load(); a != nil {
		return *a
	}
	return nil
}

type DialerClient interface {
	IsClosed() bool
	OpenStream(ctx context.Context, url string, sessionId string, body io.Reader, uploadOnly bool, rAddr, lAddr *LazyAddr) (io.ReadCloser, error)
	PostPacket(context.Context, string, string, string, io.ReadCloser) error
}

type DefaultDialerClient struct {
	transportConfig *Config
	client          *http.Client
	closed          atomic.Bool
}

func (c *DefaultDialerClient) IsClosed() bool { return c.closed.Load() }

func (c *DefaultDialerClient) applyXrayPadding(req *http.Request, urlStr string) {
	length := int(c.transportConfig.GetNormalizedXPaddingBytes().rand())
	config := XPaddingConfig{Length: length}
	if c.transportConfig.XPaddingObfsMode {
		config.Placement = XPaddingPlacement{
			Placement: c.transportConfig.XPaddingPlacement,
			Key:       c.transportConfig.XPaddingKey,
			Header:    c.transportConfig.XPaddingHeader,
			RawURL:    urlStr,
		}
		config.Method = PaddingMethod(c.transportConfig.XPaddingMethod)
	} else {
		config.Placement = XPaddingPlacement{
			Placement: PlacementQueryInHeader,
			Key:       "x_padding",
			Header:    "Referer",
			RawURL:    urlStr,
		}
	}
	c.transportConfig.ApplyXPaddingToRequest(req, config)
}

func (c *DefaultDialerClient) OpenStream(ctx context.Context, url string, sessionId string, body io.Reader, uploadOnly bool, rAddr, lAddr *LazyAddr) (io.ReadCloser, error) {
	ar := &asyncBodyReader{ready: make(chan struct{})}

	go func() {
		traceCtx := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
			GotConn: func(info httptrace.GotConnInfo) {
				rAddr.Store(info.Conn.RemoteAddr())
				lAddr.Store(info.Conn.LocalAddr())
			},
		})

		method := "GET"
		if body != nil {
			method = c.transportConfig.GetNormalizedUplinkHTTPMethod()
		}

		req, err := http.NewRequestWithContext(traceCtx, method, url, body)
		if err != nil {
			ar.err = err
			close(ar.ready)
			return
		}

		req.Header = c.transportConfig.GetRequestHeader()
		c.applyXrayPadding(req, url)
		c.transportConfig.ApplyMetaToRequest(req, sessionId, "")

		if method == c.transportConfig.GetNormalizedUplinkHTTPMethod() && !c.transportConfig.NoGRPCHeader {
			req.Header.Set("Content-Type", "application/grpc")
		}

		resp, err := c.client.Do(req)
		if err != nil {
			if !uploadOnly {
				c.closed.Store(true)
			}
			ar.err = err
			close(ar.ready)
			return
		}

		if resp.StatusCode != 200 {
			_ = resp.Body.Close()
			ar.err = fmt.Errorf("bad status %d", resp.StatusCode)
			close(ar.ready)
			return
		}

		if uploadOnly {
			go func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
			close(ar.ready)
		} else {
			ar.body = resp.Body
			close(ar.ready)
		}
	}()

	return ar, nil
}

func (c *DefaultDialerClient) PostPacket(ctx context.Context, url string, sessionId string, seqStr string, payload io.ReadCloser) error {
	defer payload.Close()
	dataPlacement := c.transportConfig.GetNormalizedUplinkDataPlacement()
	var req *http.Request
	var err error

	if dataPlacement == PlacementBody {
		req, err = http.NewRequestWithContext(ctx, c.transportConfig.GetNormalizedUplinkHTTPMethod(), url, payload)
	} else {
		// 🚀 零分配优化：如果是池化 Reader，直接拿底层 Bytes
		var data []byte
		if pr, ok := payload.(*pooledBodyReader); ok {
			pr.mu.Lock()
			if pr.buf != nil {
				data = pr.buf.Bytes()
			}
			pr.mu.Unlock()
		} else {
			data, _ = io.ReadAll(payload)
		}

		encodedData := base64.RawURLEncoding.EncodeToString(data)
		req, err = http.NewRequestWithContext(ctx, c.transportConfig.GetNormalizedUplinkHTTPMethod(), url, nil)
		if err == nil {
			key := c.transportConfig.UplinkDataKey
			cSize := int(c.transportConfig.UplinkChunkSize)
			if cSize <= 0 {
				cSize = len(encodedData)
			}

			if dataPlacement == PlacementHeader {
				for i := 0; i < len(encodedData); i += cSize {
					end := i + cSize
					if end > len(encodedData) {
						end = len(encodedData)
					}
					req.Header.Set(fmt.Sprintf("%s-%d", key, i/cSize), encodedData[i:end])
				}
				req.Header.Set(key+"-Length", fmt.Sprintf("%d", len(encodedData)))
				req.Header.Set(key+"-Upstream", "1")
			} else if dataPlacement == PlacementCookie {
				for i := 0; i < len(encodedData); i += cSize {
					end := i + cSize
					if end > len(encodedData) {
						end = len(encodedData)
					}
					req.AddCookie(&http.Cookie{Name: fmt.Sprintf("%s_%d", key, i/cSize), Value: encodedData[i:end]})
				}
				req.AddCookie(&http.Cookie{Name: key + "_upstream", Value: "1"})
			}
		}
	}

	if err != nil {
		return err
	}
	req.Header = c.transportConfig.GetRequestHeader()
	if dataPlacement == PlacementBody && !c.transportConfig.NoGRPCHeader {
		req.Header.Set("Content-Type", "application/grpc")
	}
	c.applyXrayPadding(req, url)
	c.transportConfig.ApplyMetaToRequest(req, sessionId, seqStr)

	resp, err := c.client.Do(req)
	if err != nil {
		c.closed.Store(true)
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("bad status code: %s", resp.Status)
	}
	return nil
}
