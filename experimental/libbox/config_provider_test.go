package libbox

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckConfigWithInlineProvider(t *testing.T) {
	config := `{
  "providers": [
    {
      "type": "inline",
      "tag": "provider",
      "outbounds": [
        {
          "type": "direct",
          "tag": "node-a"
        }
      ]
    }
  ],
  "outbounds": [
    {
      "type": "selector",
      "tag": "selector",
      "providers": [
        "provider"
      ]
    }
  ]
}`

	require.NoError(t, CheckConfig(config))
}
