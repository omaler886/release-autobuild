package v2rayxhttp

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyAndExtractMeta(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		sessionPlacement string
		sessionKey       string
		seqPlacement     string
		seqKey           string
	}{
		{name: "path", sessionPlacement: PlacementPath, seqPlacement: PlacementPath},
		{name: "query", sessionPlacement: PlacementQuery, sessionKey: "session_id", seqPlacement: PlacementQuery, seqKey: "seq_id"},
		{name: "header", sessionPlacement: PlacementHeader, sessionKey: "X-Session-ID", seqPlacement: PlacementHeader, seqKey: "X-Seq-ID"},
		{name: "cookie", sessionPlacement: PlacementCookie, sessionKey: "session_cookie", seqPlacement: PlacementCookie, seqKey: "seq_cookie"},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			request, err := http.NewRequest(http.MethodPost, "https://example.org/xhttp/", nil)
			require.NoError(t, err)

			err = applyMetaToRequest(request, testCase.sessionPlacement, testCase.seqPlacement, testCase.sessionKey, testCase.seqKey, "session-1", "7")
			require.NoError(t, err)

			sessionID, seqText, ok := extractRequestMeta(request, "/xhttp/", testCase.sessionPlacement, testCase.seqPlacement, testCase.sessionKey, testCase.seqKey)
			require.True(t, ok)
			require.Equal(t, "session-1", sessionID)
			require.Equal(t, "7", seqText)
		})
	}
}

func TestExtractPayloadAuto(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		placement string
		payload   []byte
	}{
		{name: "body", placement: PlacementBody, payload: []byte("body-payload")},
		{name: "header", placement: PlacementHeader, payload: []byte("header-payload")},
		{name: "cookie", placement: PlacementCookie, payload: []byte("cookie-payload")},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			request, err := http.NewRequest(http.MethodPost, "https://example.org/xhttp/", nil)
			require.NoError(t, err)

			err = applyPayloadToRequest(request, testCase.placement, testCase.payload)
			require.NoError(t, err)

			payload, err := extractPayloadFromRequest(request, PlacementAuto, 1<<20)
			require.NoError(t, err)
			require.Equal(t, testCase.payload, payload)
		})
	}
}
