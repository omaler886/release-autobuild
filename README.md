# GitHub Actions 版 Mihomo 原始节点池

这个目录可直接作为一个 GitHub 仓库使用。

## 目标

- **GitHub Actions 每日抓取 GitHub raw 池**
  - `github-crawled-raw.yaml`
- **VPS 本地只负责**
  - live 测活
  - 剔除香港
  - 最终应用到 Mihomo

## 目录

- Workflow: `.github/workflows/mihomo-raw-pools.yml`
- B 池源列表: `config/subscription_b_url.txt`
- GHA 构建脚本: `scripts/gha_build_raw_pools.py`
- 产物输出目录:
  - `published/pools/*.yaml`
  - `published/manifests/*.yaml|json`

## 必须配置的 GitHub Secret

无。

## 产物说明

Actions 每次会提交更新后的：

- `published/pools/github-crawled-raw.yaml`

以及：

- `published/manifests/github-crawled-meta.yaml`
- `published/manifests/summary.json`

## VPS 侧建议

VPS 不再负责抓源，只做：

1. 拉取 `github-crawled-raw.yaml`
2. 本地测活
3. 剔除香港
4. 应用到 Mihomo
