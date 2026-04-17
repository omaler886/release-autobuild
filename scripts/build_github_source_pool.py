#!/usr/bin/env python3
import argparse
import json
from pathlib import Path

from github_source_pool_utils import build_raw_pool, print_json


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--source-url-file', required=True, help='Path to the GitHub source URL list file')
    parser.add_argument('--output-dir', required=True, help='Base output directory for published artifacts')
    args = parser.parse_args()

    output_dir = Path(args.output_dir)
    pools_dir = output_dir / 'pools'
    manifests_dir = output_dir / 'manifests'
    pools_dir.mkdir(parents=True, exist_ok=True)
    manifests_dir.mkdir(parents=True, exist_ok=True)

    result = build_raw_pool(
        url_file=Path(args.source_url_file),
        output_pool=pools_dir / 'github-source-raw.yaml',
        output_meta=manifests_dir / 'github-source-raw.meta.yaml',
    )
    summary = {
        'template_name': 'github-source-raw-pool-template',
        **result,
    }
    (manifests_dir / 'build-summary.json').write_text(
        json.dumps(summary, ensure_ascii=False, indent=2),
        encoding='utf-8',
    )
    print_json(summary)


if __name__ == '__main__':
    main()
