#!/usr/bin/env python3
"""
Build upstream releases, upload them to Telegram, then clean up.

Single-build mode accepts exactly one project and one target. Poll-all mode
checks every configured project and target, but still builds only one target at
a time.
Secrets are read from environment variables:
  TG_BOT_TOKEN - Telegram bot token
  TG_CHAT_ID   - target chat/user id
"""

from __future__ import annotations

import argparse
import dataclasses
import datetime as dt
import fnmatch
import json
import os
import re
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
import zipfile
from pathlib import Path
from typing import Iterable


ROOT = Path(__file__).resolve().parent
DEFAULT_STATE_DIR = ROOT / "state"
DEFAULT_LOG_DIR = ROOT / "logs"
DEFAULT_PATCH_DIR = ROOT / "patches"
USER_AGENT = "codex-release-autobuild/1.0"
DEFAULT_ENV_FILES = (
    Path.home() / ".release-autobuild.env",
    ROOT / "config.env",
    ROOT / ".env",
)


@dataclasses.dataclass(frozen=True)
class Target:
    key: str
    goos: str
    goarch: str
    exe_suffix: str = ""


@dataclasses.dataclass(frozen=True)
class Project:
    key: str
    repo: str
    kind: str
    binary: str
    main: str = "."
    supports: tuple[str, ...] = ("android-arm64", "windows-amd64", "linux-amd64")
    aliases: tuple[str, ...] = ()
    gradle_dir: str = "."
    gradle_tasks: tuple[str, ...] = ("assembleRelease",)
    clone_submodules: bool = False

    @property
    def clone_url(self) -> str:
        return f"https://github.com/{self.repo}.git"


TARGETS: dict[str, Target] = {
    "android-arm64": Target("android-arm64", "android", "arm64"),
    "windows-amd64": Target("windows-amd64", "windows", "amd64", ".exe"),
    "linux-amd64": Target("linux-amd64", "linux", "amd64"),
}

TARGET_ALIASES = {
    "android64": "android-arm64",
    "android-arm64-v8a": "android-arm64",
    "android-aarch64": "android-arm64",
    "arm64-android": "android-arm64",
    "win64": "windows-amd64",
    "win-x86_64": "windows-amd64",
    "windows-x86_64": "windows-amd64",
    "windows64": "windows-amd64",
    "linux64": "linux-amd64",
    "linux-x86_64": "linux-amd64",
}

PROJECTS: dict[str, Project] = {
    "adguard": Project(
        key="adguard",
        repo="AdguardTeam/AdGuardHome",
        kind="adguardhome",
        binary="AdGuardHome",
        aliases=("adguardhome", "adguard-home"),
    ),
    "xray": Project(
        key="xray",
        repo="XTLS/Xray-core",
        kind="go",
        binary="xray",
        main="./main",
        aliases=("xray-core",),
    ),
    "mihomo": Project(
        key="mihomo",
        repo="MetaCubeX/mihomo",
        kind="go",
        binary="mihomo",
        main=".",
        aliases=("clash-meta", "clashmeta"),
    ),
    "sing-box": Project(
        key="sing-box",
        repo="SagerNet/sing-box",
        kind="go",
        binary="sing-box",
        main="./cmd/sing-box",
        aliases=("singbox",),
    ),
    "mosdns": Project(
        key="mosdns",
        repo="IrineSistiana/mosdns",
        kind="go",
        binary="mosdns",
        main=".",
    ),
    "momogram": Project(
        key="momogram",
        repo="dic1911/Momogram",
        kind="momogram",
        binary="Momogram",
        supports=("android-arm64",),
        clone_submodules=True,
        gradle_tasks=("assembleRelease",),
    ),
    "v2rayng": Project(
        key="v2rayng",
        repo="2dust/v2rayNG",
        kind="v2rayng",
        binary="v2rayNG",
        supports=("android-arm64",),
        aliases=("v2ray-ng",),
        gradle_dir="V2rayNG",
        gradle_tasks=("assembleRelease",),
    ),
}

PROJECT_ALIASES = {
    alias: key
    for key, project in PROJECTS.items()
    for alias in (key, *project.aliases)
}


class BuildError(RuntimeError):
    pass


class RunLock:
    def __init__(self, path: Path) -> None:
        self.path = path
        self.fd: int | None = None

    def __enter__(self) -> "RunLock":
        self.path.parent.mkdir(parents=True, exist_ok=True)
        try:
            self.fd = os.open(str(self.path), os.O_CREAT | os.O_EXCL | os.O_WRONLY)
        except FileExistsError as exc:
            raise BuildError(f"another build is already running: {self.path}") from exc
        os.write(self.fd, str(os.getpid()).encode("ascii", "ignore"))
        return self

    def __exit__(self, *_exc: object) -> None:
        if self.fd is not None:
            os.close(self.fd)
        try:
            self.path.unlink()
        except FileNotFoundError:
            pass


def utc_now() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()


def normalize_project(value: str) -> str:
    key = value.strip().lower()
    try:
        return PROJECT_ALIASES[key]
    except KeyError as exc:
        raise BuildError(f"unknown project: {value}") from exc


def normalize_target(value: str) -> str:
    key = value.strip().lower()
    key = TARGET_ALIASES.get(key, key)
    if key not in TARGETS:
        raise BuildError(f"unknown target: {value}")
    return key


def env_with_token() -> dict[str, str]:
    headers = {
        "Accept": "application/vnd.github+json",
        "User-Agent": USER_AGENT,
        "X-GitHub-Api-Version": "2022-11-28",
    }
    token = os.environ.get("GITHUB_TOKEN")
    if token:
        headers["Authorization"] = f"Bearer {token}"
    return headers


def strip_env_value(value: str) -> str:
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] and value[0] in {'"', "'"}:
        value = value[1:-1]
    return value


def load_env_file(path: Path, *, override: bool) -> None:
    if not path.is_file():
        return
    for lineno, raw in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[7:].strip()
        if "=" not in line:
            raise BuildError(f"invalid env line in {path}:{lineno}: {raw}")
        key, value = line.split("=", 1)
        key = key.strip()
        if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", key):
            raise BuildError(f"invalid env key in {path}:{lineno}: {key}")
        if override or key not in os.environ:
            os.environ[key] = strip_env_value(value)


def load_environment(env_file: Path | None) -> None:
    if env_file:
        load_env_file(env_file, override=True)
        return
    for path in DEFAULT_ENV_FILES:
        load_env_file(path, override=False)


def github_json(path: str) -> object:
    url = f"https://api.github.com{path}"
    request = urllib.request.Request(url, headers=env_with_token())
    try:
        with urllib.request.urlopen(request, timeout=60) as response:
            return json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", "replace")
        raise BuildError(f"GitHub API failed {exc.code}: {body[:300]}") from exc


def looks_stable_release(release: dict[str, object]) -> bool:
    if release.get("draft") or release.get("prerelease"):
        return False
    marker = f"{release.get('tag_name', '')} {release.get('name', '')}".lower()
    return not re.search(
        r"(^|[^a-z0-9])(alpha|beta|rc[0-9]*|pre[-.]?release|nightly|snapshot|preview)([^a-z0-9]|$)",
        marker,
    )


def latest_release(project: Project) -> dict[str, object]:
    data = github_json(f"/repos/{project.repo}/releases?per_page=30")
    if not isinstance(data, list):
        raise BuildError(f"unexpected releases response for {project.repo}")
    for release in data:
        if isinstance(release, dict) and looks_stable_release(release):
            tag = release.get("tag_name")
            if not isinstance(tag, str) or not tag:
                continue
            return release
    raise BuildError(f"no stable GitHub release found for {project.repo}")


def run(
    cmd: list[str],
    cwd: Path,
    env: dict[str, str] | None = None,
    log_file: Path | None = None,
) -> None:
    merged_env = os.environ.copy()
    if env:
        merged_env.update(env)
    line = f"+ {' '.join(cmd)}"
    print(line, flush=True)
    if log_file:
        with log_file.open("a", encoding="utf-8") as fh:
            fh.write(line + "\n")
    proc = subprocess.Popen(
        cmd,
        cwd=str(cwd),
        env=merged_env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        encoding="utf-8",
        errors="replace",
    )
    assert proc.stdout is not None
    with proc.stdout:
        for out_line in proc.stdout:
            print(out_line, end="")
            if log_file:
                with log_file.open("a", encoding="utf-8") as fh:
                    fh.write(out_line)
    code = proc.wait()
    if code != 0:
        raise BuildError(f"command failed with exit code {code}: {' '.join(cmd)}")


def clone_source(project: Project, tag: str, source_dir: Path, log_file: Path) -> None:
    cmd = ["git", "clone", "--depth", "1", "--branch", tag]
    if project.clone_submodules:
        cmd += ["--recurse-submodules", "--shallow-submodules"]
    cmd += [project.clone_url, str(source_dir)]
    run(cmd, cwd=source_dir.parent, log_file=log_file)
    if project.clone_submodules:
        run(["git", "submodule", "update", "--init", "--recursive"], cwd=source_dir, log_file=log_file)


def source_commit(source_dir: Path) -> str:
    try:
        return subprocess.check_output(
            ["git", "rev-parse", "HEAD"],
            cwd=str(source_dir),
            text=True,
            stderr=subprocess.DEVNULL,
        ).strip()
    except Exception:
        return ""


def write_metadata(
    dist_dir: Path,
    project: Project,
    target: Target,
    tag: str,
    commit: str,
) -> Path:
    meta = {
        "project": project.key,
        "repo": project.repo,
        "tag": tag,
        "target": target.key,
        "commit": commit,
        "built_at": utc_now(),
        "builder": USER_AGENT,
    }
    path = dist_dir / "BUILD-METADATA.json"
    path.write_text(json.dumps(meta, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return path


def go_env(target: Target) -> dict[str, str]:
    env = {
        "GOOS": target.goos,
        "GOARCH": target.goarch,
        "CGO_ENABLED": os.environ.get("CGO_ENABLED", "0"),
    }
    if target.key == "android-arm64":
        env.setdefault("GOARM64", os.environ.get("GOARM64", "v8.0"))
    return env


def build_go(project: Project, target: Target, source_dir: Path, dist_dir: Path, log_file: Path) -> list[Path]:
    out = dist_dir / (project.binary + target.exe_suffix)
    env = go_env(target)
    ldflags = os.environ.get("GO_LDFLAGS", "-s -w -buildid=")
    tags = ""
    if project.key == "sing-box":
        tags = os.environ.get(
            "SING_BOX_TAGS",
            "with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_acme,with_clash_api",
        )
    cmd = ["go", "build", "-trimpath", "-ldflags", ldflags, "-o", str(out)]
    if tags:
        cmd += ["-tags", tags]
    cmd.append(project.main)
    run(cmd, cwd=source_dir, env=env, log_file=log_file)
    return [out]


def build_adguardhome(project: Project, target: Target, source_dir: Path, dist_dir: Path, log_file: Path) -> list[Path]:
    run(["make", "init"], cwd=source_dir, log_file=log_file)
    run(["make", f"GOOS={target.goos}", f"GOARCH={target.goarch}"], cwd=source_dir, log_file=log_file)
    names = [project.binary + target.exe_suffix, project.binary]
    candidates = [source_dir / name for name in names]
    candidates += list(source_dir.glob(f"**/{project.binary}{target.exe_suffix}"))
    for candidate in candidates:
        if candidate.is_file():
            out = dist_dir / (project.binary + target.exe_suffix)
            shutil.copy2(candidate, out)
            return [out]
    raise BuildError("AdGuardHome build finished, but no binary was found")


def chmod_executable(path: Path) -> None:
    mode = path.stat().st_mode
    path.chmod(mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)


def write_local_properties(project: Project, source_dir: Path) -> None:
    lines: list[str] = []
    android_home = os.environ.get("ANDROID_HOME") or os.environ.get("ANDROID_SDK_ROOT")
    if android_home:
        lines.append(f"sdk.dir={android_home.replace(os.sep, '/')}")
    if project.key == "momogram":
        app_id = os.environ.get("TELEGRAM_APP_ID")
        app_hash = os.environ.get("TELEGRAM_APP_HASH")
        if app_id and app_hash:
            lines.append(f"TELEGRAM_APP_ID={app_id}")
            lines.append(f"TELEGRAM_APP_HASH={app_hash}")
        for name in ("ALIAS_NAME", "KEYSTORE_PASS", "ALIAS_PASS"):
            if os.environ.get(name):
                lines.append(f"{name}={os.environ[name]}")
    if not lines:
        return
    local_properties = source_dir / ("V2rayNG/local.properties" if project.key == "v2rayng" else "local.properties")
    existing = local_properties.read_text("utf-8") if local_properties.exists() else ""
    with local_properties.open("a", encoding="utf-8") as fh:
        if existing and not existing.endswith("\n"):
            fh.write("\n")
        fh.write("\n".join(lines) + "\n")


def apply_patch_hook(project: Project, source_dir: Path, patch_dir: Path, log_file: Path) -> None:
    hooks = [
        patch_dir / f"{project.key}.py",
        patch_dir / f"{project.key}.sh",
        patch_dir / f"{project.key}-no-ads.py",
        patch_dir / f"{project.key}-no-ads.sh",
    ]
    for hook in hooks:
        if hook.is_file():
            if hook.suffix == ".py":
                run([sys.executable, str(hook), str(source_dir)], cwd=source_dir, log_file=log_file)
            elif os.name == "nt":
                run(["bash", str(hook), str(source_dir)], cwd=source_dir, log_file=log_file)
            else:
                chmod_executable(hook)
                run([str(hook), str(source_dir)], cwd=source_dir, log_file=log_file)


def build_momogram(project: Project, target: Target, source_dir: Path, dist_dir: Path, log_file: Path) -> list[Path]:
    write_local_properties(project, source_dir)
    run_script = source_dir / "run"
    if run_script.exists():
        chmod_executable(run_script)
        run(["./run", "init", "libs"], cwd=source_dir, log_file=log_file)
        run(["./run", "libs", "update"], cwd=source_dir, log_file=log_file)
    return build_gradle(project, target, source_dir, dist_dir, log_file)


def build_v2rayng(project: Project, target: Target, source_dir: Path, dist_dir: Path, log_file: Path) -> list[Path]:
    write_local_properties(project, source_dir)
    aar = os.environ.get("V2RAYNG_LIBV2RAY_AAR")
    if aar:
        libs_dir = source_dir / "V2rayNG" / "app" / "libs"
        libs_dir.mkdir(parents=True, exist_ok=True)
        shutil.copy2(aar, libs_dir / "libv2ray.aar")
    return build_gradle(project, target, source_dir, dist_dir, log_file)


def build_gradle(project: Project, _target: Target, source_dir: Path, dist_dir: Path, log_file: Path) -> list[Path]:
    gradle_dir = source_dir / project.gradle_dir
    gradlew = gradle_dir / ("gradlew.bat" if os.name == "nt" else "gradlew")
    if not gradlew.exists():
        raise BuildError(f"gradle wrapper not found: {gradlew}")
    chmod_executable(gradlew)
    tasks = os.environ.get(f"{project.key.upper().replace('-', '_')}_GRADLE_TASKS")
    task_list = tasks.split() if tasks else list(project.gradle_tasks)
    for task in task_list:
        run([str(gradlew), task], cwd=gradle_dir, log_file=log_file)
    apks = collect_apks(gradle_dir)
    if not apks:
        raise BuildError(f"no APK found under {gradle_dir}")
    copied: list[Path] = []
    for apk in apks:
        dst = dist_dir / apk.name
        shutil.copy2(apk, dst)
        copied.append(dst)
    return copied


def collect_apks(gradle_dir: Path) -> list[Path]:
    all_apks = sorted(gradle_dir.glob("**/build/outputs/apk/**/*.apk"))
    if not all_apks:
        all_apks = sorted(gradle_dir.glob("**/*.apk"))
    release_apks = [p for p in all_apks if "release" in p.name.lower() or "release" in str(p.parent).lower()]
    chosen = release_apks or all_apks
    arm64 = [p for p in chosen if "arm64" in p.name.lower() or "arm64-v8a" in str(p.parent).lower()]
    universal = [p for p in chosen if "universal" in p.name.lower()]
    return arm64 or universal or chosen[:4]


def build_project(
    project: Project,
    target: Target,
    source_dir: Path,
    dist_dir: Path,
    patch_dir: Path,
    log_file: Path,
) -> list[Path]:
    apply_patch_hook(project, source_dir, patch_dir, log_file)
    if project.kind == "go":
        return build_go(project, target, source_dir, dist_dir, log_file)
    if project.kind == "adguardhome":
        return build_adguardhome(project, target, source_dir, dist_dir, log_file)
    if project.kind == "momogram":
        return build_momogram(project, target, source_dir, dist_dir, log_file)
    if project.kind == "v2rayng":
        return build_v2rayng(project, target, source_dir, dist_dir, log_file)
    raise BuildError(f"unsupported project kind: {project.kind}")


def reset_project_build_outputs(project: Project, source_dir: Path, log_file: Path) -> None:
    """Best-effort cleanup between targets when the same source clone is reused."""
    if project.kind in {"momogram", "v2rayng"}:
        # Android app projects have one supported target today. Keep Gradle caches.
        return
    if project.kind == "go":
        return
    if project.kind == "adguardhome":
        for pattern in (project.binary, project.binary + ".exe"):
            for path in source_dir.glob(f"**/{pattern}"):
                if path.is_file():
                    try:
                        path.unlink()
                    except OSError:
                        pass


def package_artifacts(
    project: Project,
    target: Target,
    tag: str,
    dist_dir: Path,
    artifacts: list[Path],
    metadata: Path,
) -> list[Path]:
    package_base = safe_name(f"{project.key}-{tag}-{target.key}")
    if len(artifacts) == 1 and artifacts[0].suffix.lower() == ".apk":
        return artifacts
    if target.key == "windows-amd64" or any(p.suffix.lower() == ".apk" for p in artifacts):
        archive = dist_dir / f"{package_base}.zip"
        with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as zf:
            for path in [metadata, *artifacts]:
                zf.write(path, arcname=path.name)
        return [archive]
    archive = dist_dir / f"{package_base}.tar.gz"
    with tarfile.open(archive, "w:gz") as tf:
        for path in [metadata, *artifacts]:
            tf.add(path, arcname=path.name)
    return [archive]


def safe_name(value: str) -> str:
    return re.sub(r"[^A-Za-z0-9._+-]+", "_", value).strip("_")


def multipart_body(fields: dict[str, str], file_field: str, file_path: Path) -> tuple[bytes, str]:
    boundary = f"----codex{uuid.uuid4().hex}"
    chunks: list[bytes] = []
    for name, value in fields.items():
        chunks.append(f"--{boundary}\r\n".encode())
        chunks.append(f'Content-Disposition: form-data; name="{name}"\r\n\r\n'.encode())
        chunks.append(value.encode("utf-8"))
        chunks.append(b"\r\n")
    chunks.append(f"--{boundary}\r\n".encode())
    chunks.append(
        (
            f'Content-Disposition: form-data; name="{file_field}"; '
            f'filename="{file_path.name}"\r\n'
            "Content-Type: application/octet-stream\r\n\r\n"
        ).encode()
    )
    chunks.append(file_path.read_bytes())
    chunks.append(b"\r\n")
    chunks.append(f"--{boundary}--\r\n".encode())
    return b"".join(chunks), boundary


def telegram_upload(file_path: Path, caption: str) -> None:
    token = os.environ.get("TG_BOT_TOKEN")
    chat_id = os.environ.get("TG_CHAT_ID")
    if not token or not chat_id:
        raise BuildError("TG_BOT_TOKEN and TG_CHAT_ID must be set")
    url = f"https://api.telegram.org/bot{token}/sendDocument"
    body, boundary = multipart_body(
        {
            "chat_id": chat_id,
            "caption": caption[:1024],
            "disable_content_type_detection": "false",
        },
        "document",
        file_path,
    )
    request = urllib.request.Request(
        url,
        data=body,
        headers={"Content-Type": f"multipart/form-data; boundary={boundary}", "User-Agent": USER_AGENT},
    )
    try:
        with urllib.request.urlopen(request, timeout=600) as response:
            data = json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        body_text = exc.read().decode("utf-8", "replace")
        raise BuildError(f"Telegram upload failed {exc.code}: {body_text[:300]}") from exc
    if not data.get("ok"):
        raise BuildError(f"Telegram upload failed: {data}")


def state_file(state_dir: Path, project: Project, target: Target) -> Path:
    return state_dir / f"{project.key}_{target.key}.json"


def read_state(path: Path) -> dict[str, object]:
    if not path.exists():
        return {}
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return {}


def write_state(path: Path, payload: dict[str, object]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    tmp.replace(path)


def list_projects() -> None:
    print("Projects:")
    for key, project in PROJECTS.items():
        print(f"  {key:10s} repo={project.repo:28s} targets={','.join(project.supports)}")
    print("\nTargets:")
    for key in TARGETS:
        print(f"  {key}")


def check_tools(project: Project) -> None:
    need = ["git"]
    if project.kind == "go":
        need.append("go")
    elif project.kind == "adguardhome":
        need += ["make", "go", "node", "npm"]
    elif project.kind in {"momogram", "v2rayng"}:
        need += ["java"]
    missing = [tool for tool in need if shutil.which(tool) is None]
    if missing:
        raise BuildError(f"missing required tool(s): {', '.join(missing)}")


def check_project_prereqs(project: Project) -> None:
    check_tools(project)
    if project.kind in {"momogram", "v2rayng"} and not (
        os.environ.get("ANDROID_HOME") or os.environ.get("ANDROID_SDK_ROOT")
    ):
        raise BuildError("ANDROID_HOME or ANDROID_SDK_ROOT must be set for Android builds")


def target_uploaded(state_dir: Path, project: Project, target: Target, tag: str) -> bool:
    previous = read_state(state_file(state_dir, project, target))
    return previous.get("tag") == tag and previous.get("target") == target.key


def target_caption(project: Project, target: Target, tag: str, commit: str) -> str:
    return f"{project.key} {tag} {target.key}\nrepo: {project.repo}\ncommit: {commit[:12]}"


def mark_target_uploaded(
    state_dir: Path,
    project: Project,
    target: Target,
    tag: str,
    commit: str,
    packages: list[Path],
) -> None:
    write_state(
        state_file(state_dir, project, target),
        {
            "project": project.key,
            "repo": project.repo,
            "tag": tag,
            "target": target.key,
            "commit": commit,
            "uploaded_at": utc_now(),
            "files": [package.name for package in packages],
        },
    )


def build_upload_one_target(
    project: Project,
    target: Target,
    tag: str,
    source_dir: Path,
    dist_dir: Path,
    patch_dir: Path,
    log_file: Path,
    state_dir: Path,
    no_upload: bool,
) -> None:
    dist_dir.mkdir(parents=True, exist_ok=True)
    commit = source_commit(source_dir)
    artifacts = build_project(project, target, source_dir, dist_dir, patch_dir, log_file)
    metadata = write_metadata(dist_dir, project, target, tag, commit)
    packages = package_artifacts(project, target, tag, dist_dir, artifacts, metadata)
    caption = target_caption(project, target, tag, commit)
    if no_upload:
        print("built packages:")
        for package in packages:
            print(f"  {package}")
    else:
        for package in packages:
            print(f"uploading {package.name} ({package.stat().st_size} bytes)")
            telegram_upload(package, caption)
        mark_target_uploaded(state_dir, project, target, tag, commit, packages)


def single_build(args: argparse.Namespace, project: Project, target: Target) -> int:
    args.log_dir.mkdir(parents=True, exist_ok=True)
    log_file = args.log_dir / f"{safe_name(project.key)}-{safe_name(target.key)}-{int(time.time())}.log"

    release = latest_release(project)
    tag = str(release["tag_name"])
    previous = read_state(state_file(args.state_dir, project, target))
    print(f"latest stable release: {project.repo} {tag}")
    print(f"target: {target.key}")
    if args.check_only:
        print(f"state: {json.dumps(previous, ensure_ascii=False)}")
        return 0
    if not args.force and previous.get("tag") == tag and previous.get("target") == target.key:
        print("already uploaded; use --force to rebuild")
        return 0
    check_project_prereqs(project)

    tmp_parent = args.work_dir
    if tmp_parent:
        tmp_parent.mkdir(parents=True, exist_ok=True)
    work_dir = Path(tempfile.mkdtemp(prefix=f"codex-build-{project.key}-{target.key}-", dir=tmp_parent))
    source_dir = work_dir / "src"
    dist_dir = work_dir / "dist"
    failed = True
    try:
        clone_source(project, tag, source_dir, log_file)
        build_upload_one_target(
            project,
            target,
            tag,
            source_dir,
            dist_dir,
            args.patch_dir,
            log_file,
            args.state_dir,
            args.no_upload,
        )
        failed = False
        return 0
    finally:
        if failed and args.keep_on_failure:
            print(f"kept failed work directory: {work_dir}")
        else:
            shutil.rmtree(work_dir, ignore_errors=True)
            print(f"cleaned work directory: {work_dir}")


def poll_all_once(args: argparse.Namespace) -> int:
    args.log_dir.mkdir(parents=True, exist_ok=True)
    tmp_parent = args.work_dir
    if tmp_parent:
        tmp_parent.mkdir(parents=True, exist_ok=True)

    failures: list[str] = []
    built_count = 0
    skipped_count = 0

    for project in PROJECTS.values():
        try:
            release = latest_release(project)
            tag = str(release["tag_name"])
        except BuildError as exc:
            failures.append(f"{project.key}: release check failed: {exc}")
            if args.stop_on_error:
                break
            continue

        targets = [TARGETS[key] for key in project.supports]
        pending = [target for target in targets if args.force or not target_uploaded(args.state_dir, project, target, tag)]
        if not pending:
            print(f"{project.key}: {tag} all targets already uploaded")
            skipped_count += len(targets)
            continue

        print(f"{project.key}: {tag} pending targets: {', '.join(t.key for t in pending)}")
        try:
            check_project_prereqs(project)
        except BuildError as exc:
            message = f"{project.key}: prerequisite check failed: {exc}"
            print(f"error: {message}", file=sys.stderr)
            failures.append(message)
            if args.stop_on_error:
                break
            continue

        log_file = args.log_dir / f"{safe_name(project.key)}-{safe_name(tag)}-{int(time.time())}.log"
        work_dir = Path(tempfile.mkdtemp(prefix=f"codex-poll-{project.key}-", dir=tmp_parent))
        source_dir = work_dir / "src"
        project_failed = False
        try:
            clone_source(project, tag, source_dir, log_file)
            for target in pending:
                dist_dir = work_dir / "dist" / target.key
                target_failed = True
                try:
                    print(f"{project.key}: building {target.key}")
                    build_upload_one_target(
                        project,
                        target,
                        tag,
                        source_dir,
                        dist_dir,
                        args.patch_dir,
                        log_file,
                        args.state_dir,
                        args.no_upload,
                    )
                    built_count += 1
                    target_failed = False
                except BuildError as exc:
                    message = f"{project.key} {target.key}: {exc}"
                    print(f"error: {message}", file=sys.stderr)
                    failures.append(message)
                    project_failed = True
                    if args.stop_on_error:
                        raise
                finally:
                    if target_failed and args.keep_on_failure:
                        print(f"kept failed target output: {dist_dir}")
                    else:
                        shutil.rmtree(dist_dir, ignore_errors=True)
                        print(f"cleaned target output: {dist_dir}")
                if target_failed and args.stop_on_error:
                    break
                if not target_failed:
                    reset_project_build_outputs(project, source_dir, log_file)
        except BuildError:
            if args.stop_on_error:
                raise
        finally:
            if project_failed and args.keep_on_failure:
                print(f"kept failed source work directory: {work_dir}")
            else:
                shutil.rmtree(work_dir, ignore_errors=True)
                print(f"cleaned source work directory: {work_dir}")

        if failures and args.stop_on_error:
            break

    print(f"poll summary: built={built_count} skipped={skipped_count} failures={len(failures)}")
    for failure in failures:
        print(f"failure: {failure}", file=sys.stderr)
    return 1 if failures else 0


def poll_all_check(args: argparse.Namespace) -> int:
    failures = 0
    for project in PROJECTS.values():
        try:
            release = latest_release(project)
            tag = str(release["tag_name"])
        except BuildError as exc:
            failures += 1
            print(f"{project.key}: release check failed: {exc}", file=sys.stderr)
            continue
        pending: list[str] = []
        uploaded: list[str] = []
        for target_key in project.supports:
            target = TARGETS[target_key]
            if target_uploaded(args.state_dir, project, target, tag):
                uploaded.append(target.key)
            else:
                pending.append(target.key)
        print(f"{project.key}: {tag}")
        print(f"  pending: {', '.join(pending) if pending else 'none'}")
        print(f"  uploaded: {', '.join(uploaded) if uploaded else 'none'}")
    return 1 if failures else 0


def parse_args(argv: Iterable[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project", help="one project key, e.g. xray or v2rayng")
    parser.add_argument("--target", help="one target, e.g. linux-amd64, windows-amd64, android-arm64")
    parser.add_argument("--poll-all", action="store_true", help="check every project/target and build pending artifacts sequentially")
    parser.add_argument("--interval", type=int, default=0, help="with --poll-all, repeat forever every N seconds")
    parser.add_argument("--stop-on-error", action="store_true", help="stop poll-all after the first failed project or target")
    parser.add_argument("--state-dir", type=Path, default=DEFAULT_STATE_DIR)
    parser.add_argument("--log-dir", type=Path, default=DEFAULT_LOG_DIR)
    parser.add_argument("--patch-dir", type=Path, default=DEFAULT_PATCH_DIR)
    parser.add_argument("--env-file", type=Path, default=None, help="load secrets/config from this env file")
    parser.add_argument("--work-dir", type=Path, default=None, help="parent directory for temporary source/build work")
    parser.add_argument("--force", action="store_true", help="build even if this tag/target was already uploaded")
    parser.add_argument("--check-only", action="store_true", help="only print latest stable release and state")
    parser.add_argument("--no-upload", action="store_true", help="build but do not upload or mark state")
    parser.add_argument("--keep-on-failure", action="store_true", help="keep temporary work directory if build fails")
    parser.add_argument("--list", action="store_true", help="list configured projects and targets")
    return parser.parse_args(list(argv))


def main(argv: Iterable[str]) -> int:
    args = parse_args(argv)
    load_environment(args.env_file)
    if args.list:
        list_projects()
        return 0

    lock_path = args.state_dir / ".build.lock"
    with RunLock(lock_path):
        if args.poll_all:
            if args.project or args.target:
                raise BuildError("--poll-all cannot be combined with --project or --target")
            if args.check_only:
                return poll_all_check(args)
            if args.interval > 0:
                while True:
                    code = poll_all_once(args)
                    if code and args.stop_on_error:
                        return code
                    print(f"sleeping {args.interval} seconds before next poll")
                    time.sleep(args.interval)
            return poll_all_once(args)

        if not args.project or not args.target:
            raise BuildError("--project and --target are required unless --list or --poll-all is used")
        project = PROJECTS[normalize_project(args.project)]
        target = TARGETS[normalize_target(args.target)]
        if target.key not in project.supports:
            raise BuildError(f"{project.key} does not support {target.key}; supported: {', '.join(project.supports)}")
        return single_build(args, project, target)


if __name__ == "__main__":
    try:
        raise SystemExit(main(sys.argv[1:]))
    except BuildError as exc:
        print(f"error: {exc}", file=sys.stderr)
        raise SystemExit(1)
