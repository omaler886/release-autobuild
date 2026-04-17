#!/usr/bin/env python3
import base64
from copy import deepcopy
from pathlib import Path
import re
import urllib.parse
import urllib.request

import yaml

USER_AGENT = 'Mozilla/5.0 (GitHub Actions Mihomo Pool Template)'


def build_opener(proxy_url: str | None = None):
    handlers = []
    if proxy_url:
        handlers.append(urllib.request.ProxyHandler({'http': proxy_url, 'https': proxy_url}))
    opener = urllib.request.build_opener(*handlers)
    opener.addheaders = [('User-Agent', USER_AGENT)]
    return opener


def fetch_text(url: str, opener) -> str:
    with opener.open(url, timeout=60) as resp:
        return resp.read().decode('utf-8', errors='replace')


def read_urls_from_file(path: Path) -> list[str]:
    urls = []
    for raw_line in path.read_text(encoding='utf-8').splitlines():
        line = raw_line.strip()
        if not line or line.startswith('#'):
            continue
        urls.append(line)
    return urls


def ensure_unique_proxy_names(proxies: list[dict]) -> list[dict]:
    out = []
    seen = {}
    for item in proxies:
        proxy = deepcopy(item)
        base_name = str(proxy.get('name') or proxy.get('server') or 'node').strip() or 'node'
        count = seen.get(base_name, 0) + 1
        seen[base_name] = count
        proxy['name'] = base_name if count == 1 else f"{base_name} {count}"
        out.append(proxy)
    return out


def _normalize_supported_types(supported_types):
    if not supported_types:
        return None
    return {str(item).strip().lower() for item in supported_types if str(item).strip()}


def _filter_supported_yaml_proxies(proxies: list[dict], supported_types):
    normalized = _normalize_supported_types(supported_types)
    if not normalized:
        return proxies
    out = []
    for proxy in proxies:
        ptype = str(proxy.get('type') or '').strip().lower()
        if ptype and ptype in normalized:
            out.append(proxy)
    return out


def load_subscription_proxies(text: str, supported_types=None):
    errors = []
    try:
        data = yaml.safe_load(text)
        if isinstance(data, dict) and isinstance(data.get('proxies'), list):
            proxies = [p for p in data['proxies'] if isinstance(p, dict) and p.get('name')]
            proxies = _filter_supported_yaml_proxies(proxies, supported_types)
            if proxies:
                return proxies, 'clash-yaml'
            errors.append('yaml proxies list empty')
        else:
            errors.append(f'yaml root type unsupported: {type(data).__name__}')
    except Exception as exc:
        errors.append(f'yaml parse failed: {exc}')

    raw = ''.join(text.strip().split())
    decoded_text = None
    for decoder in (base64.b64decode, base64.urlsafe_b64decode):
        try:
            pad = '=' * ((4 - len(raw) % 4) % 4)
            decoded = decoder(raw + pad)
            decoded_text = decoded.decode('utf-8', errors='replace')
            if '://' in decoded_text:
                break
        except Exception as exc:
            errors.append(f'base64 decode failed: {exc}')
            continue
    if not decoded_text or '://' not in decoded_text:
        raise ValueError('Unable to parse subscription as Clash YAML or Base64 URI list: ' + '; '.join(errors))

    proxies = []
    name_counts = {}
    for line in decoded_text.splitlines():
        line = line.strip()
        if not line:
            continue
        proxy = parse_uri_line(line)
        if not proxy:
            continue
        base_name = proxy.get('name') or proxy.get('server') or 'node'
        count = name_counts.get(base_name, 0) + 1
        name_counts[base_name] = count
        if count > 1:
            proxy['name'] = f"{base_name} {count}"
        proxies.append(proxy)
    if not proxies:
        inline_proxies = extract_embedded_uri_proxies(text)
        if inline_proxies:
            return inline_proxies, 'embedded-uri'
        raise ValueError('No proxies parsed from Base64 URI list')
    return proxies, 'base64-uri'


def parse_uri_line(line: str):
    line = line.strip()
    if not line:
        return None
    if line.startswith('vless://'):
        return parse_vless_uri(line)
    if line.startswith('vmess://'):
        return parse_vmess_uri(line)
    if line.startswith('trojan://'):
        return parse_trojan_uri(line)
    if line.startswith('ss://'):
        return parse_ss_uri(line)
    raise ValueError(f'Unsupported URI scheme in line: {line[:32]}...')


def extract_embedded_uri_proxies(text: str):
    uri_pattern = re.compile(r'(?:vmess|vless|trojan|ss)://[^\s<>\")]+')
    proxies = []
    name_counts = {}
    for match in uri_pattern.finditer(text):
        raw = match.group(0).strip().rstrip('.,;')
        try:
            proxy = parse_uri_line(raw)
        except Exception:
            continue
        base_name = proxy.get('name') or proxy.get('server') or 'node'
        count = name_counts.get(base_name, 0) + 1
        name_counts[base_name] = count
        if count > 1:
            proxy['name'] = f"{base_name} {count}"
        proxies.append(proxy)
    return proxies


def _common_tls_fields(proxy: dict, query: dict):
    security = (query.get('security', [''])[0] or '').lower()
    if security == 'tls':
        proxy['tls'] = True

    for insecure_key in ('allowInsecure', 'insecure'):
        if insecure_key in query:
            raw = (query.get(insecure_key, ['0'])[0] or '0').strip().lower()
            proxy['skip-cert-verify'] = raw in {'1', 'true', 'yes', 'on'}
            break

    sni = (query.get('sni', [''])[0] or query.get('servername', [''])[0] or '').strip()
    if sni:
        proxy['servername'] = sni

    fp = (query.get('fp', [''])[0] or query.get('client-fingerprint', [''])[0] or '').strip()
    if fp:
        proxy['client-fingerprint'] = fp

    alpn = (query.get('alpn', [''])[0] or '').strip()
    if alpn:
        proxy['alpn'] = [item.strip() for item in alpn.split(',') if item.strip()]


def _apply_ws_from_query(proxy: dict, query: dict):
    network = (query.get('type', [''])[0] or '').strip().lower()
    if network == 'ws':
        proxy['network'] = 'ws'
        ws_opts = {}
        path = (query.get('path', [''])[0] or '').strip()
        if path:
            ws_opts['path'] = urllib.parse.unquote(path)
        host = (query.get('host', [''])[0] or '').strip()
        if host:
            ws_opts['headers'] = {'Host': host}
        if ws_opts:
            proxy['ws-opts'] = ws_opts


def parse_vless_uri(line: str):
    parsed = urllib.parse.urlsplit(line)
    query = urllib.parse.parse_qs(parsed.query, keep_blank_values=True)
    name = urllib.parse.unquote(parsed.fragment or '').strip() or f"{parsed.hostname}:{parsed.port}"
    proxy = {
        'name': name,
        'type': 'vless',
        'server': parsed.hostname,
        'port': int(parsed.port),
        'uuid': urllib.parse.unquote(parsed.username or ''),
        'udp': True,
    }
    _common_tls_fields(proxy, query)
    flow = (query.get('flow', [''])[0] or '').strip()
    if flow:
        proxy['flow'] = flow
    _apply_ws_from_query(proxy, query)
    if (query.get('ech', [''])[0] or '').strip():
        proxy['ech-opts'] = {'enable': True}
    return proxy


def parse_trojan_uri(line: str):
    parsed = urllib.parse.urlsplit(line)
    query = urllib.parse.parse_qs(parsed.query, keep_blank_values=True)
    name = urllib.parse.unquote(parsed.fragment or '').strip() or f"{parsed.hostname}:{parsed.port}"
    proxy = {
        'name': name,
        'type': 'trojan',
        'server': parsed.hostname,
        'port': int(parsed.port),
        'password': urllib.parse.unquote(parsed.username or ''),
        'udp': True,
    }
    _common_tls_fields(proxy, query)
    _apply_ws_from_query(proxy, query)
    return proxy


def parse_vmess_uri(line: str):
    payload = line[len('vmess://'):].strip()
    pad = '=' * ((4 - len(payload) % 4) % 4)
    data = yaml.safe_load(base64.b64decode(payload + pad).decode('utf-8', errors='replace'))
    name = str(data.get('ps') or data.get('remark') or f"{data.get('add')}:{data.get('port')}").strip()
    proxy = {
        'name': name,
        'type': 'vmess',
        'server': data.get('add'),
        'port': int(data.get('port')),
        'uuid': data.get('id'),
        'alterId': int(data.get('aid') or 0),
        'cipher': 'auto',
        'udp': True,
    }
    if str(data.get('tls') or '').lower() in {'tls', 'true', '1'}:
        proxy['tls'] = True
    sni = str(data.get('sni') or data.get('host') or '').strip()
    if sni:
        proxy['servername'] = sni
    fp = str(data.get('fp') or '').strip()
    if fp:
        proxy['client-fingerprint'] = fp
    alpn = str(data.get('alpn') or '').strip()
    if alpn:
        proxy['alpn'] = [item.strip() for item in alpn.split(',') if item.strip()]
    net = str(data.get('net') or data.get('type') or '').strip().lower()
    if net == 'ws':
        proxy['network'] = 'ws'
        ws_opts = {}
        path = str(data.get('path') or '').strip()
        if path:
            ws_opts['path'] = path
        host = str(data.get('host') or '').strip()
        if host:
            ws_opts['headers'] = {'Host': host}
        if ws_opts:
            proxy['ws-opts'] = ws_opts
    return proxy


def parse_ss_uri(line: str):
    body = line[len('ss://'):]
    if '#' in body:
        body, frag = body.split('#', 1)
        name = urllib.parse.unquote(frag).strip()
    else:
        name = ''
    if '@' not in body:
        pad = '=' * ((4 - len(body) % 4) % 4)
        body = base64.b64decode(body + pad).decode('utf-8', errors='replace')
    if '@' not in body:
        raise ValueError('Unsupported ss uri format')
    creds, hostpart = body.rsplit('@', 1)
    if ':' not in creds or ':' not in hostpart:
        raise ValueError('Unsupported ss uri format')
    cipher, password = creds.split(':', 1)
    server, port = hostpart.rsplit(':', 1)
    return {
        'name': name or f"{server}:{port}",
        'type': 'ss',
        'server': server.strip(),
        'port': int(port),
        'cipher': cipher.strip(),
        'password': password.strip(),
        'udp': True,
    }


def write_yaml(path: Path, data: dict):
    def _quote_short_id(match):
        raw = match.group(2).strip().strip("'\"")
        return f"{match.group(1)}'{raw}'"

    text = yaml.safe_dump(
        data,
        allow_unicode=True,
        sort_keys=False,
        width=10**6,
        default_flow_style=False,
    )
    text = re.sub(r"^(\s+short-id:\s+)(.+)$", _quote_short_id, text, flags=re.MULTILINE)
    path.write_text(text, encoding='utf-8')
