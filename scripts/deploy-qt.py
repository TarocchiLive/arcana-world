#!/usr/bin/env python3
"""将精美浮层的动态库及插件部署到私有 libexec 目录。"""
import json
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


MANIFEST = ".arcana-qt-files.json"


def owned_paths(destination):
    """只接受目录内的文件清单，不允许清单借助父目录链接越界。"""
    marker = destination / MANIFEST
    if marker.is_symlink():
        raise SystemExit(f"Invalid Qt ownership manifest symlink: {marker}")
    if not marker.exists():
        return set()
    try:
        names = json.loads(marker.read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        raise SystemExit(f"Invalid Qt ownership manifest {marker}: {error}") from error
    if not isinstance(names, list) or any(not isinstance(name, str) for name in names):
        raise SystemExit(f"Invalid Qt ownership manifest: {marker}")
    paths = set()
    for name in names:
        relative = Path(name)
        if (not name or "\\" in name or ":" in name or relative.is_absolute()
                or any(part in ("", ".", "..") for part in name.split("/"))
                or name == MANIFEST):
            raise SystemExit(f"Invalid Qt ownership path: {name!r}")
        validate_parent(destination, relative)
        paths.add(relative)
    return paths


def validate_parent(destination, relative):
    for parent in relative.parents:
        path = destination / parent
        if path.is_symlink() or (path.exists() and not path.is_dir()):
            raise SystemExit(f"Invalid runtime parent directory: {path}")


def remove_owned(destination, paths):
    for relative in sorted(paths):
        path = destination / relative
        if path.is_dir() and not path.is_symlink():
            raise SystemExit(f"Refusing to remove runtime directory as a file: {path}")
    for relative in sorted(paths):
        (destination / relative).unlink(missing_ok=True)
    # 只清理文件清单的空父目录，保留用户后来加入的其他文件。
    parents = {parent for path in paths for parent in path.parents if parent != Path(".")}
    for parent in sorted(parents, key=lambda path: len(path.parts), reverse=True):
        path = destination / parent
        if path.is_dir() and not any(path.iterdir()):
            path.rmdir()


def publish(source, destination):
    """收集成功后逐文件发布；不承诺多个文件替换具有原子性。"""
    if not (destination / MANIFEST).exists() and any(
            (destination / name).exists() or (destination / name).is_symlink()
            for name in ("qt.conf", "arcana-world-overlay-qt",
                         "arcana-world-overlay-qt.exe", "Arcana World Elegant Overlay.app")):
        raise SystemExit(f"Existing unmanaged Qt runtime in {destination}; "
                         "use a clean/new output directory")
    previous = owned_paths(destination)
    paths = {path.relative_to(source) for path in source.rglob("*")
             if path.is_symlink() or path.is_file()}
    if Path(MANIFEST) in paths:
        raise SystemExit(f"Staging directory contains reserved manifest: {source / MANIFEST}")
    for relative in paths | previous:
        validate_parent(destination, relative)
        target = destination / relative
        if target.is_dir() and not target.is_symlink():
            raise SystemExit(f"Runtime file collides with directory: {target}")
        path = source / relative
        if path.is_symlink() and (Path(os.readlink(path)).is_absolute()
                                  or not path.resolve().is_relative_to(source)):
            raise SystemExit(f"Runtime symlink escapes staging directory: {path}")
    destination.mkdir(parents=True, exist_ok=True)
    # 在目标文件系统准备副本，再用 replace 替换只读文件和旧符号链接。
    with tempfile.TemporaryDirectory(prefix=".qt-publish-", dir=destination) as temporary:
        prepared = Path(temporary)
        for relative in sorted(paths):
            target = prepared / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source / relative, target, follow_symlinks=False)
        marker = prepared / MANIFEST
        marker.write_text(json.dumps(sorted(path.as_posix() for path in paths), indent=2) + "\n",
                          encoding="utf-8")
        for relative in sorted(paths):
            target = destination / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            (prepared / relative).replace(target)
        remove_owned(destination, previous - paths)
        marker.replace(destination / MANIFEST)


def arguments():
    usage = (f"Usage: {sys.argv[0]} STAGING_DIR DESTINATION_DIR\n"
             f"       {sys.argv[0]} --remove DESTINATION_DIR\n"
             "Collect Qt runtime files from a fresh build, then publish them.\n"
             "--remove deletes only files recorded in the Qt ownership manifest.")
    if sys.argv[1:] in (["-h"], ["--help"]):
        print(usage)
        raise SystemExit(0)
    if len(sys.argv) == 3 and sys.argv[1] == "--remove":
        destination = Path(sys.argv[2]).resolve()
        remove_owned(destination, owned_paths(destination))
        (destination / MANIFEST).unlink(missing_ok=True)
        raise SystemExit(0)
    if len(sys.argv) != 3:
        raise SystemExit(usage)
    source, destination = (Path(value).resolve() for value in sys.argv[1:])
    if not source.is_dir():
        raise SystemExit(f"Missing staging directory: {source}")
    if source.is_relative_to(destination) or destination.is_relative_to(source):
        raise SystemExit("Staging and destination directories must not contain each other")
    return source, destination


root, destination_root = arguments()
qtpaths = os.environ.get("QTPATHS", "qtpaths6")
plugins = Path(output(qtpaths, "--query", "QT_INSTALL_PLUGINS"))
# 分包安装的 Qt 插件可能位于独立前缀，优先使用 qtpaths 对应版本。
plugin_dirs = [plugins] + [
    Path(path) for path in os.environ.get("QT_PLUGIN_PATH", "").split(os.pathsep) if path
]
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
    if not platform_plugin.is_file():
        raise SystemExit(f"Missing runtime plugin: {platform_plugin}")
    destination = plugin_root / "platforms/qwindows.dll"
    destination.parent.mkdir(parents=True)
    shutil.copy2(platform_plugin, destination)
    # 运行库与编译 Qt 的 UCRT64 工具链保持一致，避免混入 Git for Windows 的同名 DLL。
    prefix = Path(os.environ["MINGW_PREFIX"])
    search = [qtbin, prefix / "bin"]
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
        for name in re.findall(r"DLL Name:\s*(\S+)", output(str(prefix / "bin/objdump.exe"), "-p", str(binary))):
            if name.lower().startswith(("api-ms-", "ext-ms-")) or name.lower() in system_names:
                continue
            dest = root / name
            if dest not in deployed:
                source = next((d / name for d in search if (d / name).is_file()), None)
                if source is None:
                    raise SystemExit(f"Missing runtime dependency {name}: {binary}")
                dest.unlink(missing_ok=True)
                shutil.copy2(source, dest)
                sources.add(source)
                deployed.add(dest)
            queue.append(dest)
    # 输出目录由构建脚本隔离；不按扩展名删除可能属于用户的 DLL。
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
        # 自编译 SDK 只部署 Cocoa；重复打包时不保留旧图片或主题插件。
        shutil.rmtree(root / "plugins", ignore_errors=True)
    elif sys.platform.startswith("linux"):
        # Qt 6.8 使用 qwayland-generic，较新版本改名为 qwayland；均支持光栅绘制。
        wayland = next((name for name in (
            "platforms/libqwayland-generic.so", "platforms/libqwayland.so"
        ) if any((directory / name).is_file() for directory in plugin_dirs)), None)
        if wayland is None:
            raise SystemExit(f"Missing raster Wayland platform plugin: {plugins}")
        # Qt 初始化需要默认 xdg-shell；逐窗口 layer-shell 由 LayerShellQt 接管。
        plugin_names = [wayland, "wayland-shell-integration/libxdg-shell.so",
                        "wayland-shell-integration/liblayer-shell.so"]
    else:
        raise SystemExit(f"Unsupported deployment platform: {sys.platform}")
    deployed_plugins = []
    for name in plugin_names:
        source = next((directory / name for directory in plugin_dirs
                       if (directory / name).is_file()), plugins / name)
        if not source.is_file():
            raise SystemExit(f"Missing runtime plugin: {source}")
        destination = root / "plugins" / name
        destination.parent.mkdir(parents=True, exist_ok=True)
        # Nix store 的文件只读；重新部署时替换副本，避免继承权限阻止覆盖。
        destination.unlink(missing_ok=True)
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
            if helper.resolve() != executable.resolve() or not executable.is_file():
                raise SystemExit(f"Invalid deployed helper symlink: {helper}")
            helper.unlink()
        else:
            helper.replace(executable)
        shutil.copy2(Path(__file__).resolve().parent.parent / "internal/overlay/qt/Info.plist",
                     bundle / "Contents/Info.plist")
        # macOS 的 Prefix 相对于应用包 Contents，而非配置文件所在的 Resources。
        (resources / "qt.conf").write_text("[Paths]\nPrefix=../..\nPlugins=plugins\nLibraries=.\n", encoding="utf-8")
        helper.symlink_to(executable.relative_to(root))
        queue = [root / "libawqt.dylib", executable] + deployed_plugins
        seen = set()
        copied = {root / "libawqt.dylib"}
        copied_frameworks = set()
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
                if not source.is_file():
                    raise SystemExit(f"Missing runtime dependency {name}: {binary}\nResolved path: {source}")
                if ".framework/" in str(source):
                    prefix, rest = str(source).split(".framework/", 1)
                    framework = Path(prefix + ".framework")
                    destination = root / framework.name
                    if destination not in copied_frameworks and framework != destination:
                        if destination.exists():
                            shutil.rmtree(destination)
                        # 开发环境也可能提供 framework，只保留运行库与资源。
                        shutil.copytree(
                            framework, destination, symlinks=True,
                            ignore=shutil.ignore_patterns(
                                "Headers", "PrivateHeaders", "Modules", "*.prl", "*.a", "*.dSYM",
                            ),
                        )
                    copied_frameworks.add(destination)
                    dest = destination / rest
                else:
                    dest = root / source.name
                    if dest not in copied and source != dest:
                        dest.unlink(missing_ok=True)
                        shutil.copy2(source, dest)
                    copied.add(dest)
                replacement = "@loader_path/" + os.path.relpath(dest, binary.parent)
                run("install_name_tool", "-change", name, replacement, str(binary))
                if dest.resolve() != binary.resolve():
                    queue.append(dest)
            if binary.suffix == ".dylib" or ".framework/" in str(binary):
                run("install_name_tool", "-id", "@rpath/" + str(binary.relative_to(root)), str(binary))
            # 依赖均已改为 loader 相对路径，不再泄漏 SDK/Homebrew 的绝对搜索路径。
            commands = output("otool", "-l", str(binary))
            for value in re.findall(r"cmd LC_RPATH\s+cmdsize \d+\s+path (.*?) \(offset", commands):
                if value.startswith("/"):
                    run("install_name_tool", "-delete_rpath", value, str(binary))
            run("strip", "-S", str(binary))
        # 改写加载路径后重新做临时签名，支持 Apple Silicon。
        for binary in seen:
            run("codesign", "--force", "--sign", "-", str(binary))
        run("codesign", "--force", "--sign", "-", str(bundle))
    elif sys.platform.startswith("linux"):
        queue = [root / "arcana-world-overlay-qt", root / "libawqt.so"] + deployed_plugins
        seen = set()
        copied = {root / "libawqt.so"}
        # glibc 与加载器必须使用目标系统的版本，不可拆开捆绑。
        system_libraries = re.compile(r"^(?:ld-linux.*|lib(?:c|m|pthread|dl|rt|resolv|util|anl|nss_[^.]+)\.so(?:\..*)?)$")
        while queue:
            binary = queue.pop()
            if binary in seen:
                continue
            if not binary.is_file():
                raise SystemExit(f"Missing runtime binary: {binary}")
            seen.add(binary)
            dependencies = output("ldd", str(binary))
            if "not found" in dependencies:
                raise SystemExit(f"Unresolved runtime dependency: {binary}\n{dependencies}")
            for name, filename in re.findall(r"^\s*(.+?) => (/.+?) \(", dependencies, re.MULTILINE):
                # Nix 的 ldd 可能把绝对路径列为依赖名；副本始终留在部署目录。
                library = Path(name).name
                if system_libraries.match(library):
                    continue
                dest = root / library
                source = Path(filename)
                if dest not in copied and source != dest:
                    # 替换旧副本而非覆盖，兼容只读的 Nix store 文件权限。
                    dest.unlink(missing_ok=True)
                    shutil.copy2(source, dest)
                copied.add(dest)
                if name != library:
                    binary.chmod(binary.stat().st_mode | 0o200)
                    run("patchelf", "--replace-needed", name, library, str(binary))
                queue.append(dest)
            binary.chmod(binary.stat().st_mode | 0o200)
            relative = os.path.relpath(root, binary.parent)
            run("patchelf", "--set-rpath", "$ORIGIN" if relative == "." else "$ORIGIN/" + relative, str(binary))
    else:
        raise SystemExit(f"Unsupported deployment platform: {sys.platform}")

publish(root, destination_root)
