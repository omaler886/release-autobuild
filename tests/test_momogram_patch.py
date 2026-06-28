import importlib.util
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("momogram_patch", ROOT / "patches" / "momogram.py")
assert SPEC is not None
momogram_patch = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(momogram_patch)


class MomogramPatchTest(unittest.TestCase):
    def test_gradle_native_target_uses_split_and_cmake_filters_without_ndk_filter(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            gradle_file = Path(tmp) / "build.gradle"
            gradle_file.write_text(
                """android {
    splits {
        abi {
            if (nativeTarget.isBlank()) {
                enable = true
                reset()
                include 'armeabi-v7a', 'arm64-v8a', 'x86', 'x86_64'
                universalApk = false
            } else if (nativeTarget.toLowerCase().equals("universal")) {
                enable = false
                universalApk = true
            } else if (!archs.contains(nativeTarget.toLowerCase())) {
                enable = false
                universalApk = false
            }
        }
    }

    defaultConfig {
        if (archs.contains(nativeTarget.toLowerCase())) {
            ndk {
                abiFilters nativeTarget
            }
        }

        externalNativeBuild {
            cmake {
                version = "3.10.2"
                arguments '-DANDROID_STL=c++_static', '-DANDROID_PLATFORM=android-21' //, '-DANDROID_SUPPORT_FLEXIBLE_PAGE_SIZES=ON'
            }
        }
    }
}
""",
                encoding="utf-8",
            )

            self.assertTrue(momogram_patch.patch_gradle_native_target(gradle_file))
            patched = gradle_file.read_text(encoding="utf-8")
            self.assertFalse(momogram_patch.patch_gradle_native_target(gradle_file))

        self.assertIn("include nativeTarget", patched)
        self.assertIn("abiFilters nativeTarget", patched)
        self.assertNotIn("ndk {\n                abiFilters nativeTarget", patched)


if __name__ == "__main__":
    unittest.main()
