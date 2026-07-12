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
    def test_gradle_heap_supports_r8_release_build(self) -> None:
        """Verify the generated Gradle limits cover observed R8 memory pressure.

        Args:
            None.

        Returns:
            None.
        """
        with tempfile.TemporaryDirectory() as tmp:
            properties_file = Path(tmp) / "gradle.properties"
            properties_file.write_text("org.gradle.jvmargs=-Xmx1536m\n", encoding="utf-8")

            changed = momogram_patch.upsert_gradle_property(
                properties_file,
                "org.gradle.jvmargs",
                "-Xmx4096m -XX:MaxMetaspaceSize=1024m -Dfile.encoding=UTF-8",
            )

            self.assertTrue(changed)
            self.assertIn("-Xmx4096m", properties_file.read_text(encoding="utf-8"))

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

    def test_gradle_release_lint_is_disabled(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            gradle_file = Path(tmp) / "build.gradle"
            gradle_file.write_text(
                """plugins {
    id 'com.android.application'
}

android {
    namespace 'momo.gram'
}
""",
                encoding="utf-8",
            )

            self.assertTrue(momogram_patch.patch_gradle_release_lint(gradle_file))
            patched = gradle_file.read_text(encoding="utf-8")
            self.assertFalse(momogram_patch.patch_gradle_release_lint(gradle_file))

        self.assertIn("checkReleaseBuilds false", patched)
        self.assertIn("abortOnError false", patched)
        self.assertIn('task.name.startsWith("lintVital")', patched)


if __name__ == "__main__":
    unittest.main()
