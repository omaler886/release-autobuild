package v2rayxhttp

import (
	"math/rand"
	"net/http"
	"strings"

	"github.com/sagernet/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"
)

type rangeConfig struct {
	From int32
	To   int32
}

func (r rangeConfig) rand() int32 {
	if r.From >= r.To {
		return r.From
	}
	return r.From + rand.Int31n(r.To-r.From+1)
}

type requestBehavior struct {
	xPaddingBytes        rangeConfig
	xPaddingObfsMode     bool
	xPaddingKey          string
	xPaddingHeader       string
	xPaddingPlacement    string
	xPaddingMethod       PaddingMethod
	uplinkHTTPMethod     string
	uplinkDataKey        string
	uplinkChunkSize      rangeConfig
	noGRPCHeader         bool
	noSSEHeader          bool
	scMaxEachPostBytes   rangeConfig
	scMinPostsIntervalMs rangeConfig
	scMaxBufferedPosts   int
	scStreamUpServerSecs rangeConfig
	serverMaxHeaderBytes int
}

func (b requestBehavior) requestPaddingConfig(rawURL string) xPaddingConfig {
	config := xPaddingConfig{Length: int(b.xPaddingBytes.rand())}
	if b.xPaddingObfsMode {
		config.Placement = xPaddingPlacement{
			Placement: b.xPaddingPlacement,
			Key:       b.xPaddingKey,
			Header:    b.xPaddingHeader,
			RawURL:    rawURL,
		}
		config.Method = b.xPaddingMethod
		return config
	}
	config.Placement = xPaddingPlacement{
		Placement: PlacementQueryInHeader,
		Key:       "x_padding",
		Header:    "Referer",
		RawURL:    rawURL,
	}
	config.Method = PaddingMethodRepeatX
	return config
}

func (b requestBehavior) responsePaddingConfig() xPaddingConfig {
	config := xPaddingConfig{Length: int(b.xPaddingBytes.rand())}
	if b.xPaddingObfsMode {
		config.Placement = xPaddingPlacement{
			Placement: b.xPaddingPlacement,
			Key:       b.xPaddingKey,
			Header:    b.xPaddingHeader,
		}
		config.Method = b.xPaddingMethod
		return config
	}
	config.Placement = xPaddingPlacement{
		Placement: PlacementHeader,
		Header:    "X-Padding",
	}
	config.Method = PaddingMethodRepeatX
	return config
}

func newRequestBehavior(mode string, uplinkPlacement string, options option.V2RayXHTTPOptions) (requestBehavior, error) {
	behavior := requestBehavior{
		xPaddingObfsMode: options.XPaddingObfsMode,
		noGRPCHeader:     options.NoGRPCHeader,
		noSSEHeader:      options.NoSSEHeader,
	}
	if options.XPaddingBytes != nil && (options.XPaddingBytes.From <= 0 || options.XPaddingBytes.To <= 0) {
		return behavior, E.New("xhttp x_padding_bytes cannot be disabled")
	}
	var err error
	behavior.xPaddingPlacement, err = normalizePaddingPlacement(options.XPaddingPlacement)
	if err != nil {
		return behavior, err
	}
	behavior.xPaddingMethod, err = normalizePaddingMethod(options.XPaddingMethod)
	if err != nil {
		return behavior, err
	}
	behavior.xPaddingBytes = normalizeRange(options.XPaddingBytes, 100, 1000)
	behavior.xPaddingKey = options.XPaddingKey
	if behavior.xPaddingKey == "" {
		behavior.xPaddingKey = "x_padding"
	}
	behavior.xPaddingHeader = options.XPaddingHeader
	if behavior.xPaddingHeader == "" {
		behavior.xPaddingHeader = "X-Padding"
	}
	behavior.uplinkHTTPMethod, err = normalizeUplinkHTTPMethod(mode, options.UplinkHTTPMethod)
	if err != nil {
		return behavior, err
	}
	behavior.scMaxEachPostBytes = normalizeRange(options.ScMaxEachPostBytes, 1_000_000, 1_000_000)
	if behavior.scMaxEachPostBytes.From <= 0 || behavior.scMaxEachPostBytes.To <= 0 {
		return behavior, E.New("xhttp sc_max_each_post_bytes must be greater than 0")
	}
	behavior.scMinPostsIntervalMs = normalizeRange(options.ScMinPostsIntervalMs, 30, 30)
	behavior.scMaxBufferedPosts = options.ScMaxBufferedPosts
	if behavior.scMaxBufferedPosts <= 0 {
		behavior.scMaxBufferedPosts = 30
	}
	behavior.scStreamUpServerSecs = normalizeRange(options.ScStreamUpServerSecs, 20, 80)
	behavior.serverMaxHeaderBytes, err = normalizeServerMaxHeaderBytes(options.ServerMaxHeaderBytes)
	if err != nil {
		return behavior, err
	}
	behavior.uplinkDataKey = normalizeUplinkDataKey(uplinkPlacement, options.UplinkDataKey)
	behavior.uplinkChunkSize = normalizeUplinkChunkSize(uplinkPlacement, options.UplinkChunkSize, behavior.scMaxEachPostBytes)
	return behavior, nil
}

func normalizeRange(input *option.V2RayXHTTPRangeOptions, defaultFrom int32, defaultTo int32) rangeConfig {
	if input == nil || input.To == 0 {
		return rangeConfig{From: defaultFrom, To: defaultTo}
	}
	return rangeConfig{From: input.From, To: input.To}
}

func normalizeUplinkChunkSize(placement string, input *option.V2RayXHTTPRangeOptions, maxEach rangeConfig) rangeConfig {
	if input == nil || input.To == 0 {
		switch placement {
		case PlacementCookie:
			return rangeConfig{From: 2 * 1024, To: 3 * 1024}
		case PlacementHeader:
			return rangeConfig{From: 3 * 1000, To: 4 * 1000}
		default:
			return maxEach
		}
	}
	from := input.From
	to := input.To
	if from < 64 {
		from = 64
	}
	if to < 64 {
		to = 64
	}
	return rangeConfig{From: from, To: to}
}

func normalizeUplinkDataKey(placement string, explicit string) string {
	if explicit != "" {
		return explicit
	}
	switch placement {
	case PlacementCookie:
		return "x_data"
	case PlacementHeader, PlacementAuto:
		return "X-Data"
	default:
		return ""
	}
}

func normalizePaddingPlacement(value string) (string, error) {
	switch strings.ToLower(value) {
	case "":
		return PlacementQueryInHeader, nil
	case PlacementCookie, PlacementHeader, PlacementQuery:
		return strings.ToLower(value), nil
	case strings.ToLower(PlacementQueryInHeader), "query_in_header", "query-in-header":
		return PlacementQueryInHeader, nil
	default:
		return "", E.New("unsupported xhttp x_padding_placement: ", value)
	}
}

func normalizePaddingMethod(value string) (PaddingMethod, error) {
	switch strings.ToLower(value) {
	case "", string(PaddingMethodRepeatX):
		return PaddingMethodRepeatX, nil
	case string(PaddingMethodTokenish):
		return PaddingMethodTokenish, nil
	default:
		return "", E.New("unsupported xhttp x_padding_method: ", value)
	}
}

func normalizeUplinkHTTPMethod(mode string, value string) (string, error) {
	method := strings.ToUpper(strings.TrimSpace(value))
	if method == "" {
		method = http.MethodPost
	}
	if method == http.MethodGet && mode != ModePacketUp {
		return "", E.New("xhttp uplink_http_method GET is only supported in packet-up mode")
	}
	return method, nil
}

func normalizeServerMaxHeaderBytes(value int) (int, error) {
	if value < 0 {
		return 0, E.New("xhttp server_max_header_bytes cannot be negative")
	}
	if value == 0 {
		return http.DefaultMaxHeaderBytes, nil
	}
	return value, nil
}
