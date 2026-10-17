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
version = output(qtpaths, "--query", "QT_VERSION")
series = ".".join(version.split(".")[:2])
qt_source_url = f"https://download.qt.io/archive/qt/{series}/{version}/submodules/qtbase-everywhere-src-{version}.tar.xz"
(root / "qt.conf").write_text("[Paths]\nPrefix=.\nPlugins=plugins\nLibraries=.\n", encoding="utf-8")

licenses = root / "licenses"
licenses.mkdir(exist_ok=True)
license_source = Path(os.environ.get("QT_LICENSE_DIR", str(qtlibs.parent / "share/licenses")))
if sys.platform == "win32":
    # 自编译 Qt 的许可包含内嵌第三方声明；MinGW 运行库在收集依赖后补齐。
    if not license_source.is_dir():
        raise SystemExit(f"Missing Qt license directory: {license_source}")
    shutil.rmtree(licenses)
    shutil.copytree(license_source, licenses)
elif license_source.is_dir():
    shutil.copytree(license_source, licenses, dirs_exist_ok=True)
else:
    # 部分二进制 SDK 不安装许可证，直接从匹配版本源码包提取。
    with tempfile.TemporaryFile() as archive:
        with urllib.request.urlopen(qt_source_url) as response:
            shutil.copyfileobj(response, archive)
        archive.seek(0)
        with tarfile.open(fileobj=archive, mode="r:xz") as source:
            for member in source:
                if member.isfile() and "/LICENSES/" in member.name:
                    data = source.extractfile(member)
                    if data is not None:
                        (licenses / Path(member.name).name).write_bytes(data.read())
(licenses / "SOURCE.txt").write_text(
    f"Qt source: {qt_source_url}\n"
    + ("LayerShellQt source: https://download.kde.org/stable/plasma/6.3.6/\n" if sys.platform.startswith("linux") else "")
    + ("MinGW sources: https://github.com/msys2/MINGW-packages\n"
       "Windows Qt build: scripts/setup-windows-qt.sh in the Arcana World source.\n" if sys.platform == "win32" else "")
    + "These shared libraries remain dynamically replaceable in libexec.\n",
    encoding="utf-8",
)
if sys.platform == "win32":
    # 浮层自行绘制，不需要图片、主题、网络、TLS 或触摸插件。
    # 从唯一的平台插件出发收集 DLL，避免自动部署工具扩大依赖集合。
    plugin_root = root / "plugins"
    if plugin_root.exists():
        shutil.rmtree(plugin_root)
    platform_plugin = plugins / "platforms/qwindows.dll"
    destination = plugin_root / "platforms/qwindows.dll"
    destination.parent.mkdir(parents=True)
    shutil.copy2(platform_plugin, destination)
    # 只使用构建 Qt 的工具链，避免 PATH 中 Git 自带的 DLL 混入产物。
    prefix = Path(os.environ["MINGW_PREFIX"])
    search = [qtbin, prefix / "bin"]
    objdump = str(prefix / "bin/objdump.exe")
    system = Path(os.environ["SystemRoot"]) / "System32"
    system_names = {p.name.lower() for p in system.iterdir()}
    queue = [root / "awqt.dll", destination] + list(root.glob("*.exe"))
    sources = {platform_plugin}
    deployed = {root / "awqt.dll"}
    seen = set()
    while queue:
        binary = queue.pop()
        if binary in seen:
            continue
        seen.add(binary)
        for name in re.findall(r"DLL Name:\s*(\S+)", output(objdump, "-p", str(binary))):
            if name.lower().startswith(("api-ms-", "ext-ms-")) or name.lower() in system_names:
                continue
            dest = root / name
            if dest not in deployed:
                source = next((d / name for d in search if (d / name).is_file()), None)
                if source is None:
                    raise SystemExit(f"Missing runtime dependency {name}: {binary}")
                shutil.copy2(source, dest)
                sources.add(source)
                deployed.add(dest)
            queue.append(dest)
    # 重复构建时也移除旧部署残留，链接后的导入库不属于运行时。
    for binary in root.glob("*.dll"):
        if binary not in deployed:
            binary.unlink()
    (root / "libawqt.dll.a").unlink(missing_ok=True)

    # 使用 MSYS2 的包归属和文件清单，避免把整个构建环境的许可证打进包。
    # MinGW 头文件和静态启动代码也进入产物，需要保留对应许可。
    msys_root = prefix.parent
    pacman = str(msys_root / "usr/bin/pacman.exe")
    sources = {source for source in sources if not source.is_relative_to(qtbin.parent)}
    sources.update((prefix / "include/_mingw.h", prefix / "lib/libmingw32.a"))
    package_paths = ["/" + source.relative_to(msys_root).as_posix() for source in sorted(sources)]
    packages = sorted(set(output(pacman, "-Qqo", "--", *package_paths).splitlines()))
    for package in packages:
        copied = False
        for name in output(pacman, "-Qql", package).splitlines():
            if "/share/licenses/" not in name:
                continue
            source = msys_root / name.lstrip("/")
            if not source.is_file():
                continue
            relative = name.split("/share/licenses/", 1)[1]
            dest = licenses / relative
            dest.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source, dest)
            copied = True
        if not copied:
            raise SystemExit(f"Missing license files for runtime package: {package}")
    # 合并完整许可和版权声明；相同文本只保留一份，并列出所有来源组件。
    notices = {}
    source_notice = (licenses / "SOURCE.txt").read_bytes()
    for path in sorted(licenses.rglob("*")):
        if path.is_file() and path != licenses / "SOURCE.txt":
            notices.setdefault(path.read_bytes(), []).append(path.relative_to(licenses).as_posix())
    shutil.rmtree(licenses)
    licenses.mkdir()
    (licenses / "SOURCE.txt").write_bytes(source_notice)
    with (licenses / "THIRD_PARTY_NOTICES.txt").open("wb") as notice_file:
        for text, paths in notices.items():
            notice_file.write(("===== " + ", ".join(paths) + " =====\n").encode("utf-8"))
            notice_file.write(text)
            notice_file.write(b"\n\n")
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
