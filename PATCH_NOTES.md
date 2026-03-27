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
- `upstream-yelnoo-stable` and `upstream-mihomo-splithttp` mirror the donor sources used for provider/xhttp follow-up
- `.github/upstream-tracking.env` records the last seen heads for all three source repositories
- `xhttp` merges upstream `testing` changes instead of rebasing automatically
- scheduled GitHub Actions runs verify the merged branch before pushing it
- donor mirror updates from `yelnoo` and `mihomo` are surfaced for manual port review instead of being blindly merged
- successful scheduled runs still publish build artifacts
- merge conflicts are treated as manual maintenance events
