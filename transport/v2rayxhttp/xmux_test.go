package v2rayxhttp

import (
	"context"
	"testing"

	"github.com/sagernet/sing-box/option"

	"github.com/stretchr/testify/require"
)

type testXMuxConn struct{}

func (testXMuxConn) IsClosed() bool { return false }

func TestXMuxReuseLimit(t *testing.T) {
	t.Parallel()

	var created int
	manager := newXMuxManager(&option.V2RayXHTTPXMuxOptions{
		MaxConnections: &option.V2RayXHTTPRangeOptions{From: 1, To: 1},
		CMaxReuseTimes: &option.V2RayXHTTPRangeOptions{From: 2, To: 2},
	}, func() xmuxConn {
		created++
		return testXMuxConn{}
	})

	first := manager.getClient(context.Background())
	second := manager.getClient(context.Background())
	third := manager.getClient(context.Background())

	require.Same(t, first, second)
	require.NotSame(t, second, third)
	require.Equal(t, 2, created)
}
