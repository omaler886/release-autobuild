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


def patch_default_abis(path: Path, default_line: str, replacement_line: str) -> bool:
    if not path.is_file():
        return False
    text = path.read_text(encoding="utf-8")
    if replacement_line in text:
        return False
    if default_line not in text:
        return False
    path.write_text(text.replace(default_line, replacement_line, 1), encoding="utf-8")
    return True


def patch_dav1d_meson_wipe(path: Path) -> bool:
    if not path.is_file():
        return False
    text = path.read_text(encoding="utf-8")
    patched = text.replace("\n  --wipe \\\n", "\n")
    if patched == text:
        return False
    path.write_text(patched, encoding="utf-8")
    return True


def patch_dav1d_arm64_only(path: Path) -> bool:
    if not path.is_file():
        return False
    text = path.read_text(encoding="utf-8")
    if "builddir-armv7" not in text and "builddir-x86" not in text:
        return False
    arm64_done = "ninja -C builddir-arm64 install\n"
    popd = "\npopd\n"
    arm64_end = text.find(arm64_done)
    popd_start = text.rfind(popd)
    if arm64_end == -1 or popd_start == -1 or popd_start <= arm64_end:
        return False
    patched = text[: arm64_end + len(arm64_done)] + text[popd_start:]
    path.write_text(patched, encoding="utf-8")
    return True


def patch_gradle_native_jobs(path: Path) -> bool:
    if not path.is_file():
        return False
    text = path.read_text(encoding="utf-8")
    old_condition = 'task.name.startsWith("externalNativeBuild")) {'
    new_condition = 'task.name.startsWith("externalNativeBuild") || task.name.startsWith("buildCMake")) {'
    if old_condition in text:
        path.write_text(text.replace(old_condition, new_condition, 1), encoding="utf-8")
        return True
    if "android_gradle_build.json:force-ninja-j1" in text:
        return False
    marker = "    tasks.all { task ->\n"
    if marker not in text:
        return False
    injected = """    tasks.all { task ->
        if (task.name.startsWith("externalNativeBuild") || task.name.startsWith("buildCMake")) {
            task.doFirst {
                def nativeBuildFiles = fileTree("${projectDir}/.cxx") {
                    include "**/android_gradle_build.json"
                    include "**/android_gradle_build_mini.json"
                }
                nativeBuildFiles.each { nativeBuildFile ->
                    def json = new groovy.json.JsonSlurper().parse(nativeBuildFile)
                    def command = json.buildTargetsCommandComponents
                    if (command instanceof List && !command.contains("-j1")) {
                        def targetIndex = command.indexOf("{LIST_OF_TARGETS_TO_BUILD}")
                        command.add(targetIndex >= 0 ? targetIndex : command.size(), "-j1")
                        nativeBuildFile.text = groovy.json.JsonOutput.prettyPrint(groovy.json.JsonOutput.toJson(json))
                    }
                }
            }
        }
        // android_gradle_build.json:force-ninja-j1
"""
    path.write_text(text.replace(marker, injected, 1), encoding="utf-8")
    return True


def patch_gradle_native_target(path: Path) -> bool:
    if not path.is_file():
        return False
    text = path.read_text(encoding="utf-8")
    changed = False

    split_old = """            } else if (!archs.contains(nativeTarget.toLowerCase())) {
                enable = false
                universalApk = false
            }
"""
    split_new = """            } else if (archs.contains(nativeTarget.toLowerCase())) {
                enable = true
                reset()
                include nativeTarget
                universalApk = false
            } else {
                enable = false
                universalApk = false
            }
"""
    if split_new not in text and split_old in text:
        text = text.replace(split_old, split_new, 1)
        changed = True

    ndk_filter_block = """        if (archs.contains(nativeTarget.toLowerCase())) {
            ndk {
                abiFilters nativeTarget
            }
        }

"""
    if ndk_filter_block in text:
        text = text.replace(ndk_filter_block, "", 1)
        changed = True

    native_build_old = """        externalNativeBuild {
            cmake {
                version = "3.10.2"
                arguments '-DANDROID_STL=c++_static', '-DANDROID_PLATFORM=android-21' //, '-DANDROID_SUPPORT_FLEXIBLE_PAGE_SIZES=ON'
            }
        }
"""
    native_build_new = """        externalNativeBuild {
            cmake {
                version = "3.10.2"
                arguments '-DANDROID_STL=c++_static', '-DANDROID_PLATFORM=android-21' //, '-DANDROID_SUPPORT_FLEXIBLE_PAGE_SIZES=ON'
                if (archs.contains(nativeTarget.toLowerCase())) {
                    abiFilters nativeTarget
                }
            }
        }
"""
    if "abiFilters nativeTarget" not in text and native_build_old in text:
        text = text.replace(native_build_old, native_build_new, 1)
        changed = True

    if changed:
        path.write_text(text, encoding="utf-8")
    return changed


def patch_gradle_release_lint(path: Path) -> bool:
    """Disable release lint because CI compiles are blocked by lint heap and bundle failures."""
    if not path.is_file():
        return False
    text = path.read_text(encoding="utf-8")
    changed = False

    lint_block = """    lint {
        checkReleaseBuilds false
        abortOnError false
    }

"""
    if "checkReleaseBuilds false" not in text:
        marker = "android {\n"
        if marker not in text:
            return False
        text = text.replace(marker, marker + lint_block, 1)
        changed = True

    task_block = """
tasks.configureEach { task ->
    if (task.name.startsWith("lintVital")) {
        task.enabled = false
    }
}
"""
    if "task.name.startsWith(\"lintVital\")" not in text:
        text = text.rstrip() + "\n" + task_block
        changed = True

    if changed:
        path.write_text(text, encoding="utf-8")
    return changed


def main() -> int:
    source_dir = Path(sys.argv[1]).resolve()
    changed: list[str] = []

    libs_init = source_dir / "bin" / "init" / "libs.sh"
    if add_set_e(libs_init):
        changed.append(str(libs_init.relative_to(source_dir)))

    ffmpeg_script = source_dir / "TMessagesProj" / "jni" / "build_ffmpeg_clang.sh"
    if patch_ffmpeg_pkg_config(ffmpeg_script):
        changed.append(str(ffmpeg_script.relative_to(source_dir)))

    abi_defaults = {
        source_dir / "TMessagesProj" / "jni" / "build_libvpx_clang.sh": (
            "\tbuild x86_64 x86 arm arm64",
            "\tbuild arm64",
        ),
        ffmpeg_script: (
            "\tbuild x86_64 arm64 arm x86",
            "\tbuild arm64",
        ),
        source_dir / "TMessagesProj" / "jni" / "build_boringssl.sh": (
            "\tbuild x86_64 arm64 arm x86",
            "\tbuild arm64",
        ),
    }
    for path, (default_line, replacement_line) in abi_defaults.items():
        if patch_default_abis(path, default_line, replacement_line):
            changed.append(f"{path.relative_to(source_dir)}:arm64-only")

    dav1d_script = source_dir / "TMessagesProj" / "jni" / "build_dav1d.sh"
    if patch_dav1d_meson_wipe(dav1d_script):
        changed.append(f"{dav1d_script.relative_to(source_dir)}:meson-setup")
    if patch_dav1d_arm64_only(dav1d_script):
        changed.append(f"{dav1d_script.relative_to(source_dir)}:arm64-only")

    tmessages_gradle = source_dir / "TMessagesProj" / "build.gradle"
    if patch_gradle_native_target(tmessages_gradle):
        changed.append(f"{tmessages_gradle.relative_to(source_dir)}:arm64-filter")
    if patch_gradle_native_jobs(tmessages_gradle):
        changed.append(f"{tmessages_gradle.relative_to(source_dir)}:ninja-j1")
    if patch_gradle_release_lint(tmessages_gradle):
        changed.append(f"{tmessages_gradle.relative_to(source_dir)}:release-lint")

    gradle_properties = source_dir / "gradle.properties"
    gradle_updates = {
        "org.gradle.jvmargs": "-Xmx1536m -XX:MaxMetaspaceSize=768m -Dfile.encoding=UTF-8",
        "org.gradle.daemon": "false",
        "org.gradle.parallel": "false",
        "org.gradle.workers.max": "1",
        "kotlin.daemon.jvmargs": "-Xmx1024m",
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
