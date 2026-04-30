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

    if changed:
        print("momogram patch hook updated:")
        for item in changed:
            print(f"  {item}")
    else:
        print("momogram patch hook: no changes needed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
