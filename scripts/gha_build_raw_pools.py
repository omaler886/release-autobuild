#!/usr/bin/env python3
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from crawl_github_mihomo_pool import derive_source_repo  # noqa: E402
from refresh_mihomo_subscription import (  # noqa: E402
    ensure_unique_proxy_names,
    fetch_text,
    load_subscription_proxies,
    read_urls_from_file,
    write_yaml,
)


def build_direct_opener():
    import urllib.request

    opener = urllib.request.build_opener()
    opener.addheaders = [('User-Agent', 'Mozilla/5.0 (GitHub Actions Mihomo Raw Pools)')]
    return opener


def crawl_urls(urls: list[str], opener):
    proxies = []
    details = []
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
            details.append({
                'url': current_url,
                'source_repo': source_repo,
                'source_type': source_type,
                'proxy_count': len(parsed),
            })
        except Exception as exc:
            details.append({
                'url': current_url,
                'source_repo': source_repo,
                'source_type': 'error',
                'proxy_count': 0,
                'error': str(exc),
            })
    return ensure_unique_proxy_names(proxies), details


def save_pool(pool_path: Path, meta_path: Path, proxies: list[dict], meta: dict):
    pool_path.parent.mkdir(parents=True, exist_ok=True)
    meta_path.parent.mkdir(parents=True, exist_ok=True)
    write_yaml(pool_path, {'proxies': proxies})
    write_yaml(meta_path, meta)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--subscription-b-url-file', required=True)
    parser.add_argument('--output-dir', required=True)
    args = parser.parse_args()

    out_dir = Path(args.output_dir)
    pool_dir = out_dir
    manifest_dir = out_dir.parent / 'manifests'
    pool_dir.mkdir(parents=True, exist_ok=True)
    manifest_dir.mkdir(parents=True, exist_ok=True)

    opener = build_direct_opener()
    now = datetime.now(timezone.utc).isoformat()

    b_urls = read_urls_from_file(Path(args.subscription_b_url_file))
    b_proxies, b_details = crawl_urls(b_urls, opener)
    b_meta = {
        'updated_at': now,
        'pool_name': 'github-crawled-raw',
        'proxy_count': len(b_proxies),
        'subscription_urls': b_urls,
        'source_details': b_details,
    }
    save_pool(pool_dir / 'github-crawled-raw.yaml', manifest_dir / 'github-crawled-meta.yaml', b_proxies, b_meta)

    summary = {
        'updated_at': now,
        'github_crawled_raw_proxy_count': len(b_proxies),
        'output_dir': str(out_dir),
    }
    (manifest_dir / 'summary.json').write_text(json.dumps(summary, ensure_ascii=False, indent=2), encoding='utf-8')
    print(json.dumps(summary, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
