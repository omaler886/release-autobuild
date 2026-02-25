package splithttp

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metacubex/randv2"
)

type XmuxConn interface{ IsClosed() bool }

type XmuxClient struct {
	XmuxConn     XmuxConn
	OpenUsage    atomic.Int32
	leftUsage    int32
	LeftRequests atomic.Int32
	UnreusableAt time.Time
}

type XmuxManager struct {
	sync.Mutex
	xmuxConfig  XmuxConfig
	concurrency int32
	connections int32
	newConnFunc func() XmuxConn
	xmuxClients []*XmuxClient
}

func NewXmuxManager(xmuxConfig XmuxConfig, newConnFunc func() XmuxConn) *XmuxManager {
	return &XmuxManager{
		xmuxConfig:  xmuxConfig,
		concurrency: xmuxConfig.GetNormalizedMaxConcurrency().rand(),
		connections: xmuxConfig.GetNormalizedMaxConnections().rand(),
		newConnFunc: newConnFunc,
		xmuxClients: make([]*XmuxClient, 0),
	}
}

func (m *XmuxManager) newXmuxClient() *XmuxClient {
	xmuxClient := &XmuxClient{XmuxConn: m.newConnFunc(), leftUsage: -1}
	if x := m.xmuxConfig.GetNormalizedCMaxReuseTimes().rand(); x > 0 {
		xmuxClient.leftUsage = x - 1
	}
	xmuxClient.LeftRequests.Store(math.MaxInt32)
	if x := m.xmuxConfig.GetNormalizedHMaxRequestTimes().rand(); x > 0 {
		xmuxClient.LeftRequests.Store(x)
	}
	if x := m.xmuxConfig.GetNormalizedHMaxReusableSecs().rand(); x > 0 {
		xmuxClient.UnreusableAt = time.Now().Add(time.Duration(x) * time.Second)
	}
	m.xmuxClients = append(m.xmuxClients, xmuxClient)
	return xmuxClient
}

func (m *XmuxManager) GetXmuxClient(ctx context.Context) *XmuxClient {
	m.Lock()
	defer m.Unlock()

	activeClients := make([]*XmuxClient, 0, len(m.xmuxClients))
	availableClients := make([]*XmuxClient, 0)

	for _, xc := range m.xmuxClients {
		if xc.XmuxConn.IsClosed() || xc.LeftRequests.Load() <= 0 ||
			(!xc.UnreusableAt.IsZero() && time.Now().After(xc.UnreusableAt)) {
			continue
		}
		activeClients = append(activeClients, xc)
		if m.concurrency <= 0 || xc.OpenUsage.Load() < m.concurrency {
			availableClients = append(availableClients, xc)
		}
	}
	m.xmuxClients = activeClients

	// ⚡ 恢复负载均衡随机性
	if len(availableClients) > 0 {
		target := availableClients[randv2.IntN(len(availableClients))]
		if target.leftUsage > 0 {
			target.leftUsage -= 1
		}
		return target
	}

	if len(m.xmuxClients) < int(m.connections) || len(m.xmuxClients) == 0 {
		return m.newXmuxClient()
	}

	return m.xmuxClients[randv2.IntN(len(m.xmuxClients))]
}
