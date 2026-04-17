# GitHub Source Raw Pool Template

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

### 核心脚本

- `scripts/build_github_source_pool.py`
- `scripts/github_source_pool_utils.py`
- `scripts/mihomo_pool_utils.py`

### 产物目录

- `published/pools/`
- `published/manifests/`

### 文档

- `docs/COPY_TO_ANOTHER_REPO.md`

---

## 工作流逻辑

1. 读取 `config/github_source_urls.txt`
2. 逐个访问 GitHub raw 源
3. 自动解析：
   - Clash/Mihomo YAML
   - Base64 URI 订阅
4. 生成统一 raw 池
5. 给每个节点打上：
   - `source_repo`
   - `source_url`
6. 将产物提交回仓库

如果某个源临时失败：

- 不会导致整个工作流直接中止
- 失败信息会写入 `published/manifests/github-source-raw.meta.yaml`

---

## 如何配置

只需要修改一个文件：

- `config/github_source_urls.txt`

每行一个 GitHub raw 源。

支持：

- Mihomo/Clash YAML
- Base64 订阅

不需要任何 GitHub Secret。

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
- **产出 raw 池**

VPS 建议只负责：

1. 拉 `published/pools/github-source-raw.yaml`
2. 本地做 live 测试
3. 剔除地区/关键词
4. 应用到 Mihomo
