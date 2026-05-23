import os
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import build_release


class GitPushAuthTest(unittest.TestCase):
    def test_github_authenticated_remote_uses_token_and_repository(self) -> None:
        with mock.patch.dict(
            os.environ,
            {
                "GITHUB_TOKEN": "tok:en/@value",
                "GITHUB_REPOSITORY": "omaler886/release-autobuild",
            },
            clear=True,
        ):
            remote, display_remote = build_release.github_authenticated_remote()

        self.assertEqual(
            remote,
            "https://x-access-token:tok%3Aen%2F%40value@github.com/omaler886/release-autobuild.git",
        )
        self.assertEqual(
            display_remote,
            "https://x-access-token:***@github.com/omaler886/release-autobuild.git",
        )

    def test_github_authenticated_remote_falls_back_to_origin_without_token(self) -> None:
        with mock.patch.dict(os.environ, {}, clear=True):
            self.assertEqual(build_release.github_authenticated_remote(), ("origin", "origin"))

    def test_run_logs_display_command_and_hides_real_command_on_failure(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            log_file = Path(tmp) / "run.log"
            secret_cmd = ["git", "push", "https://x-access-token:secret@github.com/example/repo.git"]
            display_cmd = ["git", "push", "https://x-access-token:***@github.com/example/repo.git"]

            with self.assertRaises(build_release.BuildError) as raised:
                build_release.run(secret_cmd, Path(tmp), log_file=log_file, display_cmd=display_cmd)

            self.assertIn("***", log_file.read_text(encoding="utf-8"))
            self.assertNotIn("secret", log_file.read_text(encoding="utf-8"))
            self.assertIn("***", str(raised.exception))
            self.assertNotIn("secret", str(raised.exception))


if __name__ == "__main__":
    unittest.main()
