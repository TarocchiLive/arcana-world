# 发布核验与恢复

供 [发版流程](../SKILL.md) 在推送标签后使用。`TAG`、`REPO`、`RUN_ID`、发布提交 ID 和本地目录必须来自本次任务，不能沿用旧发版记录。

## 定位正确的运行

```sh
gh run list --workflow release.yml --branch "$TAG" \
  --json databaseId,headSha,status,conclusion,url
gh run view "$RUN_ID" --json headSha,status,conclusion,jobs,url
```

检查 `headSha` 等于本次标签解引用后的提交 ID。标签是 annotated tag 时，标签对象 ID 与提交 ID 不相同：

```sh
git rev-parse "$TAG^{commit}"
git ls-remote origin "refs/tags/$TAG" "refs/tags/$TAG^{}"
```

使用受管理的后台进程等待 `gh run watch "$RUN_ID" --exit-status`，或按工具提供的完成通知等待。不要用一次 `in_progress` 查询代替完成核验，也不要高频轮询。

## 检查构建矩阵和产物

每次从 `.github/workflows/release.yml` 的当前矩阵和打包逻辑推导预期资产。不要只检查资产数量。

当前矩阵为：

| 系统 | 架构 | 文件格式 |
|---|---|---|
| macOS | amd64、arm64 | `.tar.gz` |
| Linux | amd64、arm64 | `.tar.gz` |
| Windows | 386、amd64、arm64 | `.zip` |

命名为 `arcana-world-<goos>-<goarch>`，macOS 的 `goos` 是 `darwin`。当前共七个发行包，另有 `checksums.txt`；以后工作流改变时，以实际矩阵为准。

发行包应包含主程序、`libexec/arcana-world-overlay`、`libexec/arcana-world-tts` 和 `LICENSE`；Windows 可执行文件带 `.exe`。当前工作流还会提交 GitHub 构建来源证明，不要将其误算成必须存在的额外下载附件。

## 检查 Release 状态与正文

优先使用带身份的 GitHub CLI。查询元数据：

```sh
gh release view "$TAG" --repo "$REPO" \
  --json url,tagName,isDraft,isPrerelease,publishedAt,body,assets
gh api "repos/$REPO/releases/tags/$TAG"
```

完成时必须满足：

- `draft` 为 false，`published_at` 非空。
- 正式版不是 prerelease；按当前工作流，带预发布后缀的版本是 prerelease，不设为 latest。
- 正式版的 latest 状态符合本次发布目标，可用下列接口核对：

```sh
gh api "repos/$REPO/releases/latest" --jq .tag_name
```

- 正文是从同一标签提交下的中英文 changelog 提取的内容，含 `## 中文`、`## English`，没有误带其他版本章节。
- 资产名称集合与矩阵匹配，每项非空且状态为 `uploaded`；manifest 中的记录覆盖全部发行包。

不要仅凭 Release 页面存在便认定成功。草稿、缺资产、错误版本类型或不完整正文都需要继续处理。

## 校验实际下载包

下载适合当前宿主系统与架构的包及 manifest，目录放在 `.local/release-<tag>/downloads/`：

```sh
gh release download "$TAG" --repo "$REPO" \
  --pattern "$ASSET" --pattern checksums.txt --dir "$DOWNLOAD_DIR"
```

1. 在 `checksums.txt` 中找到与 `ASSET` 完全同名的记录。
2. 用宿主支持的 SHA-256 工具计算下载文件的哈希并比较：macOS 可用 `shasum -a 256`，Linux 可用 `sha256sum`，Windows 可用 `Get-FileHash -Algorithm SHA256`。只计算、不比较不能算通过。
3. 校验通过后解压到独立目录，检查包内路径完整，执行主程序 `--version`，确认版本等于不带 `v` 的目标版本；再运行 TTS helper `-help`。
4. 不能运行本机不支持的平台包，也不能把这一项冒烟验证写成全部平台的运行验证。
5. 保留必要结果记录，移除不再需要的下载、解压和临时脚本。不要删除用户原有文件。

## 失败与重试

### 网络或凭据问题

先区分 GitHub 请求错误、身份权限错误和对象确实不存在。对 EOF、暂时性网络失败可重试原只读请求；不要因为一次查询失败便重复创建标签或 Release。缺少权限时说明具体缺口，不索取用户在聊天中提供密码或令牌。

### checks 或 build 失败

先读取失败日志：

```sh
gh run view "$RUN_ID" --log-failed
```

确认是可重试的基础设施问题后，可重跑失败任务：

```sh
gh run rerun "$RUN_ID" --failed
```

源码、依赖或配置错误必须先定位修复。标签已经推送时，不得悄悄将同一标签移到修复提交；说明影响并按用户确认的新版本或恢复方案继续。不得跳过检查、删去失败的平台或上传占位文件来完成发版。

### publish 失败或已有草稿

先检查目标标签的提交、Release 是否为草稿、已有正文及附件，并读取 publish 日志。重复运行 `gh release create` 不会自动修复同名草稿。

只有确认草稿属于本次发布、来源提交和全部产物一致，且该补救操作在用户授权内时，才继续补齐并发布。不要覆盖来源不明的附件或删除已公开的 Release。发布后仍须重新核对正文、资产、版本类型和校验和。

### 推送被拒绝

保留本地工作，刷新远端引用并检查分歧或保护规则。不自动强推、不重写其他人的提交，也不绕过仓库保护。按实际原因协调合并、PR 或权限，再继续。

所有阶段都以实际完成状态报告结果：可继续处理的失败继续处理；需要用户授权或外部权限时，明确已完成部分与阻塞点，不提前宣布发布成功。
