#!/usr/bin/env python3
"""将精美浮层的动态库及插件部署到私有 libexec 目录。"""
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys

import tarfile
import tempfile
import urllib.request

def output(*args):
    return subprocess.check_output(args, text=True).strip()


def run(*args):
    subprocess.run(args, check=True)


root = Path(sys.argv[1]).resolve()
qtpaths = os.environ.get("QTPATHS", "qtpaths6")
plugins = Path(output(qtpaths, "--query", "QT_INSTALL_PLUGINS"))
qtlibs = Path(output(qtpaths, "--query", "QT_INSTALL_LIBS"))
qtbin = Path(output(qtpaths, "--query", "QT_INSTALL_BINS"))
(root / "qt.conf").write_text("[Paths]\nPrefix=.\nPlugins=plugins\nLibraries=.\n", encoding="utf-8")

licenses = root / "licenses"
licenses.mkdir(exist_ok=True)
license_source = Path(os.environ.get("QT_LICENSE_DIR", str(qtlibs.parent / "share/licenses")))
if license_source.is_dir():
    shutil.copytree(license_source, licenses, dirs_exist_ok=True)
else:
    # 部分二进制 SDK 不安装许可证，直接从匹配版本源码包提取。
    version = output(qtpaths, "--query", "QT_VERSION")
    series = ".".join(version.split(".")[:2])
    url = f"https://download.qt.io/archive/qt/{series}/{version}/submodules/qtbase-everywhere-src-{version}.tar.xz"
    with tempfile.TemporaryFile() as archive:
        with urllib.request.urlopen(url) as response:
            shutil.copyfileobj(response, archive)
        archive.seek(0)
        with tarfile.open(fileobj=archive, mode="r:xz") as source:
            for member in source:
                if member.isfile() and "/LICENSES/" in member.name:
                    data = source.extractfile(member)
                    if data is not None:
                        (licenses / Path(member.name).name).write_bytes(data.read())
(licenses / "SOURCE.txt").write_text(
    "Qt source: https://download.qt.io/archive/qt/\n"
    "LayerShellQt source: https://download.kde.org/stable/plasma/6.3.6/\n"
    "These shared libraries remain dynamically replaceable in libexec.\n",
    encoding="utf-8",
)
if sys.platform == "win32":
    # windeployqt 负责插件和 Qt 模块，随后补齐 MinGW 与插件的间接依赖。
    run(str(qtbin / "windeployqt.exe"), "--release", "--compiler-runtime", "--no-translations",
        "--dir", str(root), "--plugindir", str(root / "plugins"), str(root / "awqt.dll"))
    search = [root, qtbin] + [Path(p) for p in os.environ["PATH"].split(os.pathsep)]
    system = Path(os.environ["SystemRoot"]) / "System32"
    system_names = {p.name.lower() for p in system.iterdir()}
    queue = list(root.rglob("*.dll")) + list(root.glob("*.exe"))
    seen = set()
    while queue:
        binary = queue.pop()
        if binary in seen:
            continue
        seen.add(binary)
        for name in re.findall(r"DLL Name:\s*(\S+)", output("objdump", "-p", str(binary))):
            if name.lower().startswith(("api-ms-", "ext-ms-")) or name.lower() in system_names:
                continue
            dest = root / name
            if not dest.exists():
                source = next((d / name for d in search if (d / name).exists()), None)
                if source is None:
                    raise SystemExit(f"Missing runtime dependency {name}: {binary}")
                shutil.copy2(source, dest)
            queue.append(dest)
else:
    # 浮层只用 QPainter 绘制内存 QPixmap，不读取图片或使用原生控件主题。
    # 只部署实际使用的插件，避免图片、主题等插件拉入 WebEngine 等无关模块。
    if sys.platform == "darwin":
        plugin_names = ["platforms/libqcocoa.dylib"]
    elif sys.platform.startswith("linux"):
        # Qt 6.8 使用 qwayland-generic，较新版本改名为 qwayland；均支持光栅绘制。
        wayland = next((name for name in (
            "platforms/libqwayland-generic.so", "platforms/libqwayland.so"
        ) if (plugins / name).is_file()), None)
        if wayland is None:
            raise SystemExit(f"Missing raster Wayland platform plugin: {plugins}")
        # Qt 初始化需要默认 xdg-shell；逐窗口 layer-shell 由 LayerShellQt 接管。
        plugin_names = [wayland, "wayland-shell-integration/libxdg-shell.so",
                        "wayland-shell-integration/liblayer-shell.so"]
    else:
        raise SystemExit(f"Unsupported deployment platform: {sys.platform}")
    deployed_plugins = []
    for name in plugin_names:
        source = plugins / name
        if not source.is_file():
            raise SystemExit(f"Missing runtime plugin: {source}")
        destination = root / "plugins" / name
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, destination)
        deployed_plugins.append(destination)
    if sys.platform == "darwin":
        # 私有应用包提供真实 Dock 名称；主程序继续通过原 helper 路径启动。
        helper = root / "arcana-world-overlay-qt"
        bundle = root / "Arcana World Elegant Overlay.app"
        executable_dir = bundle / "Contents/MacOS"
        resources = bundle / "Contents/Resources"
        executable_dir.mkdir(parents=True, exist_ok=True)
        resources.mkdir(parents=True, exist_ok=True)
        executable = executable_dir / helper.name
        if helper.is_symlink():
            helper.unlink()
        else:
            helper.replace(executable)
        shutil.copy2(Path(__file__).resolve().parent.parent / "internal/overlay/qt/Info.plist",
                     bundle / "Contents/Info.plist")
        (resources / "qt.conf").write_text("[Paths]\nPrefix=../..\nPlugins=plugins\nLibraries=.\n", encoding="utf-8")
        helper.symlink_to(executable.relative_to(root))
        queue = [root / "libawqt.dylib", executable] + deployed_plugins
        seen = set()
        def resolve(name, binary):
            if name.startswith("@loader_path/"):
                return binary.parent / name.removeprefix("@loader_path/")
            if name.startswith("@executable_path/"):
                return executable_dir / name.removeprefix("@executable_path/")
            if name.startswith("@rpath/"):
                relative = name.removeprefix("@rpath/")
                candidates = [qtlibs / relative, root / relative]
                commands = output("otool", "-l", str(binary))
                for value in re.findall(r"cmd LC_RPATH\s+cmdsize \d+\s+path (.*?) \(offset", commands):
                    value = value.replace("@loader_path", str(binary.parent)).replace("@executable_path", str(executable_dir))
                    candidates.append(Path(value) / relative)
                return next((p for p in candidates if p.exists()), candidates[0])
            return Path(name)
        while queue:
            binary = queue.pop().resolve()
            if binary in seen:
                continue
            seen.add(binary)
            binary.chmod(binary.stat().st_mode | 0o200)
            dependencies = output("otool", "-L", str(binary)).splitlines()[1:]
            for line in dependencies:
                name = line.strip().split(" (compatibility", 1)[0]
                if name.startswith(("/System/", "/usr/lib/")):
                    continue
                source = Path(os.path.normpath(resolve(name, binary)))
                if ".framework/" in str(source):
                    prefix, rest = str(source).split(".framework/", 1)
                    framework = Path(prefix + ".framework")
                    destination = root / framework.name
                    if not destination.exists():
                        shutil.copytree(framework, destination, symlinks=True)
                    dest = destination / rest
                else:
                    dest = root / source.name
                    if not dest.exists():
                        if not source.exists():
                            raise SystemExit(f"Missing runtime dependency {name}: {binary}")
                        shutil.copy2(source, dest)
                replacement = "@loader_path/" + os.path.relpath(dest, binary.parent)
                run("install_name_tool", "-change", name, replacement, str(binary))
                if dest.resolve() != binary.resolve():
                    queue.append(dest)
            if binary.suffix == ".dylib" or ".framework/" in str(binary):
                run("install_name_tool", "-id", "@rpath/" + str(binary.relative_to(root)), str(binary))
        # 改写加载路径后重新做临时签名，支持 Apple Silicon。
        for binary in seen:
            run("codesign", "--force", "--sign", "-", str(binary))
        run("codesign", "--force", "--sign", "-", str(bundle))
    elif sys.platform.startswith("linux"):
        queue = [root / "arcana-world-overlay-qt", root / "libawqt.so"] + deployed_plugins
        seen = set()
        # glibc 与加载器必须使用目标系统的版本，不可拆开捆绑。
        system_libraries = re.compile(r"^(?:ld-linux.*|lib(?:c|m|pthread|dl|rt|resolv|util|anl|nss_[^.]+)\.so(?:\..*)?)$")
        while queue:
            binary = queue.pop()
            if binary in seen or not binary.is_file():
                continue
            seen.add(binary)
            dependencies = output("ldd", str(binary))
            if "not found" in dependencies:
                raise SystemExit(f"Unresolved runtime dependency: {binary}\n{dependencies}")
            for name, filename in re.findall(r"^\s*(\S+) => (/\S+)", dependencies, re.MULTILINE):
                if system_libraries.match(name):
                    continue
                dest = root / name
                if not dest.exists():
                    shutil.copy2(filename, dest)
                queue.append(dest)
            binary.chmod(binary.stat().st_mode | 0o200)
            relative = os.path.relpath(root, binary.parent)
            run("patchelf", "--set-rpath", "$ORIGIN" if relative == "." else "$ORIGIN/" + relative, str(binary))
    else:
        raise SystemExit(f"Unsupported deployment platform: {sys.platform}")
