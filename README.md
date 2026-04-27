# Release Autobuild

`build_release.py` checks the latest stable GitHub release for one configured
project, clones that tag, builds exactly one target, uploads the result to a
Telegram bot, then deletes the source tree and generated artifacts.

Secrets are read from environment variables. The Telegram bot token should not
be written into this repository.

The script also auto-loads these private env files when present, without
requiring `source`:

1. `~/.release-autobuild.env`
2. `config.env`
3. `.env`

`config.env` and `.env` are ignored by git.

## Projects

- `adguard` -> `AdguardTeam/AdGuardHome`
- `xray` -> `XTLS/Xray-core`
- `mihomo` -> `MetaCubeX/mihomo`
- `sing-box` -> `SagerNet/sing-box`
- `mosdns` -> `IrineSistiana/mosdns`
- `momogram` -> `dic1911/Momogram`
- `v2rayng` -> `2dust/v2rayNG`

## Targets

- `android-arm64` aliases: `android64`, `android-arm64-v8a`
- `windows-amd64` aliases: `win64`, `win-x86_64`, `windows-x86_64`
- `linux-amd64` aliases: `linux64`, `linux-x86_64`

Android app projects currently support `android-arm64` only. Go projects use
Go cross-compilation for all three targets.

## Setup

On the remote Linux builder:

```bash
cd /opt
git clone <your-private-scripts-repo> release-autobuild
cd /opt/release-autobuild
cp config.env.example ~/.release-autobuild.env
nano ~/.release-autobuild.env
python3 build_release.py --list
```

On this Windows workspace:

```powershell
cd "D:\New project\release-autobuild"
Copy-Item .env.example .env
notepad .env
python build_release.py --poll-all --check-only
```

Minimum tools:

- Go projects: `python3 git go`
- AdGuard Home: `python3 git go make node npm`
- Android apps: `python3 git openjdk-17 Android SDK/NDK`
- Momogram additionally needs `yasm`, submodules, `TELEGRAM_APP_ID`, and
  `TELEGRAM_APP_HASH`

## Run One Build

```bash
source ~/.release-autobuild.env
python3 build_release.py --project xray --target linux-amd64
python3 build_release.py --project xray --target windows-amd64
python3 build_release.py --project xray --target android-arm64
```

The script records successful uploads in `state/`. If the same tag and target
were already uploaded, it exits without rebuilding. Use `--force` to rebuild.

Check without compiling:

```bash
python3 build_release.py --project sing-box --target linux-amd64 --check-only
```

Build without Telegram upload for smoke testing:

```bash
python3 build_release.py --project mosdns --target linux-amd64 --no-upload --force
```

## Poll Everything

Queue mode checks every configured project and target, then processes only the
missing artifacts. It still builds one target at a time. For each project tag it
clones the source once, builds a target, uploads that target, deletes that
target's output directory, then moves to the next target. When all pending
targets for that project are done, it deletes the cloned source tree and moves
to the next project.

Dry-run queue status:

```bash
python3 build_release.py --poll-all --check-only
```

One full pass:

```bash
python3 build_release.py --poll-all
```

Daemon-style polling every hour:

```bash
python3 build_release.py --poll-all --interval 3600
```

If a target fails, it is left pending for the next poll. By default the queue
continues to the next target/project; add `--stop-on-error` when debugging.

## Cron Example

Run one queue pass from cron. The script has a lock file, so overlapping cron
hits will not compile two repositories at once.

```cron
5 * * * * cd /opt/release-autobuild && . ~/.release-autobuild.env && python3 build_release.py --poll-all
```

Manual single-target commands are still useful for testing one project/target
without walking the whole queue.

## Chinese Quick Start

这个仓库按你的队列要求工作：

- 检测每个仓库最新稳定 GitHub release，跳过 draft、prerelease、alpha、beta、rc、nightly 等版本。
- 读取 `state/` 判断某个 `项目 + 目标平台 + release tag` 是否已经上传。
- `--poll-all` 模式一次只 clone 一个仓库。
- 同一个仓库内逐个目标编译，例如 `xray android-arm64`、`xray windows-amd64`、`xray linux-amd64`。
- 每个目标编译完成后立刻上传 Telegram，然后删除该目标产物目录。
- 该仓库所有待处理目标结束后删除源码目录，再进入下一个仓库。
- 全局锁文件在 `state/.build.lock`，避免两个任务同时编译。

把机器人 token 和用户 id 放到本地 `.env`：

```ini
TG_BOT_TOKEN=你的机器人token
TG_CHAT_ID=你的用户id
```

查看还有哪些目标没上传：

```powershell
python build_release.py --poll-all --check-only
```

开始顺序构建并上传：

```powershell
python build_release.py --poll-all
```

## v2rayNG No Promotion Patch

`patches/v2rayng-no-ads.py` is applied automatically after clone and before
Gradle runs. It hides the promotion drawer item and blanks `APP_PROMOTION_URL`.
If upstream changes those files, update the hook instead of editing
`build_release.py`.

## Notes

- Telegram upload uses `sendDocument`; very large APKs may hit Telegram Bot API
  upload limits.
- Source and output files live under a temporary directory and are removed after
  success or failure. Use `--keep-on-failure` only while debugging.
- Latest release detection ignores GitHub drafts, prereleases, and tags that
  look like alpha/beta/rc/nightly builds.
