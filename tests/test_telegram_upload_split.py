import os
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import build_release


class TelegramUploadSplitTest(unittest.TestCase):
    def test_large_package_is_split_before_upload(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            package = Path(tmp) / "large.apk"
            payload = (b"abc123" * 200_000)[:1_150_000]
            package.write_bytes(payload)

            uploads: list[tuple[str, bytes, str]] = []

            def fake_upload(path: Path, caption: str) -> None:
                uploads.append((path.name, path.read_bytes(), caption))

            with mock.patch.dict(os.environ, {"TELEGRAM_MAX_UPLOAD_BYTES": "1048576"}):
                with mock.patch.object(build_release, "telegram_upload", fake_upload):
                    uploaded_files = build_release.telegram_upload_package(package, "caption")

            self.assertEqual(uploaded_files, ["large.apk.part01of02", "large.apk.part02of02"])
            self.assertEqual([item[0] for item in uploads], uploaded_files)
            self.assertEqual(b"".join(item[1] for item in uploads), payload)
            self.assertIn("part: 1/2", uploads[0][2])
            self.assertIn("part: 2/2", uploads[1][2])
            self.assertFalse(any(path.name.startswith("large-parts-") for path in Path(tmp).iterdir()))

    def test_state_keeps_original_and_uploaded_part_names(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            state_dir = Path(tmp)
            package = state_dir / "large.apk"
            package.write_bytes(b"apk")
            project = build_release.PROJECTS["momogram"]
            target = build_release.TARGETS["android-arm64"]

            build_release.mark_target_uploaded(
                state_dir,
                project,
                target,
                "v1.0.0",
                "0123456789abcdef",
                [package],
                ["large.apk.part01of02", "large.apk.part02of02"],
            )

            state = build_release.read_state(build_release.state_file(state_dir, project, target))
            self.assertEqual(state["files"], ["large.apk"])
            self.assertEqual(state["uploaded_files"], ["large.apk.part01of02", "large.apk.part02of02"])
            self.assertEqual(state["uploads"][0]["files"], ["large.apk"])
            self.assertEqual(state["uploads"][0]["uploaded_files"], ["large.apk.part01of02", "large.apk.part02of02"])


if __name__ == "__main__":
    unittest.main()
