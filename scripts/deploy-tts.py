"""部署统一 TTS 辅助程序所需的固定版本原生运行库。"""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import urllib.request

from runtime_files import publish

SOURCES = json.loads((Path(__file__).resolve().parent / "tts-native/sources.json").read_text(encoding="utf-8"))
VERSION = "v1.13.8"
PLATFORMS = {
    ("darwin", "amd64"): ("macos", "x86_64-apple-darwin"),
    ("darwin", "arm64"): ("macos", "aarch64-apple-darwin"),
    ("linux", "amd64"): ("linux", "x86_64-unknown-linux-gnu"),
    ("linux", "arm64"): ("linux", "aarch64-unknown-linux-gnu"),
    ("windows", "amd64"): ("windows", "x86_64-pc-windows-gnu"),
    ("windows", "386"): ("windows", "i686-pc-windows-gnu"),
    ("windows", "arm64"): ("windows", "aarch64-pc-windows-gnu"),
}


def output(*args):
    return subprocess.check_output(args, text=True).strip()


def module(platform):
    info = json.loads(output("go", "mod", "download", "-json",
                             f"github.com/k2-fsa/sherpa-onnx-go-{platform}@{VERSION}"))
    if "Error" in info:
        raise SystemExit(info["Error"])
    return Path(info["Dir"])


def notices(root, source):
    target = root / "licenses/tts"
    target.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source / "LICENSE", target / "sherpa-onnx-LICENSE")
    # ORT 1.28.2 是官方 Go CPU 二进制使用的运行库；保留其第三方归属，内容按固定摘要校验。
    supplied = os.environ.get("TTS_LICENSE_DIR")
    for name in ("onnxruntime-LICENSE", "onnxruntime-ThirdPartyNotices.txt"):
        spec = SOURCES["licenses"][name]
        if supplied:
            data = (Path(supplied) / name).read_bytes()
        else:
            with urllib.request.urlopen(spec["url"], timeout=60) as response:
                data = response.read(8 << 20)
        if hashlib.sha256(data).hexdigest() != spec["sha256"]:
            raise SystemExit(f"SHA256 mismatch: {name}")
        (target / name).write_bytes(data)
    (target / "SOURCE.txt").write_text(
        f"sherpa-onnx runtime {VERSION}: https://github.com/k2-fsa/sherpa-onnx/tree/{VERSION}\n"
        f"Platform module: {source.name}\n"
        "ONNX Runtime: https://github.com/microsoft/onnxruntime/tree/v1.28.2\n" +
        ("Arcana WebGPU extension: scripts/build-tts-native.py and scripts/tts-native/\n"
         "WebGPU EP 0.4.0: https://www.nuget.org/packages/Microsoft.ML.OnnxRuntime.EP.WebGpu/0.4.0\n"
         if os.environ.get("TTS_NATIVE_DIR") else "") +
        "Model assets are installed separately and retain their own LICENSE.\n", encoding="utf-8")


def macos(root, binaries):
    for binary in binaries:
        binary.chmod(0o755)
        if binary.suffix == ".dylib":
            subprocess.check_call(["install_name_tool", "-id", "@rpath/" + binary.name, str(binary)])
        for line in output("otool", "-L", str(binary)).splitlines()[1:]:
            dependency = line.strip().split(" (", 1)[0]
            name = Path(dependency).name
            if (root / name).is_file():
                subprocess.check_call(["install_name_tool", "-change", dependency,
                                       "@loader_path/" + name, str(binary)])
            elif not dependency.startswith(("/System/Library/", "/usr/lib/")):
                raise SystemExit(f"Unbundled macOS TTS dependency: {dependency}")
        # 修改 load commands 后重新签名；不继承模块缓存中的开发路径。
        commands = output("otool", "-l", str(binary))
        for path in re.findall(r"cmd LC_RPATH\s+cmdsize \d+\s+path (.*?) \(offset", commands):
            subprocess.check_call(["install_name_tool", "-delete_rpath", path, str(binary)])
        subprocess.check_call(["codesign", "--force", "--sign", "-", str(binary)])


def linux(root, binaries, source):
    from deploy_linux import SYSTEM, licenses
    queue = list(binaries)
    seen = set()
    licensed = set()
    # 从系统复制的编译器运行库（libstdc++、libgcc_s 等）只依赖系统库，保持原样；
    # 修改它们没有收益，patchelf 处理这类库还可能崩溃。
    system_copies = set()
    while queue:
        binary = queue.pop()
        if binary in seen:
            continue
        seen.add(binary)
        dependencies = output("ldd", str(binary))
        if "not found" in dependencies:
            raise SystemExit(f"Unresolved TTS runtime dependency:\n{dependencies}")
        for name, filename in re.findall(r"^\s*(.+?) => (/.+?) \(", dependencies, re.MULTILINE):
            if SYSTEM.match(name):
                continue
            dest = root / name
            if not dest.exists():
                original = Path(filename)
                shutil.copy2(original, dest)
                dest.chmod(0o755)
                if not original.resolve().is_relative_to(source.resolve()):
                    licensed.add(original)
                    system_copies.add(dest)
                queue.append(dest)
    for binary in seen - system_copies:
        subprocess.check_call(["patchelf", "--set-rpath", "$ORIGIN", str(binary)])
    licenses(root, licensed)


def windows(root, binaries):
    system = {"d3d12.dll", "dxcore.dll", "kernelbase.dll", "cfgmgr32.dll", "winhttp.dll",
              "kernel32.dll", "advapi32.dll", "bcrypt.dll", "crypt32.dll", "dbghelp.dll", "dxgi.dll",
              "dnsapi.dll", "gdi32.dll", "iphlpapi.dll", "msvcrt.dll", "ntdll.dll", "ole32.dll",
              "oleaut32.dll", "powrprof.dll", "psapi.dll", "rpcrt4.dll", "secur32.dll", "setupapi.dll",
              "shell32.dll", "shlwapi.dll", "user32.dll", "userenv.dll", "version.dll", "winmm.dll",
              "ws2_32.dll", "ucrtbase.dll"}
    for binary in binaries:
        dependencies = re.findall(r"DLL Name:\s*(\S+)", output(os.environ.get("OBJDUMP", "objdump"), "-p", str(binary)))
        for name in dependencies:
            lower = name.lower()
            if lower in system or lower.startswith(("api-ms-win-", "ext-ms-win-")) or (root / name).exists():
                continue
            raise SystemExit(f"Unbundled Windows TTS dependency {name}; use the matching MinGW compiler with static compiler runtimes")


def main():
    if len(sys.argv) != 5:
        raise SystemExit("Usage: deploy-tts.py STAGING_DIR DESTINATION_DIR GOOS GOARCH")
    staging, destination = (Path(value).resolve() for value in sys.argv[1:3])
    goos, goarch = sys.argv[3:]
    platform = PLATFORMS.get((goos, goarch))
    if platform is None:
        raise SystemExit(f"Official sherpa-onnx {VERSION} has no Go CPU runtime for {goos}/{goarch}")
    source = module(platform[0])
    native = os.environ.get("TTS_NATIVE_DIR")
    lib = Path(native).resolve() if native else source / "lib" / platform[1]
    if native:
        patterns = {"darwin": ["*.dylib"], "linux": ["*.so", "*.so.*"], "windows": ["*.dll"]}
        plugin = {"darwin": "libonnxruntime_providers_webgpu.dylib",
                  "linux": "libonnxruntime_providers_webgpu.so",
                  "windows": "onnxruntime_providers_webgpu.dll"}[goos]
        if not (lib / plugin).is_file():
            raise SystemExit(f"Missing WebGPU provider: {lib / plugin}")
        if lib != staging and (lib / "licenses").exists():
            shutil.copytree(lib / "licenses", staging / "licenses", dirs_exist_ok=True)
    else:
        if (goos, goarch) not in {("darwin", "amd64"), ("windows", "386")}:
            raise SystemExit("WebGPU targets require scripts/build-tts-native.py output in TTS_NATIVE_DIR")
        patterns = {"darwin": ["libsherpa-onnx-c-api.dylib", "libonnxruntime*.dylib"],
                    "linux": ["libsherpa-onnx-c-api.so", "libonnxruntime.so"],
                    "windows": ["sherpa-onnx-c-api.dll", "onnxruntime.dll"]}
    for pattern in patterns[goos]:
        matches = list(lib.glob(pattern))
        if not matches and not (native and pattern == "*.so.*"):
            raise SystemExit(f"Missing pinned native library: {lib / pattern}")
        for path in matches:
            if lib != staging:
                shutil.copy2(path, staging / path.name)
            (staging / path.name).chmod(0o755)
    notices(staging, source)
    binaries = [path for path in staging.iterdir() if path.is_file()]
    if goos == "darwin":
        macos(staging, binaries)
    elif goos == "linux":
        # 官方库的 SONAME 可能带版本，先通过原始缓存解析完整依赖。
        os.environ["LD_LIBRARY_PATH"] = str(lib) + ":" + os.environ.get("LD_LIBRARY_PATH", "")
        linux(staging, binaries, lib if native else source)
    else:
        windows(staging, binaries)
    helper = Path("arcana-world-tts" + (".exe" if goos == "windows" else ""))
    publish(staging, destination, ".arcana-tts-files.json", build_outputs=(helper,))


if __name__ == "__main__":
    main()
