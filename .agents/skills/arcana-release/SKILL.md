---
name: arcana-release
description: "Arcana World 发版流程。用于版本升级、合并 dev 到 main、整理中英文更新日志、打版本标签及发布 GitHub Release。覆盖版本一致性、跨平台发行构建、发布状态核验、发行包校验和失败处理；只准备版本时不自动推送或发布。"
---

# Arcana World 发版

以仓库当前的构建文件和发布工作流为准，不复制某次发版的版本号、提交 ID、运行 ID 或校验和。以下仓库路径均相对项目根目录。

## 授权与边界

- 用户要求“发布某版本”时，执行该版本的合并、提交、分支与标签推送、流水线等待和 Release 核验。若只要求准备版本或整理日志，不执行推送和发布。
- 发版不包含重写已有历史、强制推送、移动已有标签或删除既有 Release。需要这些操作时，先说明影响并取得明确授权。
- 保留用户的无关改动。不要自动执行 `reset --hard`、`clean` 或 `stash`；有冲突的工作区先隔离处理或询问。
- 临时下载、解压目录、日志和验证记录放在 `.local/release-<tag>/`，不提交。用户 README 不用来保存发版过程。
- 提交信息遵守 `AGENTS.md` 的英文 Conventional Commits 硬约束。

## 1. 确认目标与发布状态

先读取：

- `AGENTS.md`
- `.github/workflows/release.yml`
- `Makefile`、`scripts/build.sh`
- `cmd/arcana-world/main.go`、`flake.nix`
- `CHANGELOG.md`、`CHANGELOG.en.md`

检查工作区、分支与 worktree，刷新远端信息，并确认 GitHub 身份可用：

```sh
git status --short
git branch -vv
git worktree list
git fetch origin --tags
gh auth status
```

从用户请求确定不带 `v` 的 `VERSION`，对应标签为 `TAG=v${VERSION}`。不要猜测缺失的版本号。默认开发分支为 `dev`、发布分支为 `main`；用户指定其他分支时按请求调整。

初始化后续命令使用的参数，并按发布工作流的版本规则校验输入：

```sh
: "${VERSION:?Set VERSION to the requested version without v}"
TAG="v${VERSION}"
REPO="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
```

检查本地和远端是否已有目标标签，以及目标 Release 是否已存在。已有版本先核对提交和发布状态，不重复创建、不覆盖。GitHub 请求失败不等于版本不存在；排除网络或权限问题后再继续。

## 2. 合并并统一版本

在工作区适合切换、分支未被其他 worktree 占用的前提下，将开发分支合并到发布分支，先不提交，以便把版本和日志一并纳入发布提交：

```sh
git switch main
git merge --no-ff --no-commit dev
```

若 Git 报告没有需要合并的提交，仍可按用户请求准备普通版本提交。不要为了制造合并记录改写历史。

冲突必须保留双方意图。更新日志中常见的情况是 `main` 已有上一正式版、`dev` 仍有“未发布”内容：保留上一正式版的完整历史，将开发分支的新内容整理为目标版本，不直接选择一侧覆盖另一侧。

同步当前项目版本：

| 位置 | 要求 |
|---|---|
| `cmd/arcana-world/main.go` 的 `version` | 使用不带 `v` 的目标版本，保证 `go run ... --version` 正确 |
| `flake.nix` 的 `arcana-world.version` | 与程序版本一致；检查浮层、TTS 和桌面包仍继承该值 |
| `scripts/build.sh`、`Makefile`、发布工作流 | 确认 `VERSION` 注入和去除标签前缀的路径一致，不再添加重复版本常量 |
| 用户文档和提示中的当前版本 | 必要时更新；动态 latest 链接无需改动 |

搜索遗漏时区分项目版本、依赖版本和协议版本。不要修改历史更新日志中的旧版本、Bilibili 客户端版本、IPC／数据库协议号或 Go 工具链版本来“统一版本号”。

## 3. 整理双语更新日志

- 在 `CHANGELOG.md` 和 `CHANGELOG.en.md` 顶部添加同一目标版本，日期使用实际发版日期。
- 标题格式分别为 `## [vX.Y.Z]：YYYY-MM-DD`、`## [vX.Y.Z]: YYYY-MM-DD`，与工作流的提取规则兼容。预发布版本保留完整后缀。
- 根据上一版本之后的实际提交整理新增、改进和修复；补齐遗漏的用户可见变化，不把开发笔记、skill 增补或每条内部重构都写成产品功能。
- 保留已发布章节和链接；为新版本添加 `[vX.Y.Z]: .../releases/tag/vX.Y.Z`。
- 两种语言的功能、风险和升级要求一致。主程序与辅助程序必须配套更新时，在升级提示中明确要求完整替换发行包。
- 使用 `.github/workflows/release.yml` 中的实际提取逻辑检查两个章节均非空、边界正确、相对链接正确转换。不要另写一套不兼容的提取规则。

## 4. 验证并提交

发布前运行仓库规定的检查和本机可支持的桌面构建：

```sh
make check
go test -tags tts_audio ./internal/tts/... ./cmd/arcana-world-tts
go run ./cmd/arcana-world --version
make desktop VERSION="$TAG"
```

检查构建后的主程序 `--version` 和 TTS helper `-help`。路径以实际构建输出为准，Windows 带 `.exe`。源码运行与打包程序都必须显示目标版本，不能只验证通过链接参数覆盖的发行版本。

缺少原生构建依赖时，明确记录本地验证边界，并以对应原生 CI 构建补充。编译通过不等于图形运行验证；本次修改过图形行为时，按 `AGENTS.md` 另做实际后端验证。

确认冲突全部解决，只暂存本次发布应包含的内容。发布提交可用 `release: prepare <tag>`；不要使用默认 `Merge branch ...` 信息。提交后记录完整提交 ID，确认开发分支与原发布分支均是其祖先。

## 5. 推送与等待发布

通常将 `dev` 快进到发布提交，使两个分支使用相同版本。若 worktree 或并发提交阻止快进，不强制覆盖，先处理该状态。

当前仓库的典型收尾顺序：

```sh
git switch dev
git merge --ff-only main
git switch main
git tag -a "$TAG" -m "release: publish $TAG"
git push --atomic origin main dev "$TAG"
```

执行前必须确认目标标签仍不存在、分支引用符合预期。推送被拒绝时检查远端变化，不改用强推。其他分支和 worktree 不在发布范围内。

标签推送会触发 `release.yml`。不要抢先手动创建 Release，以免与工作流冲突。按目标标签和完整提交 ID 找到本次运行，等待 checks、全部构建及 publish 完成；仅推送标签成功不算发布完成。

流水线和发布核验命令、失败处理见 [发布核验与恢复](references/github-release.md)。

## 完成条件

- 远端发布分支、已同步的开发分支与标签指向预期提交。
- 对应 Release 已公开，版本类型正确，双语正文与 changelog 提取结果一致。
- 当前构建矩阵的发行包齐全，另有 `checksums.txt`；所有资产上传完成且非空。
- 下载本机可运行的发行包，核对 SHA-256，再解压执行 `--version` 和 helper 帮助检查。
- 清理不再需要的临时程序、下载和解压目录；验证记录可留在 `.local/`。
- 最后报告 Release 链接、版本、提交、实际验证结果和未验证边界。只有工作区确实干净时才称其干净，不把用户保留的无关文件纳入提交。
