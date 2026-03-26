package v2rayxhttp

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/sagernet/sing-box/common/tls"
	"github.com/sagernet/sing-box/option"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"golang.org/x/net/http2"
)

func hostList(host string) []string {
	if host == "" {
		return nil
	}
	return []string{host}
}

func normalizePath(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path
}

func appendPathSegment(rawURL string, segment string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	parsed.Path += segment
	return parsed.String(), nil
}

func applySessionToURL(rawURL string, placement string, key string, session string) (string, error) {
	if placement == PlacementHeader || placement == PlacementCookie {
		return rawURL, nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	updated, err := applyMetaValue(parsed, nil, placement, session, defaultMetaKey(placement, key, true))
	if err != nil {
		return "", err
	}
	return updated.String(), nil
}

func applySeqToURL(rawURL string, placement string, key string, seq string) (string, error) {
	if placement == PlacementHeader || placement == PlacementCookie {
		return rawURL, nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	updated, err := applyMetaValue(parsed, nil, placement, seq, defaultMetaKey(placement, key, false))
	if err != nil {
		return "", err
	}
	return updated.String(), nil
}

func extractRequestMeta(request *http.Request, basePath string, sessionPlacement string, seqPlacement string, sessionKey string, seqKey string) (string, string, bool) {
	if request == nil || request.URL == nil {
		return "", "", false
	}
	if !strings.HasPrefix(request.URL.Path, basePath) {
		return "", "", false
	}
	var pathParts []string
	pathIndex := 0
	if sessionPlacement == PlacementPath || seqPlacement == PlacementPath {
		relative := strings.TrimPrefix(request.URL.Path, basePath)
		pathParts = strings.Split(relative, "/")
	}
	sessionID := ""
	switch sessionPlacement {
	case PlacementPath:
		if len(pathParts) > pathIndex && pathParts[pathIndex] != "" {
			sessionID = pathParts[pathIndex]
			pathIndex++
		}
	case PlacementQuery:
		sessionID = request.URL.Query().Get(defaultMetaKey(sessionPlacement, sessionKey, true))
	case PlacementHeader:
		sessionID = request.Header.Get(defaultMetaKey(sessionPlacement, sessionKey, true))
	case PlacementCookie:
		cookie, err := request.Cookie(defaultMetaKey(sessionPlacement, sessionKey, true))
		if err == nil {
			sessionID = cookie.Value
		}
	default:
		return "", "", false
	}
	seqText := ""
	switch seqPlacement {
	case PlacementPath:
		if len(pathParts) > pathIndex && pathParts[pathIndex] != "" {
			seqText = pathParts[pathIndex]
		}
	case PlacementQuery:
		seqText = request.URL.Query().Get(defaultMetaKey(seqPlacement, seqKey, false))
	case PlacementHeader:
		seqText = request.Header.Get(defaultMetaKey(seqPlacement, seqKey, false))
	case PlacementCookie:
		cookie, err := request.Cookie(defaultMetaKey(seqPlacement, seqKey, false))
		if err == nil {
			seqText = cookie.Value
		}
	default:
		return "", "", false
	}
	if seqText != "" {
		if _, err := strconv.ParseInt(seqText, 10, 64); err != nil {
			return "", "", false
		}
	}
	return sessionID, seqText, true
}

func newHTTPTransport(dialer N.Dialer, serverAddr M.Socksaddr, options option.V2RayXHTTPOptions, tlsConfig tls.Config) (http.RoundTripper, string, string, error) {
	var transport http.RoundTripper
	requestURL := &url.URL{}
	if tlsConfig == nil {
		requestURL.Scheme = "http"
		transport = &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, serverAddr)
			},
		}
	} else {
		requestURL.Scheme = "https"
		if len(tlsConfig.NextProtos()) == 0 {
			tlsConfig.SetNextProtos([]string{http2.NextProtoTLS})
		}
		tlsDialer := tls.NewDialer(dialer, tlsConfig)
		if len(tlsConfig.NextProtos()) == 1 && tlsConfig.NextProtos()[0] == "http/1.1" {
			transport = &http.Transport{
				DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return tlsDialer.DialTLSContext(ctx, serverAddr)
				},
			}
		} else {
			transport = &http2.Transport{
				DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.STDConfig) (net.Conn, error) {
					return tlsDialer.DialTLSContext(ctx, serverAddr)
				},
			}
		}
	}
	host := options.Host
	if host == "" {
		if tlsConfig != nil && tlsConfig.ServerName() != "" {
			host = tlsConfig.ServerName()
		} else {
			host = serverAddr.AddrString()
		}
	}
	requestURL.Host = host
	requestURL.Path = normalizePath(options.Path)
	return transport, requestURL.String(), host, nil
}

func closeIdleConnections(transport http.RoundTripper) {
	if connectionPool, ok := transport.(interface{ CloseIdleConnections() }); ok {
		connectionPool.CloseIdleConnections()
	}
}

func newDownloadSettings(requestURL string, host string, options option.V2RayXHTTPOptions) (string, string, http.Header, error) {
	downloadURL := requestURL
	downloadHost := host
	downloadHeaders := options.Headers.Build()
	if options.DownloadSettings == nil {
		return downloadURL, downloadHost, downloadHeaders, nil
	}
	parsed, err := url.Parse(requestURL)
	if err != nil {
		return "", "", nil, err
	}
	if options.DownloadSettings.Path != "" {
		parsed.Path = normalizePath(options.DownloadSettings.Path)
	}
	if options.DownloadSettings.Host != "" {
		downloadHost = options.DownloadSettings.Host
	}
	if options.DownloadSettings.Headers != nil {
		downloadHeaders = options.DownloadSettings.Headers.Build()
	}
	return parsed.String(), downloadHost, downloadHeaders, nil
}

func applyDefaultFetchHeaders(header http.Header) {
	if header == nil {
		return
	}
	if header.Get("User-Agent") == "" {
		header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36")
	}
	if header.Get("Accept") == "" {
		header.Set("Accept", "*/*")
	}
	if header.Get("Accept-Language") == "" {
		header.Set("Accept-Language", "en-US,en;q=0.9")
	}
	header.Set("Sec-Fetch-Mode", "cors")
	header.Set("Sec-Fetch-Dest", "empty")
	header.Set("Sec-Fetch-Site", "same-origin")
	if header.Get("Priority") == "" {
		header.Set("Priority", "u=1, i")
	}
	if header.Get("Cache-Control") == "" {
		header.Set("Cache-Control", "no-cache")
	}
	if header.Get("Pragma") == "" {
		header.Set("Pragma", "no-cache")
	}
	if header.Get("Referer") == "" {
		header.Set("Referer", "https://www.example.com/")
	}
}

func applyDefaultStreamHeaders(header http.Header) {
	applyDefaultFetchHeaders(header)
	if header.Get("Content-Type") == "" {
		header.Set("Content-Type", "application/grpc")
	}
}

func fillStreamRequest(request *http.Request, sessionPlacement string, seqPlacement string, sessionID string) error {
	return fillStreamRequestWithKeys(request, sessionPlacement, seqPlacement, "", "", sessionID)
}

func fillStreamRequestWithKeys(request *http.Request, sessionPlacement string, seqPlacement string, sessionKey string, seqKey string, sessionID string) error {
	request.Header = cloneOrNewHeader(request.Header)
	applyDefaultFetchHeaders(request.Header)
	if request.Body != nil && request.Header.Get("Content-Type") == "" {
		request.Header.Set("Content-Type", "application/grpc")
	}
	if err := applyRefererPadding(request); err != nil {
		return err
	}
	return applyMetaToRequest(request, sessionPlacement, seqPlacement, sessionKey, seqKey, sessionID, "")
}

func fillPacketRequest(request *http.Request, sessionPlacement string, seqPlacement string, uplinkPlacement string, sessionID string, seq string, payload []byte) error {
	return fillPacketRequestWithKeys(request, sessionPlacement, seqPlacement, "", "", uplinkPlacement, sessionID, seq, payload)
}

func fillPacketRequestWithKeys(request *http.Request, sessionPlacement string, seqPlacement string, sessionKey string, seqKey string, uplinkPlacement string, sessionID string, seq string, payload []byte) error {
	request.Header = cloneOrNewHeader(request.Header)
	applyDefaultFetchHeaders(request.Header)
	if err := applyPayloadToRequest(request, uplinkPlacement, payload); err != nil {
		return err
	}
	if err := applyRefererPadding(request); err != nil {
		return err
	}
	return applyMetaToRequest(request, sessionPlacement, seqPlacement, sessionKey, seqKey, sessionID, seq)
}

func applyPayloadToRequest(request *http.Request, placement string, payload []byte) error {
	switch placement {
	case "", PlacementAuto, PlacementBody:
		request.Body = ioNopCloserBytes(payload)
		request.ContentLength = int64(len(payload))
	case PlacementHeader:
		encoded := base64.RawURLEncoding.EncodeToString(payload)
		for i := 0; len(encoded) > 0; i++ {
			chunkSize := minInt(3800, len(encoded))
			chunk := encoded[:chunkSize]
			encoded = encoded[chunkSize:]
			request.Header.Set(fmt.Sprintf("x_data-%d", i), chunk)
		}
		request.ContentLength = 0
	case PlacementCookie:
		encoded := base64.RawURLEncoding.EncodeToString(payload)
		for i := 0; len(encoded) > 0; i++ {
			chunkSize := minInt(2500, len(encoded))
			chunk := encoded[:chunkSize]
			encoded = encoded[chunkSize:]
			request.Header.Add("Cookie", (&http.Cookie{Name: fmt.Sprintf("x_data_%d", i), Value: chunk}).String())
		}
		request.ContentLength = 0
	default:
		return fmt.Errorf("unsupported uplink data placement: %s", placement)
	}
	return nil
}

func extractPayloadFromRequest(request *http.Request, placement string, maxBodyBytes int64) ([]byte, error) {
	switch placement {
	case "", PlacementBody:
		if request.Body == nil {
			return nil, nil
		}
		defer request.Body.Close()
		return io.ReadAll(io.LimitReader(request.Body, maxBodyBytes))
	case PlacementAuto:
		payload, err := decodeChunkedHeader(request.Header, "x_data-")
		if err != nil {
			return nil, err
		}
		if len(payload) > 0 {
			return payload, nil
		}
		payload, err = decodeChunkedCookies(request, "x_data_")
		if err != nil {
			return nil, err
		}
		if len(payload) > 0 {
			return payload, nil
		}
		if request.Body == nil {
			return nil, nil
		}
		defer request.Body.Close()
		return io.ReadAll(io.LimitReader(request.Body, maxBodyBytes))
	case PlacementHeader:
		return decodeChunkedHeader(request.Header, "x_data-")
	case PlacementCookie:
		return decodeChunkedCookies(request, "x_data_")
	default:
		return nil, fmt.Errorf("unsupported uplink data placement: %s", placement)
	}
}

func decodeChunkedHeader(header http.Header, prefix string) ([]byte, error) {
	var chunks []string
	for i := 0; ; i++ {
		chunk := header.Get(fmt.Sprintf("%s%d", prefix, i))
		if chunk == "" {
			break
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	return base64.RawURLEncoding.DecodeString(strings.Join(chunks, ""))
}

func decodeChunkedCookies(request *http.Request, prefix string) ([]byte, error) {
	var chunks []string
	for i := 0; ; i++ {
		cookie, err := request.Cookie(fmt.Sprintf("%s%d", prefix, i))
		if err != nil {
			break
		}
		chunks = append(chunks, cookie.Value)
	}
	if len(chunks) == 0 {
		return nil, nil
	}
	return base64.RawURLEncoding.DecodeString(strings.Join(chunks, ""))
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func cloneOrNewHeader(header http.Header) http.Header {
	if header == nil {
		return make(http.Header)
	}
	return header.Clone()
}

func applyMetaToRequest(request *http.Request, sessionPlacement string, seqPlacement string, sessionKey string, seqKey string, sessionID string, seq string) error {
	var err error
	if sessionID != "" {
		request.URL, err = applyMetaValue(request.URL, request.Header, sessionPlacement, sessionID, defaultMetaKey(sessionPlacement, sessionKey, true))
		if err != nil {
			return err
		}
	}
	if seq != "" {
		request.URL, err = applyMetaValue(request.URL, request.Header, seqPlacement, seq, defaultMetaKey(seqPlacement, seqKey, false))
		if err != nil {
			return err
		}
	}
	return nil
}

func applyMetaValue(requestURL *url.URL, header http.Header, placement string, value string, key string) (*url.URL, error) {
	if requestURL == nil {
		return nil, fmt.Errorf("nil request url")
	}
	clone := *requestURL
	switch placement {
	case PlacementPath:
		clone.Path = appendURLPath(clone.Path, value)
	case PlacementQuery:
		query := clone.Query()
		query.Set(key, value)
		clone.RawQuery = query.Encode()
	case PlacementHeader:
		if header == nil {
			return nil, fmt.Errorf("nil header for header placement")
		}
		header.Set(key, value)
	case PlacementCookie:
		if header == nil {
			return nil, fmt.Errorf("nil header for cookie placement")
		}
		cookie := (&http.Cookie{Name: key, Value: value}).String()
		header.Add("Cookie", cookie)
	case "":
		clone.Path = appendURLPath(clone.Path, value)
	default:
		return nil, fmt.Errorf("unsupported placement: %s", placement)
	}
	return &clone, nil
}

func defaultMetaKey(placement string, explicit string, session bool) string {
	if explicit != "" {
		return explicit
	}
	if session {
		switch placement {
		case PlacementHeader:
			return "X-Session"
		case PlacementCookie, PlacementQuery:
			return "x_session"
		default:
			return ""
		}
	}
	switch placement {
	case PlacementHeader:
		return "X-Seq"
	case PlacementCookie, PlacementQuery:
		return "x_seq"
	default:
		return ""
	}
}

func appendURLPath(path string, value string) string {
	if strings.HasSuffix(path, "/") {
		return path + value
	}
	return path + "/" + value
}

func applyRefererPadding(request *http.Request) error {
	if request == nil || request.URL == nil {
		return nil
	}
	paddedURL := *request.URL
	padding, err := generatePadding(100)
	if err != nil {
		return err
	}
	query := paddedURL.Query()
	query.Set("x_padding", padding)
	paddedURL.RawQuery = query.Encode()
	request.Header.Set("Referer", paddedURL.String())
	return nil
}

func generatePadding(length int) (string, error) {
	if length <= 0 {
		return "", nil
	}
	random := make([]byte, length)
	_, err := rand.Read(random)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(random)
	if len(encoded) > length {
		encoded = encoded[:length]
	}
	return encoded, nil
}

type bytesReadCloser struct {
	reader *bytesReader
}

func ioNopCloserBytes(payload []byte) io.ReadCloser {
	return &bytesReadCloser{reader: newBytesReader(payload)}
}

func (b *bytesReadCloser) Read(p []byte) (int, error) {
	return b.reader.Read(p)
}

func (b *bytesReadCloser) Close() error {
	return nil
}
