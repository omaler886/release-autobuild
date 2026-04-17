#!/usr/bin/env python3
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import urllib.parse

from refresh_mihomo_subscription import (
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


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--url-file', required=True)
    parser.add_argument('--pool-path', required=True)
    parser.add_argument('--meta-path', required=True)
    parser.add_argument('--proxy-url')
    args = parser.parse_args()

    url_file = Path(args.url_file)
    pool_path = Path(args.pool_path)
    meta_path = Path(args.meta_path)
    pool_path.parent.mkdir(parents=True, exist_ok=True)
    meta_path.parent.mkdir(parents=True, exist_ok=True)

    urls = read_urls_from_file(url_file)
    if not urls:
        raise ValueError(f'No URLs found in {url_file}')

    opener = build_opener(args.proxy_url)
    all_proxies = []
    source_details = []
    for current_url in urls:
        try:
            raw_text = fetch_text(current_url, opener)
            proxies, source_type = load_subscription_proxies(raw_text)
            source_repo = derive_source_repo(current_url)
            tagged_proxies = []
            for proxy in proxies:
                item = dict(proxy)
                item['source_repo'] = source_repo
                item['source_url'] = current_url
                tagged_proxies.append(item)
            all_proxies.extend(tagged_proxies)
            source_details.append({
                'url': current_url,
                'source_repo': source_repo,
                'source_type': source_type,
                'proxy_count': len(proxies),
            })
        except Exception as exc:
            source_details.append({
                'url': current_url,
                'source_repo': derive_source_repo(current_url),
                'source_type': 'error',
                'proxy_count': 0,
                'error': str(exc),
            })

    if not all_proxies:
        raise ValueError('No proxies fetched successfully from GitHub sources')

    all_proxies = ensure_unique_proxy_names(all_proxies)
    write_yaml(pool_path, {'proxies': all_proxies})

    meta = {
        'updated_at': datetime.now(timezone.utc).isoformat(),
        'proxy_count': len(all_proxies),
        'pool_path': str(pool_path),
        'source_details': source_details,
        'subscription_urls': urls,
    }
    if args.proxy_url:
        meta['fetched_via_proxy'] = args.proxy_url
    write_yaml(meta_path, meta)
    print(json.dumps(meta, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
