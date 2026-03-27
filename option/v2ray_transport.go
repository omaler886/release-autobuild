package option

import (
	"bytes"

	C "github.com/sagernet/sing-box/constant"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/json"
	"github.com/sagernet/sing/common/json/badjson"
	"github.com/sagernet/sing/common/json/badoption"
)

type _V2RayTransportOptions struct {
	Type               string                  `json:"type"`
	HTTPOptions        V2RayHTTPOptions        `json:"-"`
	WebsocketOptions   V2RayWebsocketOptions   `json:"-"`
	QUICOptions        V2RayQUICOptions        `json:"-"`
	GRPCOptions        V2RayGRPCOptions        `json:"-"`
	HTTPUpgradeOptions V2RayHTTPUpgradeOptions `json:"-"`
	XHTTPOptions       V2RayXHTTPOptions       `json:"-"`
}

type V2RayTransportOptions _V2RayTransportOptions

func (o V2RayTransportOptions) MarshalJSON() ([]byte, error) {
	var v any
	switch o.Type {
	case C.V2RayTransportTypeHTTP:
		v = o.HTTPOptions
	case C.V2RayTransportTypeWebsocket:
		v = o.WebsocketOptions
	case C.V2RayTransportTypeQUIC:
		v = o.QUICOptions
	case C.V2RayTransportTypeGRPC:
		v = o.GRPCOptions
	case C.V2RayTransportTypeHTTPUpgrade:
		v = o.HTTPUpgradeOptions
	case C.V2RayTransportTypeXHTTP:
		v = o.XHTTPOptions
	case "":
		return nil, E.New("missing transport type")
	default:
		return nil, E.New("unknown transport type: " + o.Type)
	}
	return badjson.MarshallObjects((_V2RayTransportOptions)(o), v)
}

func (o *V2RayTransportOptions) UnmarshalJSON(bytes []byte) error {
	err := json.Unmarshal(bytes, (*_V2RayTransportOptions)(o))
	if err != nil {
		return err
	}
	var v any
	switch o.Type {
	case C.V2RayTransportTypeHTTP:
		v = &o.HTTPOptions
	case C.V2RayTransportTypeWebsocket:
		v = &o.WebsocketOptions
	case C.V2RayTransportTypeQUIC:
		v = &o.QUICOptions
	case C.V2RayTransportTypeGRPC:
		v = &o.GRPCOptions
	case C.V2RayTransportTypeHTTPUpgrade:
		v = &o.HTTPUpgradeOptions
	case C.V2RayTransportTypeXHTTP:
		v = &o.XHTTPOptions
	default:
		return E.New("unknown transport type: " + o.Type)
	}
	err = badjson.UnmarshallExcluded(bytes, (*_V2RayTransportOptions)(o), v)
	if err != nil {
		return err
	}
	return nil
}

type V2RayHTTPOptions struct {
	Host        badoption.Listable[string] `json:"host,omitempty"`
	Path        string                     `json:"path,omitempty"`
	Method      string                     `json:"method,omitempty"`
	Headers     badoption.HTTPHeader       `json:"headers,omitempty"`
	IdleTimeout badoption.Duration         `json:"idle_timeout,omitempty"`
	PingTimeout badoption.Duration         `json:"ping_timeout,omitempty"`
}

type V2RayWebsocketOptions struct {
	Path                string               `json:"path,omitempty"`
	Headers             badoption.HTTPHeader `json:"headers,omitempty"`
	MaxEarlyData        uint32               `json:"max_early_data,omitempty"`
	EarlyDataHeaderName string               `json:"early_data_header_name,omitempty"`
}

type V2RayQUICOptions struct{}

type V2RayGRPCOptions struct {
	ServiceName         string             `json:"service_name,omitempty"`
	IdleTimeout         badoption.Duration `json:"idle_timeout,omitempty"`
	PingTimeout         badoption.Duration `json:"ping_timeout,omitempty"`
	PermitWithoutStream bool               `json:"permit_without_stream,omitempty"`
	ForceLite           bool               `json:"-"` // for test
}

type V2RayHTTPUpgradeOptions struct {
	Host    string               `json:"host,omitempty"`
	Path    string               `json:"path,omitempty"`
	Headers badoption.HTTPHeader `json:"headers,omitempty"`
}

type V2RayXHTTPOptions struct {
	Host                 string                      `json:"host,omitempty"`
	Path                 string                      `json:"path,omitempty"`
	Mode                 string                      `json:"mode,omitempty"`
	XPaddingBytes        *V2RayXHTTPRangeOptions     `json:"x_padding_bytes,omitempty"`
	XPaddingObfsMode     bool                        `json:"x_padding_obfs_mode,omitempty"`
	XPaddingKey          string                      `json:"x_padding_key,omitempty"`
	XPaddingHeader       string                      `json:"x_padding_header,omitempty"`
	XPaddingPlacement    string                      `json:"x_padding_placement,omitempty"`
	XPaddingMethod       string                      `json:"x_padding_method,omitempty"`
	UplinkHTTPMethod     string                      `json:"uplink_http_method,omitempty"`
	NoGRPCHeader         bool                        `json:"no_grpc_header,omitempty"`
	NoSSEHeader          bool                        `json:"no_sse_header,omitempty"`
	SessionPlacement     string                      `json:"session_placement,omitempty"`
	SessionKey           string                      `json:"session_key,omitempty"`
	SeqPlacement         string                      `json:"seq_placement,omitempty"`
	SeqKey               string                      `json:"seq_key,omitempty"`
	UplinkDataPlacement  string                      `json:"uplink_data_placement,omitempty"`
	UplinkDataKey        string                      `json:"uplink_data_key,omitempty"`
	UplinkChunkSize      *V2RayXHTTPRangeOptions     `json:"uplink_chunk_size,omitempty"`
	ScMaxEachPostBytes   *V2RayXHTTPRangeOptions     `json:"sc_max_each_post_bytes,omitempty"`
	ScMinPostsIntervalMs *V2RayXHTTPRangeOptions     `json:"sc_min_posts_interval_ms,omitempty"`
	ScMaxBufferedPosts   int                         `json:"sc_max_buffered_posts,omitempty"`
	ScStreamUpServerSecs *V2RayXHTTPRangeOptions     `json:"sc_stream_up_server_secs,omitempty"`
	ServerMaxHeaderBytes int                         `json:"server_max_header_bytes,omitempty"`
	DownloadSettings     *V2RayXHTTPDownloadSettings `json:"download_settings,omitempty"`
	XMux                 *V2RayXHTTPXMuxOptions      `json:"xmux,omitempty"`
	Headers              badoption.HTTPHeader        `json:"headers,omitempty"`
}

type V2RayXHTTPDownloadSettings struct {
	Host    string               `json:"host,omitempty"`
	Path    string               `json:"path,omitempty"`
	Headers badoption.HTTPHeader `json:"headers,omitempty"`
}

type V2RayXHTTPXMuxOptions struct {
	MaxConcurrency   *V2RayXHTTPRangeOptions `json:"max_concurrency,omitempty"`
	MaxConnections   *V2RayXHTTPRangeOptions `json:"max_connections,omitempty"`
	CMaxReuseTimes   *V2RayXHTTPRangeOptions `json:"c_max_reuse_times,omitempty"`
	HMaxRequestTimes *V2RayXHTTPRangeOptions `json:"h_max_request_times,omitempty"`
	HMaxReusableSecs *V2RayXHTTPRangeOptions `json:"h_max_reusable_secs,omitempty"`
	HKeepAlivePeriod int64                   `json:"h_keep_alive_period,omitempty"`
}

type V2RayXHTTPRangeOptions struct {
	From int32 `json:"from,omitempty"`
	To   int32 `json:"to,omitempty"`
}

func (r *V2RayXHTTPRangeOptions) UnmarshalJSON(content []byte) error {
	content = bytes.TrimSpace(content)
	if len(content) == 0 || bytes.Equal(content, []byte("null")) {
		return nil
	}
	var single int32
	if err := json.Unmarshal(content, &single); err == nil {
		r.From = single
		r.To = single
		return nil
	}
	type rawRangeOptions V2RayXHTTPRangeOptions
	var raw rawRangeOptions
	if err := json.Unmarshal(content, &raw); err != nil {
		return err
	}
	*r = V2RayXHTTPRangeOptions(raw)
	if r.From != 0 && r.To == 0 {
		r.To = r.From
	}
	return nil
}
