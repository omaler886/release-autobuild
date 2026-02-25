package splithttp

import (
	"net"
	"sync"

	N "github.com/metacubex/mihomo/common/net"
)

type asyncStreamConn struct {
	net.Conn
	rAddr, lAddr *LazyAddr
	defaultHost  string
	onClose      func()
	closeOnce    sync.Once
}

func (c *asyncStreamConn) Close() error {
	c.closeOnce.Do(func() {
		if c.onClose != nil {
			c.onClose()
		}
	})
	return c.Conn.Close()
}

func (c *asyncStreamConn) LocalAddr() net.Addr {
	if addr := c.lAddr.Load(); addr != nil {
		return addr
	}
	return c.Conn.LocalAddr()
}

func (c *asyncStreamConn) RemoteAddr() net.Addr {
	if addr := c.rAddr.Load(); addr != nil {
		return addr
	}
	return N.NewCustomAddr("xhttp", c.defaultHost, nil)
}

func (c *asyncStreamConn) Upstream() any {
	// ⚡ 必须返回 nil 以阻止 Mihomo 内部启动冲突的异步预读协程
	return nil
}

func (c *asyncStreamConn) ReaderReplaceable() bool {
	return false // 告诉 Mihomo 这是一个私有协议流，禁止外部解包
}

func (c *asyncStreamConn) WriterReplaceable() bool {
	return false
}
