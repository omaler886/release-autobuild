# XHTTP Patch Notes

This branch tracks `SagerNet/sing-box` `testing` and adds xhttp transport support on top of upstream.

Patch scope:

- add the `xhttp` transport constant and option wiring
- register xhttp in the V2Ray transport client and server factory
- add the `transport/v2rayxhttp` implementation
- add transport-level tests under `test/v2ray_xhttp_test.go`
- complete xhttp metadata placement support for `path`, `query`, `header`, and `cookie`
- honor custom `session_key` and `seq_key`
- support `uplink_data_placement: auto`
- default TLS ALPN to `h2,http/1.1` for xhttp listeners

Maintenance model:

- `upstream-testing` is force-synced to upstream `testing`
- `xhttp` merges upstream changes instead of rebasing automatically
- daily GitHub Actions builds publish artifacts when the merge succeeds
- merge conflicts are treated as manual maintenance events
