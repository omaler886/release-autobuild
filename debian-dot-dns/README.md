# Debian VPS 原生 DoT DNS 安装方案

这个方案只使用 VPS 自带的 `systemd-resolved`，不下载 `dnsproxy`、`cloudflared`、`mosdns` 等额外二进制。

它把系统 DNS 恢复到 `systemd-resolved` 的本机 stub，再让 `systemd-resolved` 通过 DNS-over-TLS 转发到 Cloudflare 和 Google：

- Cloudflare: `1.1.1.1#cloudflare-dns.com`
- Cloudflare: `1.0.0.1#cloudflare-dns.com`
- Google: `8.8.8.8#dns.google`
- Google: `8.8.4.4#dns.google`

如果检测到 VPS 有 IPv6 默认路由，默认也会加入 Cloudflare/Google 的 IPv6 DoT 上游。可以用 `DOT_ENABLE_IPV6=0` 关闭。

## DoT 和 DoH 的区别

- 这个目录是 DoT 版：只依赖 `systemd-resolved`，走 TCP 853。
- `debian-doh-dns/` 是 DoH 版：需要 `dnsproxy`，走 HTTPS 443。

Debian 12 的 `systemd-resolved` 支持 DoT，但不支持 DoH。所以“只用 VPS 自带环境”时，合理实现是 DoT，不是 DoH。

## 会写入的位置

安装脚本只写这些位置：

- `/etc/systemd/resolved.conf.d/zz-dot-dns.conf`
- `/etc/resolv.conf`
- `/var/backups/dot-dns/<UTC时间戳>.<随机后缀>/...` 里的 `/etc/resolv.conf` 和旧 drop-in 回滚备份

如果检测到前面 DoH 方案安装的 `doh-dns.service`，默认会停止并禁用它，让当前系统 DNS 只依赖 VPS 自带环境。这个行为可以用 `DOT_DISABLE_DOH_DNS=0` 关闭。

脚本不会修改 NetworkManager、cloud-init、resolvconf、netplan 或网卡配置。

## 安装

在仓库目录执行：

```bash
sudo bash debian-dot-dns/install-dot-dns.sh install
```

脚本会执行这些动作：

- 检查 `systemd`、`systemd-resolved` 和 `resolvectl` 是否存在。
- 备份当前 `/etc/resolv.conf`。
- 写入 `systemd-resolved` drop-in，配置 Cloudflare/Google DoT 上游。
- 重启 `systemd-resolved`。
- 把 `/etc/resolv.conf` 指向 `/run/systemd/resolve/stub-resolv.conf`。
- 执行验证。
- 验证通过后，默认停止并禁用旧的 `doh-dns.service`，但保留它的启用/运行状态记录，方便卸载本方案时恢复。

如果验证失败，脚本会删除刚写入的 DoT drop-in，并恢复安装前的 `/etc/resolv.conf`。旧的 `doh-dns.service` 会在验证通过后才停止。

如果安装过程中在验证前提前失败，例如 `systemd-resolved` 重启失败或 resolved stub 文件没有生成，脚本也会执行同一套兜底回滚，避免系统停在半配置状态。重复安装时，如果旧的 `zz-dot-dns.conf` 已存在，失败回滚会优先恢复旧 drop-in。

## 验证

```bash
sudo bash debian-dot-dns/install-dot-dns.sh verify
sudo bash debian-dot-dns/install-dot-dns.sh status
resolvectl query example.com
getent hosts example.com
```

正常结果：

- `systemd-resolved.service` 是 active。
- `/etc/systemd/resolved.conf.d/zz-dot-dns.conf` 存在且由脚本管理。
- `/etc/resolv.conf` 指向 `systemd-resolved` stub。
- `resolvectl status` 里能看到 `+DNSOverTLS`。
- `resolvectl dns` 里能看到 Cloudflare/Google 的 `#server-name` 上游。
- `resolvectl query` 和 `getent hosts` 都能解析域名。

## 卸载/回滚

```bash
sudo bash debian-dot-dns/install-dot-dns.sh uninstall
```

卸载会：

- 删除脚本写入的 `zz-dot-dns.conf`。
- 重启 `systemd-resolved`。
- 从 `/var/backups/dot-dns/latest` 恢复安装前的 `/etc/resolv.conf`。
- 如果安装时停用了 `doh-dns.service`，并且备份里记录它原本是启用或运行状态，会尽量恢复。

`/var/backups/dot-dns/latest` 会指向最近一次保存了原始 `/etc/resolv.conf` 的备份目录，重复安装时不会覆盖最初的回滚点。

`doh-dns.service` 的启用/运行状态会从最近一次包含状态记录的备份目录恢复，适配重复安装或后续切换配置的场景。

## 可选参数

```bash
sudo DOT_ENABLE_IPV6=0 bash debian-dot-dns/install-dot-dns.sh install
sudo DOT_TLS_MODE=opportunistic bash debian-dot-dns/install-dot-dns.sh install
sudo DOT_DNSSEC=allow-downgrade bash debian-dot-dns/install-dot-dns.sh install
sudo DOT_DISABLE_DOH_DNS=0 bash debian-dot-dns/install-dot-dns.sh install
sudo DOT_USE_STUB_SYMLINK=0 bash debian-dot-dns/install-dot-dns.sh install
```

常用变量：

- `DOT_TLS_MODE`: `yes` 或 `opportunistic`，默认 `yes`。`yes` 会要求 DoT 可用。
- `DOT_DNSSEC`: `no`、`allow-downgrade` 或 `yes`，默认 `no`。
- `DOT_ENABLE_IPV6`: `auto`、`1` 或 `0`，默认有 IPv6 默认路由时加入 IPv6 上游。
- `DOT_DISABLE_DOH_DNS`: `1` 或 `0`，默认停止并禁用旧的 `doh-dns.service`。
- `DOT_USE_STUB_SYMLINK`: `1` 或 `0`，默认把 `/etc/resolv.conf` 做成指向 resolved stub 的 symlink；设为 `0` 时写静态 `nameserver 127.0.0.53`。
- `DOT_VERIFY_NAME`: 验证时查询的域名，默认 `example.com`。

## 常见问题

如果安装后解析失败，先看：

```bash
sudo systemctl status --no-pager -l systemd-resolved
resolvectl status
resolvectl dns
```

如果 VPS 出站 TCP 853 被防火墙或运营商限制，严格 DoT 模式会失败。可以临时改成：

```bash
sudo DOT_TLS_MODE=opportunistic bash debian-dot-dns/install-dot-dns.sh install
```

如果你必须走 443 端口，继续使用 `debian-doh-dns/` 的 DoH 方案。
