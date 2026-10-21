#!/usr/bin/env python3
"""仅在显式打包 RTMPSRT 组件时收集 libsrt 及其非系统动态依赖。"""
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

from runtime_files import publish, remove

MANIFEST = ".arcana-srt-files.json"


def output(*args):
    return subprocess.check_output(args, text=True, stderr=subprocess.STDOUT).strip()


def run(*args):
    subprocess.run(args, check=True)


def dependencies(source):
    if sys.platform == "darwin":
        result = []
        for line in output("otool", "-L", str(source)).splitlines()[1:]:
            name = line.strip().split(" (", 1)[0]
            if name.startswith(("/System/Library/", "/usr/lib/")):
                continue
            if name.startswith("@loader_path/"):
                path = source.parent / name.removeprefix("@loader_path/")
            elif name.startswith("@rpath/"):
                # Homebrew 与常见独立 libsrt 发行包把同级依赖置于 lib 目录。
                path = source.parent / name.removeprefix("@rpath/")
                if not path.exists():
                    raise SystemExit(f"Unresolved Mach-O dependency {name}; supply a relocatable SRT_LIBRARY")
            else:
                path = Path(name)
            if path.resolve() != source.resolve():
                result.append((name, path))
        return result
    if sys.platform.startswith("linux"):
        result = []
        for line in output("ldd", str(source)).splitlines():
            if "not found" in line:
                raise SystemExit(f"Unresolved dependency: {line.strip()}")
            match = re.match(r"\s*(\S+) => (\/\S+) ", line)
            if not match:
                continue
            name, path = match.groups()
            if re.match(r"^(ld-linux.*|ld64\.so.*|lib(c|m|pthread|dl|rt|resolv|util|anl|nss_[^.]+)\.so(\..*)?)$", name):
                continue
            result.append((name, Path(path)))
        return result
    if sys.platform == "win32":
        result = []
        system = Path(os.environ["SystemRoot"]) / "System32"
        for name in re.findall(r"DLL Name:\s*(\S+)", output(os.environ.get("OBJDUMP", "objdump"), "-p", str(source))):
            # Windows API 转发及系统组件不随应用分发。
            if name.lower().startswith(("api-ms-", "ext-ms-")):
                continue
            path = source.parent / name
            if path.exists():
                result.append((name, path))
            elif not (system / name).exists() or name.lower().startswith(("vcruntime", "msvcp", "concrt")):
                raise SystemExit(f"Missing DLL dependency {name} beside {source}")
        return result
    raise SystemExit(f"Unsupported SRT deployment platform: {sys.platform}")


def deploy(destination):
    source = Path(os.environ.get("SRT_LIBRARY", "")).resolve()
    notices = Path(os.environ.get("SRT_LICENSE_DIR", "")).resolve()
    if not os.environ.get("SRT_LIBRARY") or not source.is_file():
        raise SystemExit("Set SRT_LIBRARY to the native libsrt dynamic library")
    if not os.environ.get("SRT_LICENSE_DIR") or not (notices / "SOURCE.txt").is_file():
        raise SystemExit("Set SRT_LICENSE_DIR to complete libsrt/dependency licenses and SOURCE.txt")
    rootname = "srt.dll" if sys.platform == "win32" else "libsrt.dylib" if sys.platform == "darwin" else "libsrt.so"
    with tempfile.TemporaryDirectory(prefix="arcana-srt-") as temporary:
        root = Path(temporary)
        lib = root / "lib"
        lib.mkdir()
        queue = [(rootname, source)]
        copied = {}
        links = {}
        while queue:
            name, original = queue.pop()
            resolved = original.resolve(strict=True)
            if name in copied:
                if copied[name] != resolved:
                    raise SystemExit(f"Conflicting SRT dependency basename: {name}")
                continue
            if Path(name).name != name:
                raise SystemExit(f"Invalid dependency filename: {name}")
            copied[name] = resolved
            shutil.copy2(resolved, lib / name)
            (lib / name).chmod((lib / name).stat().st_mode | 0o200)
            links[name] = dependencies(original)
            for _, dependency in links[name]:
                queue.append((dependency.name, dependency))
        for name, items in links.items():
            path = str(lib / name)
            if sys.platform == "darwin":
                run("install_name_tool", "-id", "@loader_path/" + name, path)
                for old, dependency in items:
                    run("install_name_tool", "-change", old, "@loader_path/" + dependency.name, path)
                run("codesign", "--force", "--sign", "-", path)
            elif sys.platform.startswith("linux"):
                run("patchelf", "--set-rpath", "$ORIGIN", path)
        shutil.copytree(notices, root / "licenses/srt")
        publish(root, destination, MANIFEST)


if __name__ == "__main__":
    if len(sys.argv) == 3 and sys.argv[1] == "--remove":
        remove(Path(sys.argv[2]).resolve(), MANIFEST)
    elif len(sys.argv) == 2:
        deploy(Path(sys.argv[1]).resolve())
    else:
        raise SystemExit(f"Usage: {sys.argv[0]} [--remove] OUTPUT_DIR")
