#!/usr/bin/env python3
from __future__ import annotations

import re
import sys
from pathlib import Path


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


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: v2rayng-no-ads.py <source-dir>", file=sys.stderr)
        return 2

    repo = Path(sys.argv[1]).resolve()
    menu = repo / "V2rayNG" / "app" / "src" / "main" / "res" / "menu" / "menu_drawer.xml"
    config = repo / "V2rayNG" / "app" / "src" / "main" / "java" / "com" / "v2ray" / "ang" / "AppConfig.kt"
    gradle_properties = repo / "V2rayNG" / "gradle.properties"

    if not menu.is_file():
        print(f"menu file not found: {menu}", file=sys.stderr)
        return 1
    if not config.is_file():
        print(f"config file not found: {config}", file=sys.stderr)
        return 1

    text = menu.read_text(encoding="utf-8")
    text = re.sub(
        r'(<item\s+android:id="@\+id/promotion"(?:(?!/>).)*?)\s*/>',
        lambda m: m.group(1) + '\n            android:visible="false" />'
        if "android:visible=" not in m.group(0)
        else m.group(0),
        text,
        flags=re.S,
    )
    menu.write_text(text, encoding="utf-8")

    text = config.read_text(encoding="utf-8")
    text = re.sub(
        r'const val APP_PROMOTION_URL = "[^"]*"',
        'const val APP_PROMOTION_URL = ""',
        text,
    )
    config.write_text(text, encoding="utf-8")

    upsert_gradle_property(gradle_properties, "ABI_FILTERS", "arm64-v8a")
    upsert_gradle_property(
        gradle_properties,
        "org.gradle.jvmargs",
        "-Xmx2048m -XX:MaxMetaspaceSize=1024m -Dfile.encoding=UTF-8",
    )
    upsert_gradle_property(gradle_properties, "org.gradle.daemon", "false")
    upsert_gradle_property(gradle_properties, "org.gradle.workers.max", "2")
    upsert_gradle_property(gradle_properties, "kotlin.daemon.jvmargs", "-Xmx1536m")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
