> Sponsored by [Warp](https://go.warp.dev/sing-box), built for coding with multiple AI agents

<a href="https://go.warp.dev/sing-box">
<img alt="Warp sponsorship" width="400" src="https://github.com/warpdotdev/brand-assets/raw/refs/heads/main/Github/Sponsor/Warp-Github-LG-02.png">
</a>

---

# sing-box

The universal proxy platform.

## XHTTP Branch

This repository carries an xhttp transport patch on the `xhttp` branch while tracking `SagerNet/sing-box` `testing`.

- `upstream-testing` is kept as a clean mirror of upstream `testing`.
- `upstream-yelnoo-stable` mirrors `yelnoo/sing-box` `stable`.
- `upstream-mihomo-splithttp` mirrors `daiaji/mihomo` `feat/splithttp`.
- `xhttp` carries the local patch set.
- `.github/upstream-tracking.env` records the last seen heads for the three source repositories.
- `.github/workflows/sync-build.yml` refreshes mirrors every 6 hours, merges official `testing` into `xhttp`, runs focused xhttp/provider verification, and only then pushes `xhttp`.
- donor changes from `yelnoo/sing-box` and `daiaji/mihomo` are mirrored and recorded for follow-up review instead of being blindly merged.
- When upstream conflicts with the patch, the workflow fails and requires a manual conflict fix.

[![Packaging status](https://repology.org/badge/vertical-allrepos/sing-box.svg)](https://repology.org/project/sing-box/versions)

## Documentation

https://sing-box.sagernet.org

For extended features

- Providers: [中文](./docs/configuration/provider/index.zh.md), [English](./docs/configuration/provider/index.md)

## License

```
Copyright (C) 2022 by nekohasekai <contact-sagernet@sekai.icu>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.

In addition, no derivative work may use the name or imply association
with this application without prior consent.
```
