# 用户故事：按 Unix「一切皆文件」思想重新设计 notion-cli

## 用户故事

作为 notion-cli 的日常使用者，我想要用熟悉的 Unix 文件操作动词（`ls`、`read`、`mv`、`rm`、`mkdb`、`write`、`edit`、`find`）来操作 Notion 资源，以便无需记忆直译自 API 的命令名（`fetch-page`、`list-block-children`、`trash-data-source` 等），并能通过管道把页面内容串联到其他 Unix 工具中。

## 描述

当前 notion-cli 的命令设计基本是 Notion API 的直译：每种资源类型都有独立的 `fetch-*` / `trash-*` 命令，列表操作叫 `list-block-children`，搜索叫 `search`，创建叫 `create-page` / `create-database`。这些名字 API 味重、数量多（14 条）、且输出不利于管道组合（JSON 与 Markdown 混用、缺少紧凑文本模式）。

本次重新设计遵循 Unix「一切皆文件」思想，把 Notion 资源（页面、数据库、数据源、块）视作可通过路径定位的「文件」，用经典文件操作动词作为命令名，合并职责相同的命令，并让输出默认为可管道化的紧凑文本。资源用 ID 定位，可选类型前缀 `page:` / `db:` / `ds:` 区分资源类型；裸 ID 默认按页面处理（最常见场景）。

设计后的命令集（共 9 条）：

| 新命令 | 替代的旧命令 | 语义 |
| --- | --- | --- |
| `configure` | `configure` | 配置 profile（不变） |
| `find [QUERY]` | `search` | 搜索工作区 |
| `ls PATH` | `list-block-children` + `query-data-source`(基础) + 新增「列数据库的数据源」 | 列出资源的子项 |
| `read PATH` | `fetch-page` + `fetch-database` + `fetch-data-source` | 读取资源内容 |
| `mkdb TITLE --parent` | `create-database` | 创建数据库（容器，类比目录） |
| `write TITLE --parent` | `create-page` | 创建页面（类比文件） |
| `edit PAGE_REF` | `update-page` | 修改页面内容（整页替换或搜索替换） |
| `mv PAGE_REF --parent` | `move-page` | 移动页面到新父级 |
| `rm PATH` | `trash-page` + `trash-database` + `trash-data-source` | 把资源移入回收站 |

## 验收标准

- [ ] AC1：`notion --help` 列出且仅列出 9 条新命令（`configure`、`find`、`ls`、`read`、`mkdb`、`write`、`edit`、`mv`、`rm`），不再出现旧 API 风格命令（`search`、`fetch-*`、`create-page`、`create-database`、`list-block-children`、`query-data-source`、`move-page`、`update-page`、`trash-*`）。
- [ ] AC2：`read <page_id>` 输出页面 Markdown 正文（可管道化）；`read -l <page_id>` 额外含标题/URL/metadata 页脚；`read db:<id>` 输出数据库 pretty JSON；`read ds:<id>` 输出数据源 pretty JSON；`read --json <page_id>` 输出页面原始 JSON。
- [ ] AC3：`ls <page_id>` 列出块子项（紧凑 `type id title`，自动分页）；`ls db:<id>` 列出数据库的数据源；`ls ds:<id>` 列出数据源行（支持 `--filter`/`--sorts`）；`--json` 输出原始 JSON；`-l` 输出富文本。
- [ ] AC4：`find [query]` 搜索工作区，默认紧凑一行一条；`-l` 富文本；`--json` 原始 JSON；支持 `--page-size`/`--start-cursor`/`--sort-timestamp`/`--sort-direction`。
- [ ] AC5：`mkdb "Title" --parent <page_id>` 创建数据库，输出含 `data_sources[0].id` 的 JSON；`--properties` 支持自定义 schema。
- [ ] AC6：`write "Title" --parent <page_id|ds:id>` 创建页面；支持 `--content` / `--content-file` / stdin 提供正文。
- [ ] AC7：`edit <page_id> --replace --content X`（或 `--content-file`/stdin）整页替换；`edit <page_id> --old X --new Y [--replace-all-matches]` 搜索替换；`--allow-deleting-content` 透传。
- [ ] AC8：`mv <page_id> --parent <new_parent>` 移动页面；`--parent` 支持类型前缀（页面或数据源）。
- [ ] AC9：`rm <page_id>` / `rm db:<id>` / `rm ds:<id>` 分别把页面/数据库/数据源移入回收站，输出含 `in_trash: true`。
- [ ] AC10：类型前缀 `page:` / `db:` / `ds:`（及长别名 `database:` / `data-source:`）可正确解析并分发到对应 API；裸 ID 默认按页面处理。
- [ ] AC11：`-p/--profile` 在子命令前或后均可使用（保留现有 `normalizeProfileArgs` 行为）。
- [ ] AC12：`go build` 通过；单元测试（纯函数）通过；集成测试（httptest 模拟 Notion API）通过；E2E 测试（真实 Notion API）通过。
- [ ] AC13：README 更新为新命令参考，并附「旧命令 → 新命令」迁移表与示例。
- [ ] AC14：`notion configure` 行为与旧版一致。

## 范围

### 包含范围

- 把 14 条 API 风格命令重命名为/合并为 9 条 Unix 文件动词命令。
- 合并：`fetch-*` → `read`；`trash-*` → `rm`；`list-block-children` + `query-data-source`(基础) → `ls`；`search` → `find`；`create-page` → `write`；`create-database` → `mkdb`；`update-page` → `edit`；`move-page` → `mv`。
- 引入类型前缀寻址（`page:` / `db:` / `ds:`），裸 ID 默认页面。
- 输出默认紧凑可管道化文本，`-l` 富文本，`--json` 原始 JSON。
- 引入可测试接缝（可注入 API base URL / HTTP client；`cmd*` 函数返回 `(string, error)`），新增基于 `httptest` 的集成测试。
- 更新 `e2e_test.go` 适配新命令并在真实 Notion API 上通过。
- 重写 README（命令参考 + 迁移表 + 示例）。

### 排除范围

- 不新增任何 Notion API 能力（不增加当前工具不支持的操作）。
- 不实现 FUSE 真实挂载 / 虚拟文件系统（仅命令动词与寻址风格借鉴 Unix）。
- 不引入 OAuth。
- 不保留旧命令名作为兼容别名（干净重构；仅提供迁移表）。如需别名可后续追加。
- 不保留 `touch` / `cat` 命令名（本设计的中间命名已彻底移除，不留别名）。
- 不实现「裸 ID 自动探测类型」（用前缀显式区分；裸 ID 一律按页面）。
- 不实现从回收站恢复（仍通过 Notion UI）。
- 不改动 config 文件格式、install.sh、GitHub release 工作流。

## 问题

- [ ] Q1：是否需要保留旧命令名作为隐藏的废弃别名以便平滑迁移？（本设计的默认决策：否，干净重构 + 迁移表。）
- [ ] Q2：`read` 对裸 ID 是否需要在 404 时自动回退探测 database / data_source？（本设计的默认决策：否，统一用类型前缀。）
- [ ] Q3：`ls` 默认紧凑输出的列分隔符用空格还是制表符？（本设计的默认决策：空格对齐，`--json` 给原始数据；若需 `xargs` 友好可后续加 `-0`/制表符选项。）
