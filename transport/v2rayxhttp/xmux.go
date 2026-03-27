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
	leftUsage    int32
	leftRequests atomic.Int32
	unreusableAt time.Time
}

type xmuxManager struct {
	access      sync.Mutex
	concurrency int32
	connections int32
	options     *option.V2RayXHTTPXMuxOptions
	newConnFunc func() xmuxConn
	clients     []*xmuxClient
}

func newXMuxManager(config *option.V2RayXHTTPXMuxOptions, newConnFunc func() xmuxConn) *xmuxManager {
	manager := &xmuxManager{options: config, newConnFunc: newConnFunc}
	if config != nil {
		manager.concurrency = rangeValue(config.MaxConcurrency)
		manager.connections = rangeValue(config.MaxConnections)
	}
	return manager
}

func (m *xmuxManager) getClient(ctx context.Context) *xmuxClient {
	_ = ctx
	m.access.Lock()
	defer m.access.Unlock()
	for i := 0; i < len(m.clients); {
		client := m.clients[i]
		if client.conn.IsClosed() || client.leftUsage == 0 || client.leftRequests.Load() <= 0 || (!client.unreusableAt.IsZero() && time.Now().After(client.unreusableAt)) {
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
	if selected.leftUsage > 0 {
		selected.leftUsage--
	}
	selected.leftRequests.Add(-1)
	return selected
}

func (m *xmuxManager) newClient() *xmuxClient {
	client := &xmuxClient{conn: m.newConnFunc(), leftUsage: -1}
	client.leftRequests.Store(1 << 30)
	if config := m.optionsConfig(); config != nil {
		if reuse := rangeValue(config.CMaxReuseTimes); reuse > 0 {
			client.leftUsage = reuse - 1
		}
		if requests := rangeValue(config.HMaxRequestTimes); requests > 0 {
			client.leftRequests.Store(requests)
		}
		if reusableSecs := rangeValue(config.HMaxReusableSecs); reusableSecs > 0 {
			client.unreusableAt = time.Now().Add(time.Duration(reusableSecs) * time.Second)
		}
	}
	m.clients = append(m.clients, client)
	return client
}

func (m *xmuxManager) optionsConfig() *option.V2RayXHTTPXMuxOptions {
	if m == nil {
		return nil
	}
	return m.options
}

func rangeValue(config *option.V2RayXHTTPRangeOptions) int32 {
	if config == nil {
		return 0
	}
	if config.To == 0 {
		return config.From
	}
	if config.From >= config.To {
		return config.From
	}
	return config.From + rand.Int31n(config.To-config.From+1)
}
