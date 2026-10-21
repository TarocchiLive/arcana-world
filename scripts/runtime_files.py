"""按组件清单安全发布共享运行时文件。"""
import json
import os
from pathlib import Path
import shutil
import tempfile

MANIFESTS = (".arcana-qt-files.json", ".arcana-native-files.json", ".arcana-srt-files.json",
             ".arcana-tts-files.json")


def validate_parent(destination, relative):
    for parent in relative.parents:
        path = destination / parent
        if path.is_symlink() or (path.exists() and not path.is_dir()):
            raise SystemExit(f"Invalid runtime parent directory: {path}")


def owned_paths(destination, manifest):
    marker = destination / manifest
    if marker.is_symlink():
        raise SystemExit(f"Invalid runtime ownership manifest symlink: {marker}")
    if not marker.exists():
        return set()
    try:
        names = json.loads(marker.read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        raise SystemExit(f"Invalid runtime ownership manifest {marker}: {error}") from error
    if not isinstance(names, list) or any(not isinstance(name, str) for name in names):
        raise SystemExit(f"Invalid runtime ownership manifest: {marker}")
    paths = set()
    for name in names:
        relative = Path(name)
        if (not name or "\\" in name or ":" in name or relative.is_absolute()
                or any(part in ("", ".", "..") for part in name.split("/"))
                or name in MANIFESTS):
            raise SystemExit(f"Invalid runtime ownership path: {name!r}")
        validate_parent(destination, relative)
        paths.add(relative)
    return paths


def remove_owned(destination, paths):
    for relative in sorted(paths):
        validate_parent(destination, relative)
        path = destination / relative
        if path.is_dir() and not path.is_symlink():
            raise SystemExit(f"Refusing to remove runtime directory as a file: {path}")
    for relative in sorted(paths):
        (destination / relative).unlink(missing_ok=True)
    parents = {parent for path in paths for parent in path.parents if parent != Path(".")}
    for parent in sorted(parents, key=lambda path: len(path.parts), reverse=True):
        path = destination / parent
        if path.is_dir() and not any(path.iterdir()):
            path.rmdir()


def other_paths(destination, manifest):
    return set().union(*(owned_paths(destination, name) for name in MANIFESTS if name != manifest))


def remove(destination, manifest):
    remove_owned(destination, owned_paths(destination, manifest) - other_paths(destination, manifest))
    (destination / manifest).unlink(missing_ok=True)


def publish(source, destination, manifest, allow_unmanaged=False, build_outputs=()):
    """共享文件由所有使用组件认领，最后一个所有者移除时才删除。"""
    previous = owned_paths(destination, manifest)
    shared = other_paths(destination, manifest)
    paths = {path.relative_to(source) for path in source.rglob("*")
             if path.is_symlink() or path.is_file()}
    for relative in paths | previous:
        if str(relative) in MANIFESTS:
            raise SystemExit(f"Staging directory contains reserved manifest: {relative}")
        validate_parent(destination, relative)
        target = destination / relative
        if target.is_dir() and not target.is_symlink():
            raise SystemExit(f"Runtime file collides with directory: {target}")
        if (not allow_unmanaged and relative in paths and relative not in previous | shared
                and relative not in build_outputs and (target.exists() or target.is_symlink())):
            raise SystemExit(f"Existing unmanaged runtime file: {target}; use a clean/new output directory")
        path = source / relative
        if path.is_symlink() and (Path(os.readlink(path)).is_absolute()
                                  or not path.resolve().is_relative_to(source)):
            raise SystemExit(f"Runtime symlink escapes staging directory: {path}")
    destination.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".runtime-publish-", dir=destination) as temporary:
        prepared = Path(temporary)
        for relative in sorted(paths):
            target = prepared / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source / relative, target, follow_symlinks=False)
        marker = prepared / manifest
        marker.write_text(json.dumps(sorted(path.as_posix() for path in paths), indent=2) + "\n", encoding="utf-8")
        for relative in sorted(paths):
            target = destination / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            (prepared / relative).replace(target)
        remove_owned(destination, previous - paths - shared)
        marker.replace(destination / manifest)
