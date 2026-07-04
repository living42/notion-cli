# 完成报告：按 Unix「一切皆文件」思想重新设计 notion-cli

## 修订历史

- 2026-07-04 修订 1：把 `mkdir` 命令改名为 `mkdb`（避免与 Unix 标准 `mkdir` 冲突），其他设计不变。重跑 `go test -count=1 ./...` 67.2s 全量通过 78 用例。
- 2026-07-04 修订 2：精简 `README.md` 仅保留基本介绍 / Installation / Quick Start / License 四块。446 行 → 75 行；移除 Features / Resource addressing / Commands Reference / Migration / Troubleshooting / Config File / Limitations 共 7 段。纯文档变更，未重跑 build / test。

## 背景

原 notion-cli 把 14 条 Notion API 直译命令（`fetch-page` / `list-block-children` / `trash-data-source` …）暴露给用户，名字 API 味重、同类操作拆散、输出格式不利于与 Unix 工具链组合。需求来源：`idea.md`。

本次重新设计把 Notion 资源视作可寻址的「文件」，用 Unix 文件操作动词做命令名（9 条），合并同类操作，引入类型前缀寻址，输出默认紧凑可管道化，并通过注入式接缝补齐测试覆盖。

## 关键交付物

### 命令集（9 条 Unix 文件动词命令）

| 命令 | 语义 | 替代的旧命令 |
|---|---|---|
| `configure` | 配置 profile | `configure` |
| `find [QUERY]` | 搜索工作区 | `search` |
| `ls PATH` | 列出子项 | `list-block-children` + `query-data-source`(基础) |
| `read PATH` | 读取资源 | `fetch-page` / `fetch-database` / `fetch-data-source` |
| `mkdb TITLE --parent` | 创建数据库 | `create-database` |
| `write TITLE --parent` | 创建页面 | `create-page` |
| `edit PAGE_REF` | 修改页面内容 | `update-page` |
| `mv PAGE_REF --parent` | 移动页面 | `move-page` |
| `rm PATH` | 移到回收站 | `trash-page` / `trash-database` / `trash-data-source` |

资源用 `[type:]id` 寻址：`page:` / `db:`(或 `database:`) / `ds:`(或 `data-source:` / `data_source:`)；裸 ID 默认按 page。

输出三档：默认紧凑可管道化文本 / `-l` 富文本 / `--json` 原始 JSON。

### 代码

- **修改**：`api.go`（注入 `apiBaseURL` / `httpClient`；`buildCreatePageBody` / `buildMovePageBody` 改为 `(kind, id, …)`；`firstDataSourceID` 从 cmd.go 移入）、`cmd.go`（9 条新命令；`cmdXxx` 返回 `(string, error)`）、`format.go`（`prettyJSON` 由 `printPrettyJSON` 改名；新增 `formatFindCompact` / `formatLsCompact` / `formatLsLong` / `pageMetadataLines`）、`config.go`（`cmdConfigure` 改签名为 `(string, error)`）、`e2e_test.go`（适配新命令；删除两个因「单 `--parent` flag」失效的用例）、`README.md`（重写；修订 2 精简为 75 行：基本介绍 / Installation / Quick Start / License）
- **新增**：`ref.go`（`parseResourceRef` / `kindLabel`）、`ref_test.go`（单元测试）、`integration_test.go`（`httptest` 集成测试，无需 Notion 凭据）

### 文档

- `docs/issues/2607041827-redesign-cli-with-unix-file-philosophy/idea.md`
- `docs/issues/2607041827-redesign-cli-with-unix-file-philosophy/user-story.md`
- `docs/issues/2607041827-redesign-cli-with-unix-file-philosophy/tech-design.md`
- `docs/issues/2607041827-redesign-cli-with-unix-file-philosophy/mistakes.md`
- `docs/issues/2607041827-redesign-cli-with-unix-file-philosophy/worker.md`（`ark-coding/glm-5.2`）
- `docs/issues/2607041827-redesign-cli-with-unix-file-philosophy/completed.md`（本文件）
- `/Users/lizeqing/Code/notion-cli/README.md`（用户面向文档）

## 测试结果

`go test -count=1 ./...` 在真实 Notion API 下 67.2s 通过（修订 1 重跑；初版 61.3s）：

- 总计 **78 PASS / 0 FAIL / 0 SKIP**
- 单元（`ref_test.go`）：10 个（`TestParseResourceRef_*`）
- 集成（`integration_test.go`，`httptest` 模拟）：19 个（覆盖 read/ls/find/mkdb/write/edit/mv/rm 的分发、请求方法/路径/请求体、类型前缀、`--slice`/`--filter` 误用、404 透传等）
- E2E（`e2e_test.go`，真实 Notion API）：49 个（含 9 个 `TestE2E_SubcommandHelp/{configure,find,ls,read,mkdb,write,edit,mv,rm}` 子用例；覆盖 `--help` 列出 9 条新命令、`-p/--profile` 前后置、参数校验、读/写/移动/编辑/回收 happy+错误路径）

`go vet ./...`：clean。`gofmt -l`：clean。

冒烟验证：`./notion-cli -p e2e find e2e --page-size 2` 返回正确紧凑输出 `page 393d4d83-… notion-cli e2e test` + metadata 页脚；旧命令名 `search` / `fetch-page` 全部返回 `unknown command`。

## 本地访问入口

- 二进制：`/Users/lizeqing/Code/notion-cli/notion-cli`（11.9 MB，Go 1.24，依赖 `spf13/cobra`）
- 启动方式：直接运行 `./notion-cli`；`./notion-cli --help` / `./notion-cli <cmd> --help` 可用
- 9 条子命令：`configure` / `find` / `ls` / `read` / `mkdb` / `write` / `edit` / `mv` / `rm`
- 全局 flag：`-p/--profile`（前后均可）、`-v/--version`、`-h/--help`

## 已知遗留与可扩展

按 tech-design.md「后续可扩展」列出，本次均未实现：
- 旧命令名兼容别名（Q1=否）
- 裸 ID 404 时自动回退探测 db/ds（Q2=否）
- 回收站恢复
- FUSE 真实挂载
- `-0` / 制表符分隔供 `xargs`（Q3=空格）

## 备注

本次为破坏性变更：14 条旧命令名全部移除。旧脚本需按 tech-design.md 附录的「旧→新命令迁移表」迁移（修订 2 已从 README 移除该表，迁移信息保留在 tech-design.md 附录）。无旧别名（按 Q1 决策）。
