"""构建 Arcana 固定版本 sherpa 扩展；Python 仅在构建期使用。"""
import hashlib
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import tarfile
import urllib.request
import zipfile

ASSETS = Path(__file__).resolve().parent / "tts-native"
MANIFEST = json.loads((ASSETS / "sources.json").read_text())
RID = {("darwin", "arm64"): "osx-arm64", ("linux", "amd64"): "linux-x64",
       ("linux", "arm64"): "linux-arm64", ("windows", "amd64"): "win-x64",
       ("windows", "arm64"): "win-arm64"}


def download(name, cache):
    spec = MANIFEST[name]
    path = cache / name
    if not path.exists():
        with urllib.request.urlopen(spec["url"], timeout=120) as response:
            data = response.read()
        if hashlib.sha256(data).hexdigest() != spec["sha256"]:
            raise SystemExit(f"SHA256 mismatch: {name}")
        path.write_bytes(data)
    if hashlib.sha256(path.read_bytes()).hexdigest() != spec["sha256"]:
        raise SystemExit(f"SHA256 mismatch: {path}")
    return path


def replace(path, before, after, count=1):
    text = path.read_text()
    if text.count(before) != count:
        raise SystemExit(f"Pinned sherpa source no longer matches patch: {path}")
    path.write_text(text.replace(before, after))


def patch(source):
    # 两种模型都先注册插件，再创建 options/session；逆序析构保证插件最后卸载。
    for name in ("vits", "kokoro"):
        model = source / f"sherpa-onnx/csrc/offline-tts-{name}-model.cc"
        replace(model, '#include "sherpa-onnx/csrc/session.h"',
                '#include "sherpa-onnx/csrc/session.h"\n#include "sherpa-onnx/csrc/arcana-webgpu.h"')
        replace(model, '        sess_opts_(GetSessionOptions(config)),',
                '        webgpu_(env_, config),\n        sess_opts_(ArcanaWebGpu::Options(config)),', 2)
        replace(model, '        allocator_{} {',
                '        allocator_{} {\n    webgpu_.Configure(sess_opts_);', 2)
        replace(model, '  Ort::Env env_;', '  Ort::Env env_;\n  ArcanaWebGpu webgpu_;')
    shutil.copy2(ASSETS / "arcana-webgpu.h", source / "sherpa-onnx/csrc/arcana-webgpu.h")
    replace(source / "sherpa-onnx/csrc/version.cc", '"1.13.8"', '"1.13.8-arcana-webgpu2"')
    api = source / "sherpa-onnx/c-api/c-api.cc"
    replace(api, '''  SherpaOnnxOfflineTts *tts = new SherpaOnnxOfflineTts;

  tts->impl = std::make_unique<sherpa_onnx::OfflineTts>(tts_config);

  return tts;''', '''  try {
    auto tts = std::make_unique<SherpaOnnxOfflineTts>();
    tts->impl = std::make_unique<sherpa_onnx::OfflineTts>(tts_config);
    return tts.release();
  } catch (const std::exception &e) {
    SHERPA_ONNX_LOGE("Local TTS native initialization failed: %s", e.what());
    return nullptr;
  }''')
    replace(api, '  sherpa_onnx::GeneratedAudio audio = tts->impl->Generate(text, cfg, callback);', '''  sherpa_onnx::GeneratedAudio audio;
  try {
    audio = tts->impl->Generate(text, cfg, callback);
  } catch (const std::exception &e) {
    SHERPA_ONNX_LOGE("Local TTS native synthesis failed: %s", e.what());
    return nullptr;
  }''')


def source_tree(work):
    source = work / "source"
    # 每轮使用干净私有源码；绝不改动 Go module cache 或调用者提供的源码。
    if source.exists():
        shutil.rmtree(source)
    supplied = os.environ.get("TTS_SHERPA_SOURCE")
    if supplied:
        shutil.copytree(supplied, source, ignore=lambda directory, names: [
            name for name in names if (Path(directory) / name).is_symlink()])
        source.chmod(source.stat().st_mode | 0o700)
        for path in source.rglob("*"):
            path.chmod(path.stat().st_mode | (0o700 if path.is_dir() else 0o600))
    else:
        with tarfile.open(download("sherpa", work)) as archive:
            for member in archive.getmembers():
                # 上游含指向开发机的链接，原生构建不需要任何链接或设备文件。
                if not (member.isfile() or member.isdir()):
                    continue
                parts = Path(member.name).parts[1:]
                if not parts or ".." in parts:
                    continue
                target = source.joinpath(*parts)
                if member.isdir():
                    target.mkdir(parents=True, exist_ok=True)
                else:
                    target.parent.mkdir(parents=True, exist_ok=True)
                    with archive.extractfile(member) as stream, target.open("wb") as output:
                        shutil.copyfileobj(stream, output)
                    target.chmod(member.mode & 0o777)
                    os.utime(target, (member.mtime, member.mtime))
    patch(source)
    return source


def collect_notices(work, source, staging):
    target = staging / "licenses/tts/native-dependencies"
    roots = [source] + list((work / "build/_deps").glob("*-src"))
    dependencies = os.environ.get("TTS_NATIVE_DEPS_DIR")
    if dependencies:
        roots += list(Path(dependencies).iterdir())
    for root in roots:
        for path in root.rglob("*"):
            name = path.name.lower()
            if path.is_file() and (name.startswith(("license", "copying", "copyright", "notice")) or name == "thirdpartynotices.txt"):
                dest = target / root.name / path.relative_to(root)
                dest.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(path, dest)
    target.mkdir(parents=True, exist_ok=True)
    shutil.copy2(ASSETS / "sources.json", target / "sources.json")
    shutil.copy2(ASSETS / "arcana-webgpu.h", target / "arcana-webgpu.h")
    shutil.copy2(Path(__file__), target / "build-tts-native.py")


def windows_crt(staging, goarch):
    import re
    import struct

    # 只从 VS 可再分发目录取目标架构文件，不能借用 System32 或构建机 PATH。
    redist = os.environ.get("TTS_MSVC_REDIST_DIR") or os.environ.get("VCToolsRedistDir")
    if not redist:
        raise SystemExit("Windows WebGPU requires VCToolsRedistDir or TTS_MSVC_REDIST_DIR")
    arch = "arm64" if goarch == "arm64" else "x64"
    directories = list((Path(redist) / arch).glob("Microsoft.VC*.CRT"))
    if len(directories) != 1:
        raise SystemExit(f"Expected one {arch}/Microsoft.VC*.CRT directory under {redist}; "
                         "set TTS_MSVC_REDIST_DIR to the matching VS redistributable version")
    directory = directories[0]
    available = {path.name.lower(): path for path in directory.glob("*.dll")}
    queue = list(staging.glob("*.dll"))
    seen = set()
    copied = set()
    while queue:
        binary = queue.pop()
        if binary.name.lower() in seen:
            continue
        seen.add(binary.name.lower())
        imports = subprocess.check_output(
            [os.environ.get("OBJDUMP", "objdump"), "-p", str(binary)], text=True)
        for name in re.findall(r"DLL Name:\s*(\S+)", imports):
            if not name.lower().startswith(("msvcp", "vcruntime", "concrt")):
                continue
            target = staging / name
            if name.lower() in copied:
                continue
            original = available.get(name.lower())
            if original is None:
                raise SystemExit(f"Missing redistributable {name} required by {binary.name}: {directory}")
            with original.open("rb") as stream:
                header = stream.read(64)
                if len(header) != 64 or header[:2] != b"MZ":
                    raise SystemExit(f"Invalid CRT DLL: {original}")
                stream.seek(struct.unpack_from("<I", header, 0x3c)[0])
                signature = stream.read(6)
            machine = 0xaa64 if goarch == "arm64" else 0x8664
            if len(signature) != 6 or signature[:4] != b"PE\0\0" or struct.unpack_from("<H", signature, 4)[0] != machine:
                raise SystemExit(f"CRT DLL architecture does not match {arch}: {original}")
            shutil.copy2(original, target)
            copied.add(name.lower())
            queue.append(target)
    if not copied:
        raise SystemExit("Expected Windows WebGPU VC runtime imports were not found")
    target = staging / "licenses/tts/msvc"
    target.mkdir(parents=True, exist_ok=True)
    spec = MANIFEST["licenses"]["msvc-runtime-LICENSE.docx"]
    license_url = spec["url"]
    supplied = os.environ.get("TTS_MSVC_LICENSE_FILE")
    if supplied:
        document = Path(supplied)
        if not document.is_file() or not document.stat().st_size:
            raise SystemExit(f"Missing Visual C++ runtime license: {document}")
        shutil.copy2(document, target / ("LICENSE" + document.suffix))
    else:
        with urllib.request.urlopen(license_url, timeout=60) as response:
            data = response.read(8 << 20)
        if hashlib.sha256(data).hexdigest() != spec["sha256"]:
            raise SystemExit("Visual C++ runtime license changed upstream; "
                             "set TTS_MSVC_LICENSE_FILE to the official copy")
        (target / "LICENSE.docx").write_bytes(data)
    (target / "SOURCE.txt").write_text(
        f"Microsoft Visual C++ redistributable, app-local deployment ({arch})\n"
        f"Source directory: {directory}\nFiles: {', '.join(sorted(copied))}\n"
        "Redistribution requires a licensed Visual Studio installation and compliance with its terms.\n"
        "https://learn.microsoft.com/en-us/cpp/windows/redistributing-visual-cpp-files\n"
        "https://learn.microsoft.com/en-us/visualstudio/releases/2022/redistribution\n"
        "https://visualstudio.microsoft.com/license-terms/vs2022-cruntime/\n"
        f"Runtime license document: {license_url}\n", encoding="utf-8")


def windows_generator():
    """按当前 VS 开发者环境选择生成器；未加载环境时沿用 VS 2022。"""
    generators = {"17": "Visual Studio 17 2022", "18": "Visual Studio 18 2026"}
    major = os.environ.get("VisualStudioVersion", "17").split(".", 1)[0]
    if major not in generators:
        raise SystemExit(f"Unsupported Visual Studio {major}; set TTS_NATIVE_CMAKE_GENERATOR")
    return generators[major]


def main():
    if len(sys.argv) != 5:
        raise SystemExit("Usage: build-tts-native.py BUILD_DIR STAGING_DIR GOOS GOARCH")
    work, staging = (Path(value).resolve() for value in sys.argv[1:3])
    goos, goarch = sys.argv[3:]
    rid = RID.get((goos, goarch))
    if not rid:
        raise SystemExit(f"WebGPU native build unsupported: {goos}/{goarch}")
    work.mkdir(parents=True, exist_ok=True)
    staging.mkdir(parents=True, exist_ok=True)
    source = source_tree(work)
    generator = os.environ.get("TTS_NATIVE_CMAKE_GENERATOR") or (windows_generator() if goos == "windows" else "Ninja")
    args = ["cmake", "-S", str(source), "-B", str(work / "build"), "-G", generator,
            "-DCMAKE_BUILD_TYPE=Release", "-DBUILD_SHARED_LIBS=ON",
            "-DSHERPA_ONNX_ENABLE_BINARY=OFF", "-DSHERPA_ONNX_BUILD_C_API_EXAMPLES=OFF",
            "-DSHERPA_ONNX_ENABLE_PORTAUDIO=OFF", "-DSHERPA_ONNX_ENABLE_WEBSOCKET=OFF",
            "-DSHERPA_ONNX_ENABLE_TESTS=OFF", "-DSHERPA_ONNX_ENABLE_SPEAKER_DIARIZATION=OFF",
            "-DSHERPA_ONNX_USE_PRE_INSTALLED_ONNXRUNTIME_IF_AVAILABLE=OFF"]
    if goos == "darwin":
        # 保留 CPU 运行库的系统下限；要求 macOS 14 的插件只在选择 GPU 时加载。
        args += ["-DCMAKE_OSX_DEPLOYMENT_TARGET=11.0", "-DCMAKE_OSX_ARCHITECTURES=arm64"]
    native_env = os.environ.copy()
    if goos == "windows":
        # Go 使用 MinGW；上游 C++ 库使用 MSVC，二者只通过稳定 C ABI 交互。
        native_env.pop("CC", None)
        native_env.pop("CXX", None)
        args += ["-A", "ARM64" if goarch == "arm64" else "x64"]
    # Nix/离线构建可提供每个 FetchContent 名称对应的已解包目录。
    dependencies = os.environ.get("TTS_NATIVE_DEPS_DIR")
    if dependencies:
        for path in Path(dependencies).resolve().iterdir():
            if path.is_dir():
                args += [f"-DFETCHCONTENT_SOURCE_DIR_{path.name.upper()}={path}"]
        args += ["-DFETCHCONTENT_FULLY_DISCONNECTED=ON"]
    args += shlex.split(os.environ.get("TTS_NATIVE_CMAKE_ARGS", ""))
    subprocess.check_call(args, env=native_env)
    subprocess.check_call(["cmake", "--build", str(work / "build"), "--config", "Release",
                           "--target", "sherpa-onnx-c-api", "--parallel", os.environ.get("BUILD_JOBS", "2")],
                          env=native_env)
    patterns = {"darwin": ("*.dylib",), "linux": ("*.so", "*.so.*"), "windows": ("*.dll",)}
    roots = [work / "build/lib", work / "build/bin", work / "build/_deps/onnxruntime-src/lib"]
    if dependencies:
        roots += [Path(dependencies) / "onnxruntime/lib"]
    copied = set()
    for root in roots:
        for pattern in patterns[goos]:
            for path in root.rglob(pattern):
                if path.name not in copied:
                    shutil.copy2(path, staging / path.name)
                    copied.add(path.name)
    required = "sherpa-onnx-c-api.dll" if goos == "windows" else "libsherpa-onnx-c-api." + ("dylib" if goos == "darwin" else "so")
    if required not in copied:
        raise SystemExit(f"Native build did not produce {required}")
    package = os.environ.get("TTS_WEBGPU_PACKAGE") or download("webgpu", work)
    if hashlib.sha256(Path(package).read_bytes()).hexdigest() != MANIFEST["webgpu"]["sha256"]:
        raise SystemExit("WebGPU package SHA256 mismatch")
    with zipfile.ZipFile(package) as archive:
        prefix = f"runtimes/{rid}/native/"
        for member in archive.namelist():
            if member.startswith(prefix) and not member.endswith("/"):
                (staging / Path(member).name).write_bytes(archive.read(member))
        for name in ("LICENSE", "ThirdPartyNotices.txt"):
            target = staging / "licenses/tts" / ("webgpu-" + name)
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(archive.read(name))
    if goos == "windows":
        windows_crt(staging, goarch)
    collect_notices(work, source, staging)


if __name__ == "__main__":
    main()
