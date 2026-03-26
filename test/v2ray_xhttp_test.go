package main

import (
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
)

func TestV2RayXHTTP(t *testing.T) {
	t.Run("self", func(t *testing.T) {
		testV2RayTransportSelf(t, &option.V2RayTransportOptions{
			Type: C.V2RayTransportTypeXHTTP,
		})
	})
	t.Run("stream-one", func(t *testing.T) {
		testV2RayTransportSelf(t, &option.V2RayTransportOptions{
			Type: C.V2RayTransportTypeXHTTP,
			XHTTPOptions: option.V2RayXHTTPOptions{
				Mode: "stream-one",
				Path: "/xhttp-stream-one",
			},
		})
	})
	t.Run("stream-up", func(t *testing.T) {
		testV2RayTransportSelf(t, &option.V2RayTransportOptions{
			Type: C.V2RayTransportTypeXHTTP,
			XHTTPOptions: option.V2RayXHTTPOptions{
				Mode: "stream-up",
				Path: "/xhttp-stream-up",
			},
		})
	})
}
