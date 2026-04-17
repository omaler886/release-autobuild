#!/usr/bin/env python3
import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import re
import urllib.parse
import urllib.request

import yaml

GITHUB_API = 'https://api.github.com'


def build_api_opener():
    opener = urllib.request.build_opener()
    headers = [
        ('User-Agent', 'Mozilla/5.0 (GitHub Search Discovery Template)'),
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


def load_search_config(path: Path):
    data = yaml.safe_load(path.read_text(encoding='utf-8')) or {}
    return {
        'repository_search_queries': [str(x).strip() for x in data.get('repository_search_queries', []) if str(x).strip()],
        'code_search_queries': [str(x).strip() for x in data.get('code_search_queries', []) if str(x).strip()],
        'max_repository_results_per_query': int(data.get('max_repository_results_per_query', 10)),
        'max_code_results_per_query': int(data.get('max_code_results_per_query', 10)),
        'exclude_repo_regexes': [re.compile(str(x), re.IGNORECASE) for x in data.get('exclude_repo_regexes', []) if str(x).strip()],
    }


def repo_allowed(full_name: str, patterns):
    return not any(regex.search(full_name) for regex in patterns['exclude_repo_regexes'])


def search_repositories(opener, query: str, per_page: int):
    url = f'{GITHUB_API}/search/repositories?q={urllib.parse.quote(query)}&sort=updated&order=desc&per_page={per_page}'
    return fetch_json(url, opener)


def search_code(opener, query: str, per_page: int):
    url = f'{GITHUB_API}/search/code?q={urllib.parse.quote(query)}&per_page={per_page}'
    return fetch_json(url, opener)


def raw_url(owner: str, repo: str, ref: str, path: str):
    quoted_path = '/'.join(urllib.parse.quote(part, safe='') for part in path.split('/'))
    return f'https://raw.githubusercontent.com/{owner}/{repo}/{ref}/{quoted_path}'


def discover_with_search(patterns):
    opener = build_api_opener()
    repo_candidates = []
    repo_meta = []
    for query in patterns['repository_search_queries']:
        try:
            payload = search_repositories(opener, query, patterns['max_repository_results_per_query'])
            accepted = []
            for item in payload.get('items', []):
                full_name = str(item.get('full_name') or '')
                if not full_name or not repo_allowed(full_name, patterns):
                    continue
                accepted.append({
                    'full_name': full_name,
                    'default_branch': item.get('default_branch') or '',
                    'html_url': item.get('html_url') or '',
                    'stargazers_count': int(item.get('stargazers_count') or 0),
                })
            repo_meta.append({
                'query': query,
                'total_count': int(payload.get('total_count') or 0),
                'accepted_count': len(accepted),
                'accepted_repos': accepted,
            })
            repo_candidates.extend(accepted)
        except Exception as exc:
            repo_meta.append({
                'query': query,
                'total_count': 0,
                'accepted_count': 0,
                'accepted_repos': [],
                'error': str(exc),
            })

    code_candidates = []
    code_meta = []
    for query in patterns['code_search_queries']:
        try:
            payload = search_code(opener, query, patterns['max_code_results_per_query'])
            accepted = []
            for item in payload.get('items', []):
                repository = item.get('repository') or {}
                full_name = str(repository.get('full_name') or '')
                if not full_name or not repo_allowed(full_name, patterns):
                    continue
                if '/' not in full_name:
                    continue
                owner, repo = full_name.split('/', 1)
                ref = str(repository.get('default_branch') or 'main')
                path = str(item.get('path') or '')
                accepted.append({
                    'full_name': full_name,
                    'path': path,
                    'raw_url': raw_url(owner, repo, ref, path),
                    'html_url': item.get('html_url') or '',
                })
            code_meta.append({
                'query': query,
                'total_count': int(payload.get('total_count') or 0),
                'accepted_count': len(accepted),
                'accepted_sources': accepted,
            })
            code_candidates.extend(accepted)
        except Exception as exc:
            code_meta.append({
                'query': query,
                'total_count': 0,
                'accepted_count': 0,
                'accepted_sources': [],
                'error': str(exc),
            })

    deduped_repos = []
    seen_repos = set()
    for item in repo_candidates:
        full_name = item['full_name']
        if full_name in seen_repos:
            continue
        seen_repos.add(full_name)
        ref = item.get('default_branch') or ''
        deduped_repos.append(f'{full_name}@{ref}' if ref else full_name)

    deduped_sources = []
    seen_sources = set()
    for item in code_candidates:
        url = item['raw_url']
        if url in seen_sources:
            continue
        seen_sources.add(url)
        deduped_sources.append(url)

    return deduped_repos, deduped_sources, {'repository_search': repo_meta, 'code_search': code_meta}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--queries-file', required=True)
    parser.add_argument('--output-repo-file', required=True)
    parser.add_argument('--output-source-file', required=True)
    parser.add_argument('--meta-file', required=True)
    args = parser.parse_args()

    patterns = load_search_config(Path(args.queries_file))
    repos, sources, meta_sections = discover_with_search(patterns)

    output_repo_file = Path(args.output_repo_file)
    output_source_file = Path(args.output_source_file)
    meta_file = Path(args.meta_file)
    output_repo_file.parent.mkdir(parents=True, exist_ok=True)
    output_source_file.parent.mkdir(parents=True, exist_ok=True)
    meta_file.parent.mkdir(parents=True, exist_ok=True)

    output_repo_file.write_text('\n'.join(repos) + ('\n' if repos else ''), encoding='utf-8')
    output_source_file.write_text('\n'.join(sources) + ('\n' if sources else ''), encoding='utf-8')

    meta = {
        'updated_at': datetime.now(timezone.utc).isoformat(),
        'queries_file': Path(args.queries_file).as_posix(),
        'discovered_repo_count': len(repos),
        'discovered_source_count': len(sources),
        'output_repo_file': output_repo_file.as_posix(),
        'output_source_file': output_source_file.as_posix(),
        **meta_sections,
    }
    meta_file.write_text(json.dumps(meta, ensure_ascii=False, indent=2), encoding='utf-8')
    print(json.dumps(meta, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
