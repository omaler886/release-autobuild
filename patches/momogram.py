#!/usr/bin/env python3
from __future__ import annotations

import sys
from pathlib import Path


MARKER = 'echo "Configuring..."\n\n\t./configure \\\n'
INJECTED = (
    'echo "Configuring..."\n\n'
    '\tPKG_CONFIG_PATH="$(pwd)/../dav1d/build/${CPU}/lib/pkgconfig${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}"\n'
    '\texport PKG_CONFIG_PATH\n\n'
    '\t./configure \\\n'
)


def upsert_gradle_property(path: Path, name: str, value: str) -> bool:
    if not path.is_file():
        return False
    lines = path.read_text(encoding="utf-8").splitlines()
    target = f"{name}={value}"
    changed = False
    for index, line in enumerate(lines):
        if line.startswith(f"{name}="):
            if line != target:
                lines[index] = target
                changed = True
            break
    else:
        lines.append(target)
        changed = True
    if changed:
        path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return changed


def add_set_e(path: Path) -> bool:
    if not path.is_file():
        return False
    lines = path.read_text(encoding="utf-8").splitlines()
    if any(line.strip() == "set -e" for line in lines[:5]):
        return False
    if not lines or lines[0] != "#!/bin/bash":
        return False
    lines.insert(1, "set -e")
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return True


def patch_ffmpeg_pkg_config(path: Path) -> bool:
    if not path.is_file():
        return False
    text = path.read_text(encoding="utf-8")
    if 'PKG_CONFIG_PATH="$(pwd)/../dav1d/build/${CPU}/lib/pkgconfig${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}"' in text:
        return False
    if MARKER not in text:
        return False
    path.write_text(text.replace(MARKER, INJECTED, 1), encoding="utf-8")
    return True


def main() -> int:
    source_dir = Path(sys.argv[1]).resolve()
    changed: list[str] = []

    libs_init = source_dir / "bin" / "init" / "libs.sh"
    if add_set_e(libs_init):
        changed.append(str(libs_init.relative_to(source_dir)))

    ffmpeg_script = source_dir / "TMessagesProj" / "jni" / "build_ffmpeg_clang.sh"
    if patch_ffmpeg_pkg_config(ffmpeg_script):
        changed.append(str(ffmpeg_script.relative_to(source_dir)))

    gradle_properties = source_dir / "gradle.properties"
    gradle_updates = {
        "org.gradle.jvmargs": "-Xmx2048m -XX:MaxMetaspaceSize=1024m -Dfile.encoding=UTF-8",
        "org.gradle.daemon": "false",
        "org.gradle.workers.max": "2",
        "kotlin.daemon.jvmargs": "-Xmx1536m",
    }
    for name, value in gradle_updates.items():
        if upsert_gradle_property(gradle_properties, name, value):
            changed.append(f"{gradle_properties.relative_to(source_dir)}:{name}")

    if changed:
        print("momogram patch hook updated:")
        for item in changed:
            print(f"  {item}")
    else:
        print("momogram patch hook: no changes needed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
