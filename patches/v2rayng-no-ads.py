#!/usr/bin/env python3
from __future__ import annotations

import re
import sys
from pathlib import Path


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: v2rayng-no-ads.py <source-dir>", file=sys.stderr)
        return 2

    repo = Path(sys.argv[1]).resolve()
    menu = repo / "V2rayNG" / "app" / "src" / "main" / "res" / "menu" / "menu_drawer.xml"
    config = repo / "V2rayNG" / "app" / "src" / "main" / "java" / "com" / "v2ray" / "ang" / "AppConfig.kt"

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
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
