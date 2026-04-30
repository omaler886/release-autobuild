# Release Autobuild / 自动发布构建

[![Release Autobuild](https://github.com/omaler886/release-autobuild/actions/workflows/release-autobuild.yml/badge.svg)](https://github.com/omaler886/release-autobuild/actions/workflows/release-autobuild.yml)

`release-autobuild` 用来自动跟踪上游项目的最新稳定 GitHub Release，可按上游仓库并行编译、同一项目内顺序处理目标平台，上传到 Telegram，并用 `state/` 记录已上传版本，避免重复构建。

`release-autobuild` tracks the latest stable GitHub Release of configured upstream projects, can build different upstream repositories in parallel while processing targets from the same project sequentially, uploads artifacts to Telegram, and records completed uploads in `state/` to avoid duplicate builds.

它适合部署在 Linux 构建机上通过 cron 运行，也可以直接使用 GitHub Actions 定时执行。

It can run on a Linux builder through cron, or run on schedule through GitHub Actions.

## 功能概览 / Features

中文:

- 自动查询上游最新稳定 Release，跳过 draft、prerelease、alpha、beta、rc、nightly、snapshot、preview 等版本。
- 支持单项目单目标构建，也支持 `--poll-all` 队列模式。
- `--poll-all` 会同步读取上游全部稳定 Release；如果上游没有 GitHub Releases，则自动 fallback 到 tags。默认只构建/上传最近 3 个稳定版本，可用 `--push-release-limit` 或 `PUSH_RELEASE_LIMIT` 调整。
- GitHub Actions 可自动生成 `upstream/<project>` 分支，例如 `upstream/xray`、`upstream/mihomo`，每个分支写入该上游仓库的 Release 同步元数据。
- 队列模式默认保守顺序执行；设置 `--jobs` 或 `BUILD_JOBS` 后，不同上游项目可在同一构建机内并行构建，同一项目内仍逐个目标构建和上传。
- 可选本地源码缓存 `SOURCE_CACHE_DIR`，每个上游仓库使用独立本地 branch，例如 `autobuild/xray`、`autobuild/mihomo`，实际构建仍在临时 clone 中完成。
- 使用 `state/*.json` 记录 `项目 + 目标平台 + tag` 上传历史，相同版本默认跳过，可用 `--force` 强制重建。
- 上传前自动打包产物；Windows 默认 zip，Linux/Android Go 目标默认 tar.gz，APK 目标可直接上传 APK。
- 支持项目补丁钩子，例如 `patches/v2rayng-no-ads.py` 会在 v2rayNG 构建前隐藏推广入口。
- 内置全局锁 `state/.build.lock`，避免 cron 或 Actions 并发触发时同时编译。

English:

- Detects the latest stable upstream GitHub Release and skips drafts, prereleases, alpha, beta, rc, nightly, snapshot, and preview builds.
- Supports single project/target builds and a full queue mode through `--poll-all`.
- `--poll-all` syncs the full stable upstream Release list; if a repository has no GitHub Releases, it falls back to tags. It builds/uploads only the newest 3 stable versions by default. Tune it with `--push-release-limit` or `PUSH_RELEASE_LIMIT`.
- GitHub Actions can generate `upstream/<project>` branches, such as `upstream/xray` and `upstream/mihomo`, each containing release sync metadata for that upstream repository.
- Queue mode runs conservatively by default; with `--jobs` or `BUILD_JOBS`, different upstream projects can build in parallel on one builder while targets from the same project still build and upload sequentially.
- Optional local source cache through `SOURCE_CACHE_DIR`; each upstream repository gets its own local branch, such as `autobuild/xray` or `autobuild/mihomo`, while real builds still happen in temporary clones.
- Uses `state/*.json` to record `project + target + tag` upload history; already uploaded releases are skipped unless `--force` is used.
- Packages artifacts before upload; Windows uses zip, Linux/Android Go targets use tar.gz, and APK targets can be uploaded directly.
- Supports project patch hooks, such as `patches/v2rayng-no-ads.py`, which hides the v2rayNG promotion entry before building.
- Uses the global lock file `state/.build.lock` to prevent concurrent builds from cron or GitHub Actions.

## 支持项目 / Supported Projects

| 项目 / Project | 上游仓库 / Upstream Repository | 类型 / Type | 支持目标 / Supported Targets | 常用别名 / Aliases |
| --- | --- | --- | --- | --- |
| `adguard` | `AdguardTeam/AdGuardHome` | AdGuardHome | `android-arm64`, `windows-amd64`, `linux-amd64` | `adguardhome`, `adguard-home` |
| `xray` | `XTLS/Xray-core` | Go | `android-arm64`, `windows-amd64`, `linux-amd64` | `xray-core` |
| `mihomo` | `MetaCubeX/mihomo` | Go | `android-arm64`, `windows-amd64`, `linux-amd64` | `clash-meta`, `clashmeta` |
| `sing-box` | `SagerNet/sing-box` | Go | `android-arm64`, `windows-amd64`, `linux-amd64` | `singbox` |
| `sing-box-subscribe` | `yelnoo/sing-box` | Go | `android-arm64`, `windows-amd64`, `linux-amd64` | `singbox-subscribe`, `sing-box-sub`, `singbox-sub`, `yelnoo-sing-box` |
| `mosdns` | `IrineSistiana/mosdns` | Go | `android-arm64`, `windows-amd64`, `linux-amd64` | - |
| `momogram` | `dic1911/Momogram` | Android/Gradle | `android-arm64` | - |
| `v2rayng` | `2dust/v2rayNG` | Android/Gradle | `android-arm64` | `v2ray-ng` |

## 支持目标 / Supported Targets

| 目标 / Target | 说明 / Description | 别名 / Aliases |
| --- | --- | --- |
| `android-arm64` | Android arm64 / arm64-v8a | `android64`, `android-arm64-v8a`, `android-aarch64`, `arm64-android` |
| `windows-amd64` | Windows x86_64 | `win64`, `win-x86_64`, `windows-x86_64`, `windows64` |
| `linux-amd64` | Linux x86_64 | `linux64`, `linux-x86_64` |

Android App 项目目前只支持 `android-arm64`。Go 项目通过 Go 交叉编译生成对应平台二进制。

Android app projects currently support `android-arm64` only. Go projects use Go cross-compilation to produce binaries for each target.

## 快速开始 / Quick Start

### 本地试运行 / Local Dry Run

Windows PowerShell:

```powershell
cd "D:\New project\release-autobuild"
Copy-Item .env.example .env
notepad .env

python build_release.py --list
python build_release.py --poll-all --check-only
```

Linux:

```bash
git clone https://github.com/omaler886/release-autobuild.git
cd release-autobuild
cp config.env.example ~/.release-autobuild.env
nano ~/.release-autobuild.env

python3 build_release.py --list
python3 build_release.py --poll-all --check-only
```

### 构建一个目标 / Build One Target

只检查最新版本和本地状态:

Check the latest release and local state only:

```bash
python3 build_release.py --project xray --target linux-amd64 --check-only
```

构建但不上传，适合冒烟测试:

Build without uploading, useful for smoke testing:

```bash
python3 build_release.py --project mosdns --target linux-amd64 --no-upload --force
```

构建并上传:

Build and upload:

```bash
python3 build_release.py --project xray --target linux-amd64
python3 build_release.py --project xray --target windows-amd64
python3 build_release.py --project xray --target android-arm64
```

如果当前 tag 和目标已经在 `state/` 中记录成功上传，脚本会直接跳过。需要重新构建时加 `--force`。

If the current tag and target are already recorded in `state/`, the script exits without rebuilding. Use `--force` to rebuild.

### 轮询所有项目 / Poll All Projects

查看所有项目还有哪些目标待构建:

Print pending targets for every project:

```bash
python3 build_release.py --poll-all --check-only
```

执行一轮队列:

Run one full queue pass:

```bash
python3 build_release.py --poll-all
```

默认会同步读取每个上游仓库全部稳定 Release；没有 Releases 的仓库会使用 tags。只构建/上传最近 3 个稳定版本:

By default, every upstream repository's full stable Release list is synced; repositories without Releases use tags. Only the newest 3 stable versions are built/uploaded:

```bash
python3 build_release.py --poll-all --push-release-limit 3
```

在同一构建机内并行处理不同上游仓库，或限制 `poll-all` 只处理指定项目:

Build different upstream repositories in parallel on one builder, or limit a poll-all pass to selected projects:

```bash
python3 build_release.py --poll-all --jobs 3
python3 build_release.py --poll-all --projects xray,mihomo
```

使用本地源码缓存时，每个上游仓库会写入自己的本地 branch，随后从缓存 clone 临时工作树进行编译:

With a local source cache, each upstream repository is written to its own local branch, then a temporary worktree clone is used for the actual build:

```bash
python3 build_release.py --poll-all --jobs 3 --source-cache-dir /opt/release-autobuild-sources
```

每小时循环轮询:

Repeat every hour:

```bash
python3 build_release.py --poll-all --interval 3600
```

默认情况下，某个目标失败后队列会继续处理后续目标或项目。调试时可以加 `--stop-on-error`，遇到第一个失败立即停止。

By default, a failed target stays pending and the queue continues. Add `--stop-on-error` while debugging to stop after the first failure.

## 配置 / Configuration

脚本会自动读取环境变量。没有使用 `--env-file` 时，加载顺序如下:

The script loads environment variables automatically. Without `--env-file`, values are loaded in this order:

1. 当前 shell 中已有的环境变量 / Existing environment variables from the current shell
2. `~/.release-autobuild.env`
3. `config.env`
4. `.env`

同一个变量只采用最先出现的值。使用 `--env-file <path>` 时，只读取指定文件，并覆盖当前进程中的同名变量。

For duplicate variables, the first value wins. With `--env-file <path>`, only the specified file is loaded, and variables in that file override existing values in the current process.

`config.env` 和 `.env` 已被 `.gitignore` 忽略，不要把真实 token 提交到仓库。

`config.env` and `.env` are ignored by git. Do not commit real tokens or secrets.

### 必填变量 / Required Variables

| 变量 / Variable | 说明 / Description |
| --- | --- |
| `TG_BOT_TOKEN` | Telegram Bot Token |
| `TG_CHAT_ID` | 接收文件的 Telegram 用户、群组或频道 ID / Telegram user, group, or channel ID that receives uploaded files |

### 常用可选变量 / Common Optional Variables

| 变量 / Variable | 说明 / Description |
| --- | --- |
| `GITHUB_TOKEN` | 提高 GitHub API 速率限制 / Increases the GitHub API rate limit |
| `GITHUB_API_RETRIES` | GitHub API 短重试次数，默认 `3` / GitHub API retry attempts, default `3` |
| `GITHUB_API_TIMEOUT` | 单次 GitHub API 请求超时秒数，默认 `60` / Per-request GitHub API timeout in seconds, default `60` |
| `BUILD_JOBS` | `--poll-all` 并行构建的上游项目数，默认 `1` / Number of upstream projects built in parallel by `--poll-all`, default `1` |
| `PUSH_RELEASE_LIMIT` | `--poll-all` 构建/上传的最近稳定版本数量，默认 `3` / Number of newest stable versions built/uploaded by `--poll-all`, default `3` |
| `UPSTREAM_BRANCH_PREFIX` | 远端 Release 元数据分支前缀，默认 `upstream` / Remote release metadata branch prefix, default `upstream` |
| `SOURCE_CACHE_DIR` | 本地上游源码缓存目录；启用后每个上游仓库写入独立本地 branch / Local upstream source cache directory; each upstream repo is written to its own local branch |
| `SOURCE_BRANCH_PREFIX` | 本地缓存 branch 前缀，默认 `autobuild` / Local cache branch prefix, default `autobuild` |
| `SOURCE_BRANCH_<PROJECT>` | 覆盖单个项目的本地缓存 branch，例如 `SOURCE_BRANCH_XRAY` / Override one project's local cache branch, for example `SOURCE_BRANCH_XRAY` |
| `ANDROID_HOME` / `ANDROID_SDK_ROOT` | Android SDK 路径，Android 项目必需 / Android SDK path, required for Android builds |
| `JAVA_HOME` | JDK 路径，建议 JDK 17 / JDK path, JDK 17 is recommended |
| `TELEGRAM_APP_ID` / `TELEGRAM_APP_HASH` | Momogram 构建所需 Telegram API 参数 / Telegram API credentials for Momogram builds |
| `ALIAS_NAME` / `KEYSTORE_PASS` / `ALIAS_PASS` | Momogram 签名参数 / Momogram signing options |
| `V2RAYNG_LIBV2RAY_AAR` | 自定义 `libv2ray.aar` 路径；未设置时自动下载 `2dust/AndroidLibXrayLite` 最新稳定 Release 资产 / Custom `libv2ray.aar` path; if unset, the latest stable asset from `2dust/AndroidLibXrayLite` is downloaded |
| `CGO_ENABLED` | Go 构建时的 CGO 开关，默认 `0` / CGO switch for Go builds, default `0` |
| `GO_LDFLAGS` | Go 构建 ldflags，默认 `-s -w -buildid=` / Go build ldflags, default `-s -w -buildid=` |
| `GOARM64` | Android arm64 Go 目标使用，默认 `v8.0` / Used for Android arm64 Go targets, default `v8.0` |
| `SING_BOX_TAGS` | sing-box 构建 tags / Build tags for sing-box |
| `MOMOGRAM_GRADLE_TASKS` / `V2RAYNG_GRADLE_TASKS` | 覆盖对应 Android 项目的 Gradle task 列表；Momogram 默认 `TMessagesProj:assembleRelease` / Overrides Gradle tasks for the corresponding Android project; Momogram defaults to `TMessagesProj:assembleRelease` |

## 构建环境依赖 / Build Dependencies

| 项目类型 / Project Type | 最低依赖 / Minimum Dependencies |
| --- | --- |
| Go 项目 / Go projects | `python3`, `git`, `go` |
| AdGuardHome | `python3`, `git`, `go`, `make`, `node`, `npm` |
| Android App | `python3`, `git`, `openjdk-17`, Android SDK/NDK |
| Momogram | Android App 依赖，加 `autoconf`, `automake`, `libtool`, `meson`, `nasm`, `ninja-build`, `pkg-config`, `yasm`，并需要 NDK `21.4.7075529` / Android app dependencies plus `autoconf`, `automake`, `libtool`, `meson`, `nasm`, `ninja-build`, `pkg-config`, `yasm`, and NDK `21.4.7075529` |

GitHub Actions 工作流会安装 Go、Java 17、Node、Android SDK/NDK 和常见 native 构建工具。自建机器请按实际项目补齐依赖。

The GitHub Actions workflow installs Go, Java 17, Node, Android SDK/NDK, and common native build tools. For self-hosted builders, install the dependencies required by the projects you want to build.

## GitHub Actions

仓库内置 `.github/workflows/release-autobuild.yml`，支持手动运行和每 6 小时定时运行。

The repository includes `.github/workflows/release-autobuild.yml`, supporting manual runs and scheduled runs every 6 hours.

### Secrets

在 GitHub 仓库的 `Settings -> Secrets and variables -> Actions` 中添加:

Add these under `Settings -> Secrets and variables -> Actions` in your GitHub repository:

```text
TG_BOT_TOKEN
TG_CHAT_ID
```

可选 / Optional:

```text
TELEGRAM_APP_ID
TELEGRAM_APP_HASH
V2RAYNG_LIBV2RAY_AAR
```

### 手动运行 / Manual Run

中文:

1. 打开 GitHub 仓库的 `Actions`。
2. 选择 `Release Autobuild`。
3. 点击 `Run workflow`。
4. 选择运行模式。

English:

1. Open `Actions` in the GitHub repository.
2. Select `Release Autobuild`.
3. Click `Run workflow`.
4. Choose a run mode.

| mode | 行为 / Behavior |
| --- | --- |
| `poll-all` | 检查所有项目和目标，并用 GitHub Actions matrix 按项目并行构建 / Check every project and target, then build projects in parallel with a GitHub Actions matrix |
| `check` | 用 matrix 并行输出各项目 pending/uploaded 状态，不编译 / Print each project pending/uploaded state through the matrix, without compiling |
| `single` | 只构建指定 `project` 和 `target` / Build one specified `project` and `target` |

| 输入 / Input | 说明 / Description |
| --- | --- |
| `project` | `single` 模式必填，例如 `xray` / Required in `single` mode, for example `xray` |
| `target` | `single` 模式必填，例如 `linux-amd64` / Required in `single` mode, for example `linux-amd64` |
| `force` | 重建已经上传过的 tag/target / Rebuild an already uploaded tag/target |
| `no_upload` | 只构建，不上传 Telegram，也不写入 `state/` / Build only, without uploading to Telegram or writing `state/` |
| `stop_on_error` | 队列遇到第一个失败即停止 / Stop the queue after the first failure |
| `jobs` | `poll-all`/`check` 的 GitHub Actions matrix 最大并行项目数，默认 `3`，工作流会按免费版友好的策略上限限制为 `3` / Maximum parallel project jobs for the GitHub Actions matrix in `poll-all`/`check`, default `3`, capped at `3` for free-plan-friendly runs |
| `push_release_limit` | `poll-all` 构建/上传的最近稳定版本数量，默认 `3` / Newest stable versions built/uploaded in `poll-all`, default `3` |
| `sync_branches` | 生成/更新 `upstream/<project>` 元数据分支，默认开启 / Create/update `upstream/<project>` metadata branches, enabled by default |

工作流的关键行为:

Workflow behavior:

- `concurrency` 保证同一时间只有一个 Release Autobuild 运行。
- `concurrency` ensures only one Release Autobuild run is active at a time.
- `poll-all` 和 `check` 会按上游项目拆成 matrix jobs，使用标准 `ubuntu-latest` GitHub-hosted runner，默认最多 3 个项目同时运行。
- `poll-all` and `check` are split into one matrix job per upstream project, using standard `ubuntu-latest` GitHub-hosted runners with at most 3 projects running at once by default.
- 工作流不使用 larger runner、自托管 runner 或付费外部构建服务。
- The workflow does not use larger runners, self-hosted runners, or paid external build services.
- 构建结果上传到 Telegram。
- Built artifacts are uploaded to Telegram.
- 各 matrix job 通过 artifact 回传自己的 `state/*.json`，最后由一个汇总 job 统一提交，避免并行 `git push` 冲突。
- Each matrix job returns its own `state/*.json` files through artifacts; one final aggregation job commits them to avoid parallel `git push` conflicts.
- `logs/` 会作为 workflow artifact 保存 14 天，便于排查失败原因。
- `logs/` is uploaded as a workflow artifact and retained for 14 days for debugging.

## Cron 示例 / Cron Examples

在自建 Linux 构建机上每小时执行一轮:

Run one queue pass every hour on a self-hosted Linux builder:

```cron
5 * * * * cd /opt/release-autobuild && python3 build_release.py --poll-all >> /var/log/release-autobuild.log 2>&1
```

如果你使用 `~/.release-autobuild.env`，脚本会自动读取，不需要在 cron 中 `source`。如果你想指定某个配置文件:

If you use `~/.release-autobuild.env`, the script loads it automatically, so `source` is not required in cron. To use a specific env file:

```cron
5 * * * * cd /opt/release-autobuild && python3 build_release.py --env-file /etc/release-autobuild.env --poll-all >> /var/log/release-autobuild.log 2>&1
```

本地构建机资源足够时，可以并行不同上游仓库，并把源码缓存到独立 branch:

When the local builder has enough resources, build different upstream repositories in parallel and keep cached sources on separate branches:

```cron
5 * * * * cd /opt/release-autobuild && python3 build_release.py --poll-all --jobs 3 --source-cache-dir /opt/release-autobuild-sources >> /var/log/release-autobuild.log 2>&1
```

## CLI 参数 / CLI Options

```text
--list                  列出支持的项目和目标 / List configured projects and targets
--project <name>        单项目模式的项目名 / Project name for single-build mode
--target <target>       单项目模式的目标平台 / Target platform for single-build mode
--poll-all              检查所有项目和目标，构建待处理产物 / Check every project/target and build pending artifacts
--projects <names>      与 --poll-all 搭配，只处理逗号或空格分隔的指定项目 / Limit --poll-all to comma/space-separated projects
--interval <seconds>    与 --poll-all 搭配，按间隔循环执行 / Repeat every N seconds with --poll-all
--jobs <n>              poll-all 并行上游项目数，默认 BUILD_JOBS 或 1 / Parallel upstream projects for poll-all, default BUILD_JOBS or 1
--push-release-limit <n> poll-all 只构建/上传最近 n 个稳定版本，默认 PUSH_RELEASE_LIMIT 或 3 / Build/upload newest n stable versions in poll-all, default PUSH_RELEASE_LIMIT or 3
--check-only            只检查最新 Release 和 state，不编译 / Check release and state only, without compiling
--force                 即使 state 已记录成功，也重新构建 / Rebuild even if the state already marks it uploaded
--no-upload             构建但不上传 Telegram，也不更新 state / Build without uploading to Telegram or updating state
--stop-on-error         poll-all 模式下遇到第一个失败立即停止 / Stop poll-all after the first failure
--keep-on-failure       构建失败时保留临时目录 / Keep the temporary work directory after a failure
--state-dir <path>      指定 state 目录，默认 ./state / State directory, default ./state
--log-dir <path>        指定日志目录，默认 ./logs / Log directory, default ./logs
--patch-dir <path>      指定补丁钩子目录，默认 ./patches / Patch hook directory, default ./patches
--env-file <path>       指定 env 文件 / Load a specific env file
--work-dir <path>       指定临时源码和产物目录的父目录 / Parent directory for temporary source and build outputs
--source-cache-dir <path>       本地上游源码缓存目录 / Local upstream source cache directory
--source-branch-prefix <prefix> 本地缓存 branch 前缀，默认 autobuild / Local cache branch prefix, default autobuild
--sync-upstream-branches        生成/推送 upstream/<project> 元数据分支 / Generate and push upstream/<project> metadata branches
--upstream-branch-prefix <name> 元数据分支前缀，默认 upstream / Metadata branch prefix, default upstream
```

## 目录说明 / Repository Layout

| 路径 / Path | 说明 / Description |
| --- | --- |
| `build_release.py` | 主脚本，包含项目配置、Release 查询、构建、打包、上传和 state 管理 / Main script for project config, release lookup, build, package, upload, and state management |
| `.github/workflows/release-autobuild.yml` | GitHub Actions 工作流 / GitHub Actions workflow |
| `config.env.example` | Linux/远程构建机配置模板 / Config template for Linux or remote builders |
| `.env.example` | 本地配置模板 / Local config template |
| `state/*.json` | 每个项目和目标的上传状态，建议提交到仓库 / Upload state per project and target, recommended to commit |
| `logs/` | 构建日志，默认不提交 / Build logs, ignored by default |
| `patches/` | 构建前补丁钩子目录 / Patch hooks executed before build |

补丁钩子命名规则:

Patch hook naming rules:

```text
patches/<project>.py
patches/<project>.sh
patches/<project>-no-ads.py
patches/<project>-no-ads.sh
```

命中的钩子会在 clone 完上游源码后、开始构建前执行，并接收源码目录作为第一个参数。

Matched hooks run after the upstream source is cloned and before the build starts. The source directory is passed as the first argument.

## v2rayNG 去推广补丁 / v2rayNG No Promotion Patch

`patches/v2rayng-no-ads.py` 会自动应用到 v2rayNG 源码:

`patches/v2rayng-no-ads.py` is applied automatically to v2rayNG source code:

- 隐藏侧边栏 promotion 菜单项。
- Hides the promotion drawer item.
- 将 `APP_PROMOTION_URL` 置空。
- Clears `APP_PROMOTION_URL`.

如果 v2rayNG 上游文件路径或字段名变化，请优先更新这个补丁脚本，不要把项目特化逻辑直接塞进 `build_release.py`。

If upstream v2rayNG file paths or field names change, update this patch script first instead of putting project-specific logic directly into `build_release.py`.

## 常见问题 / FAQ

### 一直提示 already uploaded / It keeps saying already uploaded

说明 `state/<project>_<target>.json` 的上传历史里已经记录该 tag。需要重建时加 `--force`。

It means `state/<project>_<target>.json` already contains that tag in its upload history. Use `--force` to rebuild.

```bash
python3 build_release.py --project xray --target linux-amd64 --force
```

### GitHub API 速率限制 / GitHub API rate limit

设置 `GITHUB_TOKEN`。GitHub Actions 中已经默认传入 `${{ github.token }}`；本地或自建机器可以写入 `~/.release-autobuild.env`。

Set `GITHUB_TOKEN`. GitHub Actions already passes `${{ github.token }}` by default. For local or self-hosted builders, put it in `~/.release-autobuild.env`.

### Android 构建找不到 SDK / Android build cannot find the SDK

确认设置了 `ANDROID_HOME` 或 `ANDROID_SDK_ROOT`，并安装项目需要的 build-tools、platform、NDK 和 CMake。

Make sure `ANDROID_HOME` or `ANDROID_SDK_ROOT` is set, and install the required build-tools, platform, NDK, and CMake versions.

### Telegram 上传失败 / Telegram upload failed

检查 `TG_BOT_TOKEN`、`TG_CHAT_ID` 是否正确，以及机器人是否有权限给目标聊天发送文件。非常大的 APK 或压缩包也可能触及 Telegram Bot API 限制。

Check `TG_BOT_TOKEN`, `TG_CHAT_ID`, and whether the bot has permission to send files to the target chat. Very large APKs or archives may also hit Telegram Bot API limits.

### 构建失败后临时目录被删除 / Temporary directory was removed after failure

调试时加 `--keep-on-failure`，脚本会保留失败的临时工作目录并打印路径。

Use `--keep-on-failure` while debugging. The script will keep the failed temporary work directory and print its path.

### 提示 another build is already running / another build is already running

脚本检测到 `state/.build.lock`。先确认没有正在运行的构建进程；如果是异常退出留下的陈旧锁文件，可以手动删除后重试。

The script found `state/.build.lock`. First make sure no build is still running. If it is a stale lock from an abnormal exit, remove the lock file manually and retry.

## 增加新项目 / Add a New Project

中文:

1. 在 `build_release.py` 的 `PROJECTS` 中添加 `Project(...)`。
2. 选择已有 `kind`：`go`、`adguardhome`、`momogram`、`v2rayng`。
3. 如果构建流程不同，新增一个 `build_<kind>()`，并在 `build_project()` 中分发。
4. 在 `supports` 中声明目标平台。
5. 需要修改上游源码时，在 `patches/` 下添加项目钩子。
6. 用 `--check-only`、`--no-upload --force`、正式上传三步验证。

English:

1. Add a `Project(...)` entry to `PROJECTS` in `build_release.py`.
2. Choose an existing `kind`: `go`, `adguardhome`, `momogram`, or `v2rayng`.
3. If the build flow is different, add a new `build_<kind>()` function and dispatch it from `build_project()`.
4. Declare supported target platforms in `supports`.
5. Add a project hook under `patches/` if upstream source changes are needed before build.
6. Verify with `--check-only`, then `--no-upload --force`, then a real upload.

## 安全提示 / Security Notes

- 不要提交 `.env`、`config.env`、真实 Telegram token、签名密码或私有 AAR。
- Do not commit `.env`, `config.env`, real Telegram tokens, signing passwords, or private AAR files.
- `state/*.json` 只记录项目、tag、commit、目标平台、上传历史和已上传文件名，适合提交。
- `state/*.json` only records project, tag, commit, target, upload history, and uploaded file names, so it is suitable to commit.
- Actions 的 secrets 只应放在 GitHub Secrets 中，不要写入 workflow 明文。
- Action secrets should live in GitHub Secrets only. Do not write them into workflow files in plain text.
