#!/usr/bin/env bash
set -euo pipefail

unset HTTP_PROXY HTTPS_PROXY ALL_PROXY http_proxy https_proxy all_proxy NO_PROXY no_proxy

go test ./transport/v2rayxhttp ./experimental/libbox ./provider/... ./protocol/group/... ./experimental/clashapi/... ./route/rule -count=1
go build ./cmd/sing-box

pushd test >/dev/null
go test ./... -run '^(TestOptionsWrapper|TestV2RayXHTTP)$' -count=1
popd >/dev/null
