#!/usr/bin/env python3
import argparse
import json
from pathlib import Path

from github_source_pool_utils import build_raw_pool, print_json
from mihomo_pool_utils import read_urls_from_file


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--source-url-file', required=True, help='Path to the GitHub source URL list file')
    parser.add_argument('--extra-source-url-file', action='append', default=[], help='Additional discovered source URL list files')
    parser.add_argument('--output-dir', required=True, help='Base output directory for published artifacts')
    args = parser.parse_args()

    output_dir = Path(args.output_dir)
    pools_dir = output_dir / 'pools'
    manifests_dir = output_dir / 'manifests'
    pools_dir.mkdir(parents=True, exist_ok=True)
    manifests_dir.mkdir(parents=True, exist_ok=True)

    source_file = Path(args.source_url_file)
    base_urls = read_urls_from_file(source_file)
    extra_urls = []
    extra_files = [Path(item) for item in args.extra_source_url_file]
    for path in extra_files:
        if path.exists():
            extra_urls.extend(read_urls_from_file(path))

    merged_urls = []
    seen = set()
    for url in [*base_urls, *extra_urls]:
        if url in seen:
            continue
        seen.add(url)
        merged_urls.append(url)

    merged_source_file = manifests_dir / 'effective-source-urls.txt'
    merged_source_file.write_text('\n'.join(merged_urls) + ('\n' if merged_urls else ''), encoding='utf-8')

    result = build_raw_pool(
        url_file=merged_source_file,
        output_pool=pools_dir / 'github-source-raw.yaml',
        output_meta=manifests_dir / 'github-source-raw.meta.yaml',
    )
    summary = {
        'template_name': 'github-source-raw-pool-template',
        'base_source_file': source_file.as_posix(),
        'extra_source_files': [path.as_posix() for path in extra_files if path.exists()],
        'effective_source_file': merged_source_file.as_posix(),
        **result,
    }
    (manifests_dir / 'build-summary.json').write_text(
        json.dumps(summary, ensure_ascii=False, indent=2),
        encoding='utf-8',
    )
    print_json(summary)


if __name__ == '__main__':
    main()
