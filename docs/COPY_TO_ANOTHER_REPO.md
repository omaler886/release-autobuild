# 如何复制到别的仓库

这份模板可以直接复制到任意一个新的 GitHub 仓库里使用。

## 最小复制集

至少复制这些路径：

- `.github/workflows/build-github-source-pool.yml`
- `config/github_source_urls.txt`
- `scripts/build_github_source_pool.py`
- `scripts/github_source_pool_utils.py`
- `scripts/mihomo_pool_utils.py`

如果你还想保留现成的示例产物，也可以一起复制：

- `published/pools/github-source-raw.yaml`
- `published/manifests/github-source-raw.meta.yaml`
- `published/manifests/build-summary.json`

## 复制后的必改项

### 1. 修改源列表

编辑：

- `config/github_source_urls.txt`

把里面的 URL 替换成你自己的 GitHub raw 源。

### 2. 修改定时任务

编辑：

- `.github/workflows/build-github-source-pool.yml`

修改 `cron`：

```yaml
schedule:
  - cron: '17 2 * * *'
```

### 3. 修改产物提交说明（可选）

同一个 workflow 文件里可以改：

```yaml
git commit -m "chore: refresh GitHub source raw pool"
```

## 运行方式

### 手动

进入 GitHub 仓库：

- Actions
- `build-github-source-pool`
- Run workflow

### 自动

等 cron 到时间自动跑。

## 产物文件

构建后会更新：

- `published/pools/github-source-raw.yaml`
- `published/manifests/github-source-raw.meta.yaml`
- `published/manifests/build-summary.json`

## 节点来源追踪

每个节点都会附带：

- `source_repo`
- `source_url`

这样后续可以精确回答：

> 某个节点到底来自哪个 GitHub 仓库、哪个具体 raw URL
