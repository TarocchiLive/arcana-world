"""部署两个 Linux 后端共用的动态库、数据和许可。"""
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys

from runtime_files import publish

SYSTEM = re.compile(r"^(?:ld-linux.*|ld64\.so.*|lib(?:c|m|pthread|dl|rt|resolv|util|anl|nss_[^.]+)\.so(?:\..*)?)$")


def output(*args):
    try:
        # 打包目录可能在 LD_LIBRARY_PATH 中；patchelf 不能加载正在改写的编译器运行库。
        environment = None
        if args[0] == "patchelf":
            environment = os.environ.copy()
            environment.pop("LD_LIBRARY_PATH", None)
        return subprocess.check_output(args, text=True, stderr=subprocess.STDOUT, env=environment).strip()
    except (OSError, subprocess.CalledProcessError) as error:
        raise SystemExit(f"Linux runtime deployment failed: {args!r}\n{getattr(error, 'output', str(error))}") from error


def copy_tree(source, destination):
    shutil.copytree(source, destination, dirs_exist_ok=True)
    # Nix 数据目录只读；临时副本必须可清理，不能继承 store 的目录权限。
    for directory in [destination, *destination.rglob("*")]:
        if directory.is_dir():
            directory.chmod(directory.stat().st_mode | 0o700)


def licenses(root, sources):
    """保留发行版版权与源码声明；本地 Nix 构建明确提示再分发许可缺口。"""
    directory = root / "licenses/linux"
    directory.mkdir(parents=True, exist_ok=True)
    supplied = os.environ.get("LINUX_LICENSE_DIR")
    if supplied:
        source = Path(supplied)
        if not source.is_dir() or not (source / "SOURCE.txt").is_file():
            raise SystemExit("LINUX_LICENSE_DIR must contain dependency licenses and SOURCE.txt source notices")
        copy_tree(source, directory)
        return
    if sources and all(str(source.resolve()).startswith("/nix/store/") for source in sources):
        notice = ("Local Nix build: complete redistribution license notices are not installed. "
                  "Use Ubuntu release packaging or supply LINUX_LICENSE_DIR before redistribution.")
        print("WARNING: " + notice, file=sys.stderr)
        (directory / "SOURCE.txt").write_text(
            notice + "\nNixpkgs sources: https://github.com/NixOS/nixpkgs\nRuntime inputs:\n"
            + "\n".join(str(source.resolve()) for source in sorted(sources)) + "\n", encoding="utf-8")
        for source in sorted(sources):
            package = Path("/nix/store") / source.resolve().parts[3]
            available = package / "share/licenses"
            if available.is_dir():
                copy_tree(available, directory / package.name)
        return
    if shutil.which("dpkg-query") is None:
        raise SystemExit("Linux dependency licenses require dpkg-query or LINUX_LICENSE_DIR with SOURCE.txt")
    packages = set()
    for source in sorted(sources):
        candidates = [source, source.resolve()]
        for path in list(candidates):
            if str(path).startswith("/usr/"):
                candidates.append(Path(str(path)[4:]))
        package = None
        for candidate in candidates:
            result = subprocess.run(["dpkg-query", "-S", str(candidate)], text=True, capture_output=True)
            if result.returncode == 0:
                package = result.stdout.split(": ", 1)[0]
                break
        if package is None:
            raise SystemExit(f"No package copyright metadata for {source}; provide LINUX_LICENSE_DIR including SOURCE.txt")
        packages.add(package)
    for package in sorted(packages):
        name = package.split(":", 1)[0]
        copyright_file = Path("/usr/share/doc") / name / "copyright"
        if not copyright_file.is_file():
            raise SystemExit(f"Missing runtime license: {copyright_file}")
        target = directory / name
        target.mkdir(exist_ok=True)
        shutil.copy2(copyright_file, target / "copyright")
        metadata = output("dpkg-query", "-W", "-f=${binary:Package}\t${Version}\t${source:Package}\t${source:Version}\\n", package)
        (target / "SOURCE.txt").write_text(
            "Binary package, version, source package, source version:\n" + metadata + "\n",
            encoding="utf-8")
    # Debian 版权文件引用的公共许可必须随包提供，不能依赖目标主机。
    common = Path("/usr/share/common-licenses")
    if common.is_dir():
        copy_tree(common, directory / "common-licenses")
    (directory / "SOURCE.txt").write_text(
        "Sources: https://archive.ubuntu.com/ubuntu/pool/ and https://deb.debian.org/debian/pool/\n"
        "Each package SOURCE.txt records its binary and source versions.\n"
        "See each package copyright for upstream source URLs and licenses.\n"
        "Libraries remain dynamically replaceable in libexec.\n", encoding="utf-8")


def collect(root, binaries, licensed_prefix=None):
    for tool in ("ldd", "patchelf", "pkg-config"):
        if shutil.which(tool) is None:
            raise SystemExit(f"Linux portable runtime deployment requires {tool}")
    sources = set()
    queue = [(binary, binary) for binary in binaries]
    copied = {binary.name for binary in binaries if binary.parent == root}
    seen = set()
    while queue:
        original, binary = queue.pop()
        if binary in seen:
            continue
        if not original.is_file():
            raise SystemExit(f"Missing Linux runtime binary: {original}")
        seen.add(binary)
        binary.chmod(binary.stat().st_mode | 0o200)
        dependencies = output("ldd", str(original))
        if "not found" in dependencies:
            raise SystemExit(f"Unresolved Linux runtime dependency: {original}\n{dependencies}")
        needed = output("patchelf", "--print-needed", str(binary)).splitlines()
        for name in needed:
            if "/" in name:
                output("patchelf", "--replace-needed", name, Path(name).name, str(binary))
        for name, filename in re.findall(r"^\s*(.+?) => (/.+?) \(", dependencies, re.MULTILINE):
            library = Path(name).name
            if SYSTEM.match(library):
                continue
            source = Path(filename)
            dest = root / library
            if library not in copied:
                shutil.copy2(source, dest)
                dest.chmod(dest.stat().st_mode | 0o200)
                copied.add(library)
                if licensed_prefix is None or not source.resolve().is_relative_to(licensed_prefix.resolve()):
                    sources.add(source)
                queue.append((source, dest))
            # 同一 SONAME 复用首次解析的副本，与动态加载器的单一库选择保持一致。
        binary.chmod(binary.stat().st_mode | 0o200)
    # 收集完成后才改写路径，避免 ldd 被半成品运行库影响。
    for binary in seen:
        relative = os.path.relpath(root, binary.parent)
        output("patchelf", "--set-rpath", "$ORIGIN" if relative == "." else "$ORIGIN/" + relative, str(binary))
    xkb_value = output("pkg-config", "--variable=xkb_base", "xkeyboard-config")
    if not xkb_value:
        raise SystemExit("xkeyboard-config pkg-config metadata does not define xkb_base")
    xkb = Path(xkb_value)
    for name in ("rules", "symbols", "keycodes", "types", "compat"):
        if not (xkb / name).is_dir():
            raise SystemExit(f"Missing xkeyboard-config runtime data: {xkb / name}")
    target = root / "share/X11/xkb"
    target.mkdir(parents=True, exist_ok=True)
    for name in ("rules", "symbols", "keycodes", "types", "compat", "geometry"):
        if (xkb / name).is_dir():
            copy_tree(xkb / name, target / name)
    sources.add(xkb / "rules/evdev")
    # 独立配置只使用稳定的 fontconfig 语法，不导入构建机的 conf.d 或 store 路径。
    fonts = root / "etc/fonts"
    fonts.mkdir(parents=True, exist_ok=True)
    (fonts / "fonts.conf").write_text('''<?xml version="1.0"?>
<!DOCTYPE fontconfig SYSTEM "urn:fontconfig:fonts.dtd">
<fontconfig>
  <dir>/usr/share/fonts</dir>
  <dir>/usr/local/share/fonts</dir>
  <dir>/run/current-system/sw/share/X11/fonts</dir>
  <dir>/run/current-system/sw/share/fonts</dir>
  <dir>/run/current-system/sw/share/fonts/truetype</dir>
  <dir>~/.nix-profile/share/fonts</dir>
  <dir>~/.local/state/nix/profile/share/fonts</dir>
  <dir>~/.local/share/fonts</dir>
  <dir>~/.fonts</dir>
  <dir prefix="xdg">fonts</dir>
  <include ignore_missing="yes">/etc/fonts/conf.d/00-nixos-cache.conf</include>
  <cachedir prefix="xdg">fontconfig</cachedir>
  <alias><family>sans-serif</family><prefer><family>Noto Sans</family><family>DejaVu Sans</family></prefer></alias>
  <alias><family>serif</family><prefer><family>Noto Serif</family><family>DejaVu Serif</family></prefer></alias>
  <alias><family>monospace</family><prefer><family>Noto Sans Mono</family><family>DejaVu Sans Mono</family></prefer></alias>
</fontconfig>
''', encoding="utf-8")
    licenses(root, sources)


if __name__ == "__main__":
    if len(sys.argv) != 3:
        raise SystemExit(f"Usage: {sys.argv[0]} STAGING_DIR DESTINATION_DIR")
    root, destination = (Path(value).resolve() for value in sys.argv[1:])
    if not root.is_dir() or root.is_relative_to(destination) or destination.is_relative_to(root):
        raise SystemExit("Linux staging and destination must be existing, separate directories")
    collect(root, [root / "arcana-world-overlay-native"])
    # 原生 helper 原本由 go build 直接覆盖；升级旧输出时接管这个明确的构建产物。
    publish(root, destination, ".arcana-native-files.json",
            build_outputs=(Path("arcana-world-overlay-native"),))
