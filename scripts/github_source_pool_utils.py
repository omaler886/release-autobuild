#!/usr/bin/env python3
from datetime import datetime, timezone
from pathlib import Path
import json
import urllib.parse

from mihomo_pool_utils import (
    build_opener,
    ensure_unique_proxy_names,
    fetch_text,
    load_subscription_proxies,
    read_urls_from_file,
    write_yaml,
)


def derive_source_repo(url: str) -> str:
    parsed = urllib.parse.urlsplit(url)
    parts = [item for item in parsed.path.split('/') if item]
    if parsed.netloc == 'raw.githubusercontent.com' and len(parts) >= 2:
        return f'{parts[0]}/{parts[1]}'
    if parsed.netloc == 'github.com' and len(parts) >= 2:
        return f'{parts[0]}/{parts[1]}'
    return parsed.netloc


def crawl_source_urls(urls: list[str], opener):
    proxies = []
    source_details = []
    for current_url in urls:
        source_repo = derive_source_repo(current_url)
        try:
            raw_text = fetch_text(current_url, opener)
            parsed, source_type = load_subscription_proxies(raw_text)
            tagged = []
            for proxy in parsed:
                item = dict(proxy)
                item['source_repo'] = source_repo
                item['source_url'] = current_url
                tagged.append(item)
            proxies.extend(tagged)
            source_details.append({
                'url': current_url,
                'source_repo': source_repo,
                'source_type': source_type,
                'proxy_count': len(parsed),
            })
        except Exception as exc:
            source_details.append({
                'url': current_url,
                'source_repo': source_repo,
                'source_type': 'error',
                'proxy_count': 0,
                'error': str(exc),
            })
    return ensure_unique_proxy_names(proxies), source_details


def build_raw_pool(url_file: Path, output_pool: Path, output_meta: Path):
    urls = read_urls_from_file(url_file)
    if not urls:
        raise ValueError(f'No URLs found in {url_file}')

    opener = build_opener()
    proxies, source_details = crawl_source_urls(urls, opener)
    now = datetime.now(timezone.utc).isoformat()

    output_pool.parent.mkdir(parents=True, exist_ok=True)
    output_meta.parent.mkdir(parents=True, exist_ok=True)
    write_yaml(output_pool, {'proxies': proxies})
    meta = {
        'updated_at': now,
        'pool_name': 'github-source-raw',
        'proxy_count': len(proxies),
        'source_list_path': url_file.as_posix(),
        'output_pool_path': output_pool.as_posix(),
        'source_details': source_details,
        'source_urls': urls,
    }
    write_yaml(output_meta, meta)
    return {
        'updated_at': now,
        'proxy_count': len(proxies),
        'source_count': len(urls),
        'output_pool_path': output_pool.as_posix(),
        'output_meta_path': output_meta.as_posix(),
    }


def print_json(data: dict):
    print(json.dumps(data, ensure_ascii=False, indent=2))
