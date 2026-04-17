# GitHub Source Raw Pool Template

[![Build GitHub Source Pool](https://github.com/omaler886/mihomo_ossp_proxies/actions/workflows/build-github-source-pool.yml/badge.svg?branch=main)](https://github.com/omaler886/mihomo_ossp_proxies/actions/workflows/build-github-source-pool.yml)
![No Secrets Required](https://img.shields.io/badge/secrets-none-brightgreen)
![Discovery](https://img.shields.io/badge/discovery-fixed%20sources%20%2B%20search%20API%20%2B%20trees%20API-blue)
![Output](https://img.shields.io/badge/output-github--source--raw.yaml-orange)

这是一个 **纯 GitHub 可移植模板仓库**，用于：

- 从一组 GitHub raw 源抓取可给 Mihomo/Clash 使用的节点
- 为每个节点附加来源标签
  - `source_repo`
  - `source_url`
- 由 GitHub Actions 定时生成并提交原始池产物

它的定位是：

> **生产 GitHub 来源的 raw 节点池**

而不是在仓库里做本地测活、地区过滤或 VPS 应用。

---

## Quick Start

1. 复制这个仓库到你自己的 GitHub 仓库
2. 修改固定源列表：
   - `config/github_source_urls.txt`
3. 按需修改自动发现配置：
   - `config/github_seed_repos.txt`
   - `config/discovery_patterns.yaml`
   - `config/search_queries.yaml`
4. 在仓库的 Actions 页面启用 workflow
5. 手动运行：
   - `build-github-source-pool`
6. 构建完成后直接读取：
   - `published/pools/github-source-raw.yaml`

---

## Output Links

- Workflow page: [build-github-source-pool](https://github.com/omaler886/mihomo_ossp_proxies/actions/workflows/build-github-source-pool.yml)
- Actions runs: [workflow runs](https://github.com/omaler886/mihomo_ossp_proxies/actions/workflows/build-github-source-pool.yml)
- Raw pool: [github-source-raw.yaml](https://raw.githubusercontent.com/omaler886/mihomo_ossp_proxies/main/published/pools/github-source-raw.yaml)
- Source metadata: [github-source-raw.meta.yaml](https://raw.githubusercontent.com/omaler886/mihomo_ossp_proxies/main/published/manifests/github-source-raw.meta.yaml)
- Build summary: [build-summary.json](https://raw.githubusercontent.com/omaler886/mihomo_ossp_proxies/main/published/manifests/build-summary.json)
- Search summary: [search-summary.json](https://raw.githubusercontent.com/omaler886/mihomo_ossp_proxies/main/published/manifests/search-summary.json)
- Trees discovery summary: [discovery-summary.json](https://raw.githubusercontent.com/omaler886/mihomo_ossp_proxies/main/published/manifests/discovery-summary.json)
- Effective source list: [effective-source-urls.txt](https://raw.githubusercontent.com/omaler886/mihomo_ossp_proxies/main/published/manifests/effective-source-urls.txt)
- Search discovered source list: [search-discovered-source-urls.txt](https://raw.githubusercontent.com/omaler886/mihomo_ossp_proxies/main/published/manifests/search-discovered-source-urls.txt)
- Trees discovered source list: [discovered-source-urls.txt](https://raw.githubusercontent.com/omaler886/mihomo_ossp_proxies/main/published/manifests/discovered-source-urls.txt)

---

## 你会得到什么

GitHub Actions 每次运行后会更新：

- `published/pools/github-source-raw.yaml`
- `published/manifests/github-source-raw.meta.yaml`
- `published/manifests/build-summary.json`

其中：

- `github-source-raw.yaml`
  - 是最终 raw 池
- `github-source-raw.meta.yaml`
  - 是来源统计和抓取元数据
- `build-summary.json`
  - 是本次构建摘要

---

## 仓库结构

### Workflow

- `.github/workflows/build-github-source-pool.yml`

### 配置

- `config/github_source_urls.txt`
- `config/github_seed_repos.txt`
- `config/discovery_patterns.yaml`
- `config/search_queries.yaml`

### 核心脚本

- `scripts/build_github_source_pool.py`
- `scripts/search_github_candidates.py`
- `scripts/discover_github_sources.py`
- `scripts/github_source_pool_utils.py`
- `scripts/mihomo_pool_utils.py`

### 产物目录

- `published/pools/`
- `published/manifests/`

### 文档

- `docs/COPY_TO_ANOTHER_REPO.md`

---

## 工作流逻辑

1. 读取固定源列表：`config/github_source_urls.txt`
2. 读取 Search API / code search 查询：`config/search_queries.yaml`
3. 使用 GitHub Search API / code search 自动发现：
   - 更多候选仓库
   - 更多候选文件
4. 读取种子仓库列表：`config/github_seed_repos.txt`
5. 合并“固定种子仓库 + Search API 发现的仓库”
6. 使用 GitHub Trees API 递归扫描这些仓库中的候选文件
7. 根据 `config/discovery_patterns.yaml` 过滤候选文件
8. 合并：
   - 固定源
   - Search API 直接发现的候选源
   - Trees API 发现的候选源
9. 自动解析：
   - Clash/Mihomo YAML
   - Base64 URI 订阅
10. 生成统一 raw 池
11. 给每个节点打上：
   - `source_repo`
   - `source_url`
12. 将产物提交回仓库

如果某个源临时失败：

- 不会导致整个工作流直接中止
- 失败信息会写入 `published/manifests/github-source-raw.meta.yaml`
- 自动发现阶段的失败信息会写入 `published/manifests/discovery-summary.json`
- Search API / code search 的结果会写入 `published/manifests/search-summary.json`

---

## 如何配置

只需要修改一个文件：

- `config/github_source_urls.txt`

如果你想打开“自己搜”能力，还要改：

- `config/github_seed_repos.txt`
- `config/discovery_patterns.yaml`
- `config/search_queries.yaml`

每行一个 GitHub raw 源。

支持：

- Mihomo/Clash YAML
- Base64 订阅

不需要任何 GitHub Secret。

GitHub Actions 会自动使用内置的 `github.token` 去调用 GitHub API 做：

- Search API / code search
- Trees API 仓库扫描

---

## 如何复制到别的 repo

看这里：

- [`docs/COPY_TO_ANOTHER_REPO.md`](docs/COPY_TO_ANOTHER_REPO.md)

---

## 适合放在什么位置

这类模板适合放在：

- 专门的公开 raw 池仓库
- 上游抓取/聚合仓库
- VPS 消费端的上游仓库

---

## VPS 侧推荐分工

这个仓库只负责：

- **抓 GitHub 源**
- **自动发现额外 GitHub 候选源**
- **产出 raw 池**

VPS 建议只负责：

1. 拉 `published/pools/github-source-raw.yaml`
2. 本地做 live 测试
3. 剔除地区/关键词
4. 应用到 Mihomo
