package v2rayxhttp

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sagernet/sing-box/option"
)

type xmuxConn interface {
	IsClosed() bool
}

type xmuxClient struct {
	conn         xmuxConn
	openUsage    atomic.Int32
	leftRequests atomic.Int32
}

type xmuxManager struct {
	access      sync.Mutex
	concurrency int32
	connections int32
	newConnFunc func() xmuxConn
	clients     []*xmuxClient
}

func newXMuxManager(config *option.V2RayXHTTPXMuxOptions, newConnFunc func() xmuxConn) *xmuxManager {
	manager := &xmuxManager{newConnFunc: newConnFunc}
	if config != nil {
		manager.concurrency = int32(config.MaxConcurrency)
		manager.connections = int32(config.MaxConnections)
	}
	return manager
}

func (m *xmuxManager) getClient(ctx context.Context) *xmuxClient {
	_ = ctx
	m.access.Lock()
	defer m.access.Unlock()
	for i := 0; i < len(m.clients); {
		client := m.clients[i]
		if client.conn.IsClosed() || client.leftRequests.Load() <= 0 {
			m.clients = append(m.clients[:i], m.clients[i+1:]...)
			continue
		}
		i++
	}
	if len(m.clients) == 0 || (m.connections > 0 && len(m.clients) < int(m.connections)) {
		return m.newClient()
	}
	pool := m.clients
	if m.concurrency > 0 {
		pool = make([]*xmuxClient, 0, len(m.clients))
		for _, client := range m.clients {
			if client.openUsage.Load() < m.concurrency {
				pool = append(pool, client)
			}
		}
		if len(pool) == 0 {
			return m.newClient()
		}
	}
	selected := pool[rand.New(rand.NewSource(time.Now().UnixNano())).Intn(len(pool))]
	selected.leftRequests.Add(-1)
	return selected
}

func (m *xmuxManager) newClient() *xmuxClient {
	client := &xmuxClient{conn: m.newConnFunc()}
	client.leftRequests.Store(1 << 30)
	m.clients = append(m.clients, client)
	return client
}
