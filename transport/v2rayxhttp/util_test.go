package v2rayxhttp

import (
	"net/http"
	"testing"

	"github.com/sagernet/sing-box/option"

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
			behavior, err := newRequestBehavior(ModePacketUp, testCase.placement, option.V2RayXHTTPOptions{
				UplinkDataPlacement: testCase.placement,
			})
			require.NoError(t, err)

			err = applyPayloadToRequest(request, testCase.placement, behavior, testCase.payload)
			require.NoError(t, err)

			payload, err := extractPayloadFromRequest(request, PlacementAuto, behavior, 1<<20)
			require.NoError(t, err)
			require.Equal(t, testCase.payload, payload)
		})
	}
}

func TestPayloadCustomKeyAndChunkSize(t *testing.T) {
	t.Parallel()

	behavior, err := newRequestBehavior(ModePacketUp, PlacementHeader, option.V2RayXHTTPOptions{
		UplinkDataPlacement: PlacementHeader,
		UplinkDataKey:       "X-Custom-Data",
		UplinkChunkSize: &option.V2RayXHTTPRangeOptions{
			From: 4,
			To:   4,
		},
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, "https://example.org/xhttp/", nil)
	require.NoError(t, err)

	payloadIn := []byte("hello-custom-key")
	err = applyPayloadToRequest(request, PlacementHeader, behavior, payloadIn)
	require.NoError(t, err)
	require.NotEmpty(t, request.Header.Get("X-Custom-Data-0"))

	payloadOut, err := extractPayloadFromRequest(request, PlacementHeader, behavior, 1<<20)
	require.NoError(t, err)
	require.Equal(t, payloadIn, payloadOut)
}

func TestXPaddingObfsRoundTrip(t *testing.T) {
	t.Parallel()

	behavior, err := newRequestBehavior(ModePacketUp, PlacementBody, option.V2RayXHTTPOptions{
		XPaddingObfsMode:  true,
		XPaddingKey:       "pad",
		XPaddingHeader:    "X-Pad",
		XPaddingPlacement: "query_in_header",
		XPaddingMethod:    "tokenish",
		XPaddingBytes: &option.V2RayXHTTPRangeOptions{
			From: 32,
			To:   32,
		},
	})
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, "https://example.org/xhttp/", nil)
	require.NoError(t, err)

	applyXPaddingToRequest(request, behavior.requestPaddingConfig(request.URL.String()))
	paddingValue, placement := extractXPaddingFromRequest(request, behavior)
	require.Equal(t, PlacementQueryInHeader, placement)
	require.True(t, isXPaddingValid(paddingValue, behavior.xPaddingBytes, behavior.xPaddingMethod))
}
