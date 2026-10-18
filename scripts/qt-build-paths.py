#!/usr/bin/env python3
"""Qt 构建目录的规范化、所有权检查与清理。"""

from pathlib import Path
import shutil
import sys


MARKER = '.arcana-owned'
OWNER = 'arcana-qt-build-v1\n'


def canonical(value):
    # GitHub 环境文件按行解析，CMake 的列表分隔符也不能出现在前缀中。
    if any(character in value for character in ('\n', '\r', ';')):
        raise SystemExit('Qt paths must not contain newlines or semicolons.')
    path = Path(value).expanduser()
    if path.is_symlink():
        raise SystemExit(f'Refusing a symlink Qt directory: {path}')
    return path.resolve()


def check(prefix, work):
    if prefix == work or prefix in work.parents or work in prefix.parents:
        raise SystemExit('Qt install and work directories must be separate.')
    protected = (Path.home().resolve(), Path.cwd().resolve(), Path(__file__).resolve().parent.parent)
    for path in (prefix, work):
        if any(path == root or path in root.parents for root in protected):
            raise SystemExit(f'Refusing a protected Qt directory: {path}')
        if path.exists():
            if not path.is_dir():
                raise SystemExit(f'Qt path is not a directory: {path}')
            marker = path / MARKER
            owned = marker.is_file() and not marker.is_symlink() and marker.read_text() == OWNER
            if any(path.iterdir()) and not owned:
                raise SystemExit(f'Refusing an unowned Qt directory: {path}; choose a new empty directory.')


def main():
    if len(sys.argv) < 2:
        raise SystemExit('Usage: qt-build-paths.py canonical PATH | {check,reset} PREFIX WORK')
    mode, *values = sys.argv[1:]
    if mode == 'canonical' and len(values) == 1:
        print(canonical(values[0]).as_posix())
        return
    if mode not in ('check', 'reset') or len(values) != 2:
        raise SystemExit('Usage: qt-build-paths.py canonical PATH | {check,reset} PREFIX WORK')
    prefix, work = map(canonical, values)
    # 两个目录全部检查通过后才允许修改，防止半途失败破坏另一个目录。
    check(prefix, work)
    if mode == 'reset':
        for path in (prefix, work):
            if path.exists():
                shutil.rmtree(path)
            path.mkdir(parents=True)
            (path / MARKER).write_text(OWNER)


if __name__ == '__main__':
    main()
