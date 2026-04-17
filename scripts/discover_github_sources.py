#!/usr/bin/env python3
import argparse
from datetime import datetime, timezone
import fnmatch
import json
import os
from pathlib import Path
import re
import urllib.parse
import urllib.request

import yaml

from github_source_pool_utils import derive_source_repo
from mihomo_pool_utils import build_opener, load_subscription_proxies, read_urls_from_file

GITHUB_API = 'https://api.github.com'


def build_github_api_opener():
    opener = urllib.request.build_opener()
    headers = [
        ('User-Agent', 'Mozilla/5.0 (GitHub Source Discovery Template)'),
        ('Accept', 'application/vnd.github+json'),
        ('X-GitHub-Api-Version', '2022-11-28'),
    ]
    token = os.environ.get('GH_API_TOKEN') or os.environ.get('GITHUB_TOKEN')
    if token:
        headers.append(('Authorization', f'Bearer {token}'))
    opener.addheaders = headers
    return opener


def fetch_json(url: str, opener):
    with opener.open(url, timeout=60) as resp:
        return json.loads(resp.read().decode('utf-8', errors='replace'))


def parse_seed_repo_lines(path: Path):
    items = []
    for raw_line in path.read_text(encoding='utf-8').splitlines():
        line = raw_line.strip()
        if not line or line.startswith('#'):
            continue
        if '@' in line:
            full_name, ref = line.split('@', 1)
        else:
            full_name, ref = line, ''
        if '/' not in full_name:
            continue
        owner, repo = full_name.split('/', 1)
        items.append({'owner': owner.strip(), 'repo': repo.strip(), 'ref': ref.strip()})
    return items


def load_discovery_patterns(path: Path):
    data = yaml.safe_load(path.read_text(encoding='utf-8')) or {}
    return {
        'supported_proxy_types': [str(x).strip() for x in data.get('supported_proxy_types', []) if str(x).strip()],
        'include_globs': [str(x).strip() for x in data.get('include_globs', []) if str(x).strip()],
        'include_path_keywords': [str(x).strip().lower() for x in data.get('include_path_keywords', []) if str(x).strip()],
        'exclude_regexes': [re.compile(str(x)) for x in data.get('exclude_regexes', []) if str(x).strip()],
        'content_indicator_regexes': [re.compile(str(x), re.IGNORECASE | re.MULTILINE) for x in data.get('content_indicator_regexes', []) if str(x).strip()],
        'max_candidate_files_per_repo': int(data.get('max_candidate_files_per_repo', 120)),
    }


def get_default_branch(owner: str, repo: str, opener):
    url = f'{GITHUB_API}/repos/{owner}/{repo}'
    data = fetch_json(url, opener)
    return data.get('default_branch') or 'main'


def get_repo_tree(owner: str, repo: str, ref: str, opener):
    url = f'{GITHUB_API}/repos/{owner}/{repo}/git/trees/{urllib.parse.quote(ref, safe="")}?recursive=1'
    return fetch_json(url, opener)


def path_matches(path: str, patterns: dict):
    lower = path.lower()
    if any(regex.search(path) for regex in patterns['exclude_regexes']):
        return False
    glob_ok = any(fnmatch.fnmatch(lower, glob.lower()) for glob in patterns['include_globs']) if patterns['include_globs'] else True
    if not glob_ok:
        return False
    return True


def raw_url(owner: str, repo: str, ref: str, path: str):
    quoted_path = '/'.join(urllib.parse.quote(part, safe='') for part in path.split('/'))
    return f'https://raw.githubusercontent.com/{owner}/{repo}/{ref}/{quoted_path}'


def content_looks_promising(text: str, patterns: dict, supported_types):
    try:
        proxies, _ = load_subscription_proxies(text, supported_types=supported_types)
        if proxies:
            return True
    except Exception:
        pass
    return False


def discover_sources(seed_repos, patterns):
    api_opener = build_github_api_opener()
    raw_opener = build_opener()
    discovered = []
    discovery_meta = []

    for item in seed_repos:
        owner = item['owner']
        repo = item['repo']
        ref = item['ref'] or get_default_branch(owner, repo, api_opener)
        repo_id = f'{owner}/{repo}'
        tree_payload = get_repo_tree(owner, repo, ref, api_opener)
        entries = tree_payload.get('tree') or []
        truncated = bool(tree_payload.get('truncated'))
        candidate_paths = []
        for entry in entries:
            if entry.get('type') != 'blob':
                continue
            path = str(entry.get('path') or '')
            if path_matches(path, patterns):
                candidate_paths.append(path)
        candidate_paths = candidate_paths[: patterns['max_candidate_files_per_repo']]

        accepted = []
        rejected = []
        for path in candidate_paths:
            url = raw_url(owner, repo, ref, path)
            try:
                with raw_opener.open(url, timeout=30) as resp:
                    text = resp.read().decode('utf-8', errors='replace')
                if content_looks_promising(text, patterns, patterns['supported_proxy_types']):
                    accepted.append(url)
                else:
                    rejected.append({'path': path, 'reason': 'content_not_matching'})
            except Exception as exc:
                rejected.append({'path': path, 'reason': str(exc)})

        discovered.extend(accepted)
        discovery_meta.append({
            'seed_repo': repo_id,
            'ref': ref,
            'tree_truncated': truncated,
            'candidate_file_count': len(candidate_paths),
            'accepted_source_count': len(accepted),
            'accepted_sources': accepted,
            'sample_rejections': rejected[:20],
        })

    deduped = []
    seen = set()
    for url in discovered:
        if url in seen:
            continue
        seen.add(url)
        deduped.append(url)
    return deduped, discovery_meta


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--seed-repo-file', required=True)
    parser.add_argument('--patterns-file', required=True)
    parser.add_argument('--output-file', required=True)
    parser.add_argument('--meta-file', required=True)
    args = parser.parse_args()

    seed_repos = parse_seed_repo_lines(Path(args.seed_repo_file))
    patterns = load_discovery_patterns(Path(args.patterns_file))
    discovered, discovery_meta = discover_sources(seed_repos, patterns)

    output_file = Path(args.output_file)
    meta_file = Path(args.meta_file)
    output_file.parent.mkdir(parents=True, exist_ok=True)
    meta_file.parent.mkdir(parents=True, exist_ok=True)
    output_file.write_text('\n'.join(discovered) + ('\n' if discovered else ''), encoding='utf-8')
    meta = {
        'updated_at': datetime.now(timezone.utc).isoformat(),
        'seed_repo_file': Path(args.seed_repo_file).as_posix(),
        'patterns_file': Path(args.patterns_file).as_posix(),
        'discovered_source_count': len(discovered),
        'discovered_sources_file': output_file.as_posix(),
        'repos': discovery_meta,
    }
    meta_file.write_text(json.dumps(meta, ensure_ascii=False, indent=2), encoding='utf-8')
    print(json.dumps(meta, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
