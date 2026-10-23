#!/usr/bin/env python3
"""按清单下载并校验预编译原生依赖，或在依赖工作流中打包。"""
import hashlib
import json
import os
from pathlib import Path
import sys
import tarfile
import tempfile
import urllib.error
import urllib.request

SCRIPTS = Path(__file__).resolve().parent
MANIFEST = SCRIPTS / "native-deps.json"
# 这些文件决定依赖包内容；任一变化即视为旧包过期。
INPUTS = {
    "qt": {
        "linux": ["setup-linux-qt.sh", "qt-build-paths.py"],
        "darwin": ["setup-macos-qt.sh", "qt-build-paths.py"],
        "windows": ["setup-windows-qt.sh", "qt-build-paths.py"],
    },
    "tts": {goos: ["build-tts-native.py", "tts-native/sources.json", "tts-native/arcana-webgpu.h"]
            for goos in ("linux", "darwin", "windows")},
}
# Qt 安装前缀中的本机构建标记，不随依赖包分发。
LOCAL_MARKERS = {".complete", ".arcana-owned"}
# 退出码 3 通知调用脚本改为从源码构建，与其他失败区分。
EXIT_BUILD_FROM_SOURCE = 3
USAGE = f"""Usage: native-deps.py COMMAND ...

resolve COMPONENT GOOS GOARCH          Print the pack SHA256; exit {EXIT_BUILD_FROM_SOURCE} to build from source.
fetch COMPONENT GOOS GOARCH DEST       Download, verify, and extract into DEST; exit {EXIT_BUILD_FROM_SOURCE} to build from source.
status                                 List manifest packs and whether local build inputs still match.
pack COMPONENT GOOS GOARCH SOURCE OUT  Archive SOURCE as OUT/COMPONENT-GOOS-GOARCH.tar.xz.
manifest URL DIR                       Write DIR/native-deps.json for the packs in DIR published under URL.

COMPONENT is qt or tts. NATIVE_DEPS selects auto (default), prebuilt, or source:
auto builds from source when the pack is missing or older than the local build scripts,
prebuilt makes that an error, and source never downloads packs.
Downloads are cached in $XDG_CACHE_HOME/arcana-native-deps (default: ~/.cache)."""


def inputs(component, goos):
    digest = hashlib.sha256()
    for name in INPUTS[component][goos]:
        # Windows 检出可能转换换行，统一后再计算。
        data = (SCRIPTS / name).read_bytes().replace(b"\r\n", b"\n")
        digest.update(f"{name}\0{len(data)}\0".encode() + data)
    return digest.hexdigest()


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1 << 20), b""):
            digest.update(chunk)
    return digest.hexdigest()


def load_manifest():
    if not MANIFEST.exists():
        return None
    return json.loads(MANIFEST.read_text(encoding="utf-8"))


def resolve(component, goos, goarch):
    mode = os.environ.get("NATIVE_DEPS") or "auto"
    if mode not in ("auto", "prebuilt", "source"):
        raise SystemExit("NATIVE_DEPS must be auto, prebuilt, or source")
    if mode == "source":
        return None
    name = f"{component}-{goos}-{goarch}"
    manifest = load_manifest()
    spec = manifest and manifest["packs"].get(name)
    if not spec:
        problem = f"No prebuilt {name} in scripts/native-deps.json"
    elif spec["inputs"] != inputs(component, goos):
        problem = f"Prebuilt {name} predates the local build scripts"
    else:
        return name, manifest["url"] + name + ".tar.xz", spec
    if mode == "prebuilt":
        raise SystemExit(f"{problem}; publish new native dependencies or set NATIVE_DEPS=source")
    print(f"{problem}; building from source.", file=sys.stderr)
    return None


def download(url, spec):
    root = Path(os.environ.get("XDG_CACHE_HOME") or Path.home() / ".cache") / "arcana-native-deps"
    path = root / f"{spec['sha256']}.tar.xz"
    if path.is_file() and sha256(path) == spec["sha256"]:
        return path
    root.mkdir(parents=True, exist_ok=True)
    total = spec["size"]
    print(f"Downloading {url} ({total / 1048576:.1f} MiB)", file=sys.stderr, flush=True)
    digest = hashlib.sha256()
    received = reported = 0
    descriptor, part = tempfile.mkstemp(dir=root, suffix=".part")
    try:
        with os.fdopen(descriptor, "wb") as temp, urllib.request.urlopen(url, timeout=60) as response:
            for chunk in iter(lambda: response.read(1 << 20), b""):
                temp.write(chunk)
                digest.update(chunk)
                received += len(chunk)
                # 按四分之一进度输出，慢速网络下也能看出下载仍在进行。
                if total and received * 4 // total > reported:
                    reported = received * 4 // total
                    print(f"  {min(received * 100 // total, 100)}%", file=sys.stderr, flush=True)
        if digest.hexdigest() != spec["sha256"]:
            raise SystemExit(f"SHA256 mismatch: {url}")
        os.replace(part, path)
    except (urllib.error.URLError, TimeoutError, ConnectionError) as error:
        # 自动模式不静默改为源码构建：Qt 源码构建耗时很长，应由用户决定。
        raise SystemExit(f"Download failed: {url}: {error}\n"
                         "Retry later, or set NATIVE_DEPS=source to build locally.") from None
    finally:
        if os.path.exists(part):
            os.unlink(part)
    return path


def fetch(component, goos, goarch, dest):
    if not dest.is_dir():
        raise SystemExit(f"Destination is not a directory: {dest}")
    resolved = resolve(component, goos, goarch)
    if resolved is None:
        raise SystemExit(EXIT_BUILD_FROM_SOURCE)
    name, url, spec = resolved
    archive = download(url, spec)
    with tarfile.open(archive) as stream:
        # 内容已按清单摘要校验；新版 Python 另外限制越界路径和特殊文件。
        if hasattr(tarfile, "data_filter"):
            stream.extractall(dest, filter="data")
        else:
            stream.extractall(dest)
    print(f"Using prebuilt {name} ({spec['sha256'][:12]})", file=sys.stderr)


def status():
    manifest = load_manifest()
    if manifest is None:
        print("No scripts/native-deps.json; native dependencies build from source.")
        return
    print(manifest["url"])
    for name, spec in sorted(manifest["packs"].items()):
        component, goos, _ = name.split("-")
        print(f"{name}\t{'current' if spec['inputs'] == inputs(component, goos) else 'outdated'}")


def pack(component, goos, goarch, source, out):
    if not source.is_dir() or not any(source.iterdir()):
        raise SystemExit(f"Nothing to pack in {source}")
    out.mkdir(parents=True, exist_ok=True)
    target = out / f"{component}-{goos}-{goarch}.tar.xz"
    with tarfile.open(target, "w:xz") as archive:
        for path in sorted(source.iterdir()):
            if path.name not in LOCAL_MARKERS:
                archive.add(path, arcname=path.name)
    print(target)


def manifest(url, directory):
    packs = {}
    for path in sorted(directory.glob("*.tar.xz")):
        name = path.name.removesuffix(".tar.xz")
        component, goos, _ = name.split("-")
        packs[name] = {"inputs": inputs(component, goos), "sha256": sha256(path), "size": path.stat().st_size}
    if not packs:
        raise SystemExit(f"No packs in {directory}")
    target = directory / "native-deps.json"
    target.write_text(json.dumps({"url": url.rstrip("/") + "/", "packs": packs}, indent=2) + "\n", encoding="utf-8")
    print(target)


def main():
    command, *args = sys.argv[1:] or ["--help"]
    platform = args[:3]
    if command in ("resolve", "fetch", "pack") and len(args) >= 3:
        component, goos, _ = platform
        if goos not in INPUTS.get(component, {}):
            raise SystemExit(f"Unsupported native dependency: {component} for {goos}")
    if command == "resolve" and len(args) == 3:
        resolved = resolve(*platform)
        if resolved is None:
            raise SystemExit(EXIT_BUILD_FROM_SOURCE)
        print(resolved[2]["sha256"])
    elif command == "fetch" and len(args) == 4:
        fetch(*platform, Path(args[3]))
    elif command == "status" and not args:
        status()
    elif command == "pack" and len(args) == 5:
        pack(*platform, Path(args[3]), Path(args[4]))
    elif command == "manifest" and len(args) == 2:
        manifest(args[0], Path(args[1]))
    elif command in ("-h", "--help") and not args:
        print(USAGE)
    else:
        raise SystemExit(USAGE)


if __name__ == "__main__":
    main()
