package v2rayxhttp

import (
	"crypto/rand"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/http2/hpack"
)

const PlacementQueryInHeader = "queryInHeader"

type PaddingMethod string

const (
	PaddingMethodRepeatX  PaddingMethod = "repeat-x"
	PaddingMethodTokenish PaddingMethod = "tokenish"
)

const (
	charsetBase62            = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	avgHuffmanBytesPerChar62 = 0.8
	paddingTolerance         = 2
)

type xPaddingPlacement struct {
	Placement string
	Key       string
	Header    string
	RawURL    string
}

type xPaddingConfig struct {
	Length    int
	Placement xPaddingPlacement
	Method    PaddingMethod
}

func randomBase62(length int) string {
	if length <= 0 {
		return ""
	}
	value := make([]byte, length)
	limit := big.NewInt(int64(len(charsetBase62)))
	for i := range value {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			value[i] = charsetBase62[i%len(charsetBase62)]
			continue
		}
		value[i] = charsetBase62[index.Int64()]
	}
	return string(value)
}

func generateTokenishPadding(target int) string {
	n := int(math.Ceil(float64(target) / avgHuffmanBytesPerChar62))
	if n < 1 {
		n = 1
	}
	padding := randomBase62(n)
	for i := 0; i < 150; i++ {
		current := int(hpack.HuffmanEncodeLength(padding))
		diff := current - target
		if diff >= -paddingTolerance && diff <= paddingTolerance {
			return padding
		}
		if diff < 0 {
			padding += "X"
		} else if len(padding) > 1 {
			padding = padding[:len(padding)-1]
		}
	}
	return padding
}

func generatePadding(method PaddingMethod, length int) string {
	if length <= 0 {
		return ""
	}
	if method == PaddingMethodTokenish {
		return generateTokenishPadding(length)
	}
	return strings.Repeat("X", length)
}

func applyXPaddingToHeader(header http.Header, config xPaddingConfig) {
	if header == nil {
		return
	}
	padding := generatePadding(config.Method, config.Length)
	switch placement := config.Placement; placement.Placement {
	case PlacementHeader:
		header.Set(placement.Header, padding)
	case PlacementQueryInHeader:
		parsed, err := url.Parse(placement.RawURL)
		if err != nil || parsed == nil {
			return
		}
		query := parsed.Query()
		query.Set(placement.Key, padding)
		parsed.RawQuery = query.Encode()
		header.Set(placement.Header, parsed.String())
	}
}

func applyXPaddingToRequest(request *http.Request, config xPaddingConfig) {
	if request == nil {
		return
	}
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	switch config.Placement.Placement {
	case PlacementHeader, PlacementQueryInHeader:
		applyXPaddingToHeader(request.Header, config)
		return
	}
	padding := generatePadding(config.Method, config.Length)
	switch config.Placement.Placement {
	case PlacementCookie:
		request.AddCookie(&http.Cookie{Name: config.Placement.Key, Value: padding, Path: "/"})
	case PlacementQuery:
		query := request.URL.Query()
		query.Set(config.Placement.Key, padding)
		request.URL.RawQuery = query.Encode()
	}
}

func applyXPaddingToResponse(writer http.ResponseWriter, config xPaddingConfig) {
	if writer == nil {
		return
	}
	switch config.Placement.Placement {
	case PlacementHeader, PlacementQueryInHeader:
		applyXPaddingToHeader(writer.Header(), config)
		return
	}
	if config.Placement.Placement == PlacementCookie {
		http.SetCookie(writer, &http.Cookie{
			Name:  config.Placement.Key,
			Value: generatePadding(config.Method, config.Length),
			Path:  "/",
		})
	}
}

func extractXPaddingFromRequest(request *http.Request, behavior requestBehavior) (string, string) {
	if request == nil {
		return "", ""
	}
	if !behavior.xPaddingObfsMode {
		referer := request.Header.Get("Referer")
		if referer != "" {
			parsed, err := url.Parse(referer)
			if err == nil && parsed != nil {
				return parsed.Query().Get("x_padding"), PlacementQueryInHeader
			}
		}
		return request.URL.Query().Get("x_padding"), PlacementQuery
	}
	if cookie, err := request.Cookie(behavior.xPaddingKey); err == nil && cookie != nil && cookie.Value != "" {
		return cookie.Value, PlacementCookie
	}
	headerValue := request.Header.Get(behavior.xPaddingHeader)
	if headerValue != "" {
		if behavior.xPaddingPlacement == PlacementHeader {
			return headerValue, PlacementHeader
		}
		parsed, err := url.Parse(headerValue)
		if err == nil && parsed != nil {
			return parsed.Query().Get(behavior.xPaddingKey), PlacementQueryInHeader
		}
	}
	return request.URL.Query().Get(behavior.xPaddingKey), PlacementQuery
}

func isXPaddingValid(value string, bounds rangeConfig, method PaddingMethod) bool {
	if value == "" {
		return false
	}
	switch method {
	case PaddingMethodTokenish:
		size := int32(hpack.HuffmanEncodeLength(value))
		from := bounds.From - paddingTolerance
		if from < 0 {
			from = 0
		}
		return size >= from && size <= bounds.To+paddingTolerance
	default:
		size := int32(len(value))
		return size >= bounds.From && size <= bounds.To
	}
}
