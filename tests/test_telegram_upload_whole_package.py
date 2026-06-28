import os
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import build_release


class TelegramUploadWholePackageTest(unittest.TestCase):
    def test_large_package_is_rejected_instead_of_split(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            package = Path(tmp) / "large.apk"
            package.write_bytes(b"x" * 1_048_577)

            with mock.patch.dict(os.environ, {"TELEGRAM_MAX_UPLOAD_BYTES": "1048576"}):
                with mock.patch.object(build_release, "telegram_upload") as upload:
                    with self.assertRaises(build_release.BuildError) as raised:
                        build_release.telegram_upload_package(package, "caption")

            self.assertIn("TELEGRAM_OVERSIZE_MODE=split", str(raised.exception))
            upload.assert_not_called()
            self.assertEqual([path.name for path in Path(tmp).iterdir()], ["large.apk"])

    def test_large_package_can_be_split_when_enabled(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            package = Path(tmp) / "large.apk"
            package.write_bytes(b"x" * 1_048_577)

            with mock.patch.dict(
                os.environ,
                {"TELEGRAM_MAX_UPLOAD_BYTES": "1048576", "TELEGRAM_OVERSIZE_MODE": "split"},
            ):
                with mock.patch.object(build_release, "telegram_upload") as upload:
                    uploaded_names = build_release.telegram_upload_package(package, "caption")

            self.assertEqual(uploaded_names, ["large.apk.part01of02", "large.apk.part02of02"])
            self.assertEqual([call.args[0].name for call in upload.call_args_list], uploaded_names)
            self.assertEqual([path.name for path in Path(tmp).iterdir()], ["large.apk"])

    def test_package_is_uploaded_as_one_file_when_within_limit(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            package = Path(tmp) / "package.tar.gz"
            package.write_bytes(b"payload")

            with mock.patch.dict(os.environ, {"TELEGRAM_MAX_UPLOAD_BYTES": "1048576"}):
                with mock.patch.object(build_release, "telegram_upload") as upload:
                    uploaded_names = build_release.telegram_upload_package(package, "caption")

            self.assertEqual(uploaded_names, ["package.tar.gz"])
            upload.assert_called_once_with(package, "caption")

    def test_state_records_only_original_package_names(self) -> None:
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
            )

            state = build_release.read_state(build_release.state_file(state_dir, project, target))
            self.assertEqual(state["files"], ["large.apk"])
            self.assertNotIn("uploaded_files", state)
            self.assertEqual(state["uploads"][0]["files"], ["large.apk"])
            self.assertNotIn("uploaded_files", state["uploads"][0])

    def test_state_records_split_upload_part_names_separately(self) -> None:
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
