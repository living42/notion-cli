# 技术设计：按 Unix「一切皆文件」思想重新设计 notion-cli

## 背景

notion-cli 是一个 Go 编写的命令行工具（模块 `github.com/living42/notion-cli`，Go 1.24，依赖 `spf13/cobra`），通过 Notion REST API（`Notion-Version: 2025-09-03`）搜索、读取、创建、移动、更新、回收 Notion 的页面 / 数据库 / 数据源。

当前源码结构（仓库根目录扁平布局）：

| 文件 | 职责 |
| --- | --- |
| `main.go` | 入口；`normalizeProfileArgs` 把 `-p/--profile` 提到参数最前，再交给 cobra |
| `cmd.go` | cobra 命令树（14 条命令）+ 各 `cmdXxx` 处理函数（直接 `fmt.Println` 输出） |
| `api.go` | `notionGet/Post/Patch`（硬编码 `https://api.notion.com`，用 `http.DefaultClient`）、ID 归一化、构造请求体 |
| `format.go` | 各种 `formatXxxOutput`（返回字符串）、`printPrettyJSON`、`extractTitle` 等 |
| `config.go` | `~/.config/notion-cli/config.json` 多 profile 读写、`cmdConfigure` |
| `input.go` | `parseSlice`、`readCreateContent`/`readReplaceContent`、`parseJSONOption` |
| `errors.go` | `cliError`、`failf` |
| `e2e_test.go` | 端到端测试：构建二进制、用真实 Notion 凭据（`NOTION_CLI_E2E_TESTING_SECRET` + `NOTION_CLI_E2E_TESTING_ROOT_PAGE_LINK`）创建/回收 fixture |

面临的问题（来自 `idea.md`）：

- 命令名直译 API（`fetch-page`、`list-block-children`、`trash-data-source`…），API 味重、难记。
- 同类操作拆成多条命令（`fetch-*` ×3、`trash-*` ×3），数量多达 14 条。
- 输出 Markdown 与 JSON 混用，缺少紧凑、可管道化的默认文本模式，不利于与 `grep`/`xargs`/`sed` 等 Unix 工具组合。

需求来源：`idea.md` ——「按 unix 所有皆文件的思想设计」重新设计 CLI。

> 现状关键事实：本机已配置真实 E2E 凭据，`go test ./...` 当前在 ~54s 内通过（真实 API 调用 + fixture 建拆）。因此 E2E 不是 skip，必须在新命令下继续通过。

---

## 目标与非目标

### 功能目标

1. 用 9 条 Unix 文件动词命令替换 14 条 API 风格命令：`configure`、`find`、`ls`、`read`、`mkdb`、`write`、`edit`、`mv`、`rm`。
2. 合并同类命令：`fetch-{page,database,data-source}` → `read`；`trash-{page,database,data-source}` → `rm`；`list-block-children` + `query-data-source`(基础) → `ls`；`search` → `find`；`create-page` → `write`；`create-database` → `mkdb`；`update-page` → `edit`；`move-page` → `mv`。
3. 引入类型前缀寻址 `page:` / `db:` / `ds:`（含长别名），裸 ID 默认按页面。
4. 输出三档：默认紧凑可管道化文本、`-l` 富文本、`--json` 原始 JSON。
5. 引入可测试接缝：API base URL 与 HTTP client 可注入；`cmd*` 处理函数返回 `(string, error)` 而非直接打印。
6. 新增基于 `httptest` 的集成测试（无需 Notion 凭据）；更新 `e2e_test.go` 适配新命令并在真实 API 通过。
7. 重写 README：新命令参考 + 旧→新迁移表 + 示例。

### 非目标

1. 不新增任何 Notion API 能力。
2. 不实现 FUSE 真实挂载 / 虚拟文件系统。
3. 不引入 OAuth。
4. 不保留旧命令名兼容别名（干净重构）。
5. 不实现裸 ID 类型自动探测。
6. 不实现回收站恢复。
7. 不改动 config 文件格式、`install.sh`、GitHub release 工作流。
8. 不改动 Notion API 版本（`2025-09-03`）与既有请求体结构。

---

## 现状分析

- **数据模型**：无本地数据库；所有数据来自 Notion API 的 JSON 响应。资源类型：`page`（`/v1/pages/{id}`、`/v1/pages/{id}/markdown`）、`database`（`/v1/databases/{id}`）、`data_source`（`/v1/data_sources/{id}`、`/v1/data_sources/{id}/query`）、`block`（`/v1/blocks/{id}/children`）。数据库对象含 `data_sources[]` 数组。
- **API 接口现状**：`api.go` 的 `notionRequest` 硬编码 `https://api.notion.com` + `http.DefaultClient`，无注入接缝。`cmd*` 函数直接 `fmt.Println` / `printPrettyJSON`，无法在进程内捕获输出做测试。
- **命令现状**：14 条命令（见 README「Commands Reference」）。`-p/--profile` 通过 `main.go` 的 `normalizeProfileArgs` 手动提前到参数最前以兼容「子命令后跟 -p」。
- **测试现状**：仅有 `e2e_test.go`，依赖真实凭据；`TestMain` 构建 fixture（在 root 页面下建 scratch 页面 + 测试数据库/数据源），逐用例跑二进制子进程，teardown 回收 root 下所有子项。无单元/集成测试，无 HTTP 注入接缝。
- **认证现状**：`~/.config/notion-cli/config.json` 多 profile，`NOTION_CLI_CONFIG_PATH` 可覆盖路径（e2e 用此隔离）。

---

## 总体设计

### 核心设计原则

1. **动词即命令**：用 `ls/read/mv/rm/mkdb/write/edit/find` 等 Unix 文件动词做命令名，降低记忆成本。
2. **同类合并**：一种意图一个命令（读→`read`、删→`rm`、列→`ls`），按资源类型内部分发。
3. **资源即路径**：资源用 ID 定位，类型前缀 `page:`/`db:`/`ds:` 区分类型；裸 ID 默认页面（最常见）。
4. **默认可管道化**：输出默认紧凑文本（一行一条），`-l` 给人看，`--json` 给机器看。
5. **复用现有 API 层**：不改动 Notion 端点与请求体结构，仅重构命令分发与输出格式。
6. **可测试**：注入 API base URL/HTTP client，`cmd*` 返回字符串，进程内可用 `httptest` 验证。

### 术语约定

- **资源引用（ref）**：命令中定位资源的字符串，形如 `[type:]id`。`type` ∈ {`page`, `db`/`database`, `ds`/`data-source`/`data_source`}；省略则默认 `page`。
- **紧凑输出**：一行一条记录，字段以空格分隔，形如 `type id title`。
- **富文本输出（`-l`）**：多行人类可读块（沿用现有 `formatXxx` 风格）。
- **原始输出（`--json`）**：Notion 原始 JSON（pretty）。

### 架构概述

```
用户命令
  └─ cobra 命令树（cmd.go：9 条动词命令）
       └─ parseResourceRef → (kind, id)        # ref.go：类型前缀解析
       └─ cmdXxx(...) (string, error)          # cmd.go：分发到 API
            ├─ notionGet/Post/Patch            # api.go：可注入 baseURL/httpClient
            └─ formatXxxOutput(...) string     # format.go：紧凑/-l/--json 三档
       └─ RunE: fmt.Println(out)              # 仅在命令层打印
```

---

## 详细设计

### 资源引用解析（ref.go，新增）

`parseResourceRef(raw string) (kind string, id string, err error)`：

- 输入形如 `page:abc…`、`db:abc…`、`database:abc…`、`ds:abc…`、`data-source:abc…`、`data_source:abc…` 或裸 `abc…`。
- 以首个 `:` 切分前缀与 ID；前缀归一化：
  - `page` → `page`
  - `db`、`database` → `database`
  - `ds`、`data-source`、`data_source` → `data_source`
  - 未识别前缀 → `cliError{"Unknown resource type prefix: X"}`
- 无 `:` → `kind="page"`（默认），`id=raw`。
- ID 经现有 `normalizeNotionID(id, kindLabel)` 归一化（接受 32-hex 或 UUID，统一为小写 UUID）。
- 返回 `(kind, normID, nil)`。

辅助：`kindLabel(kind) string` → `"page"|"database"|"data source"`，用于错误信息。

### API 层接缝（api.go，改动）

- 新增包级变量：
  ```go
  var apiBaseURL = "https://api.notion.com"
  var httpClient = &http.Client{}
  ```
- `notionRequest` 用 `apiBaseURL + path` 与 `httpClient.Do(req)`。
- 提供 `setAPIBase(url string)`（测试用，配合 `t.Cleanup` 还原）—— 或直接在测试里赋值 `apiBaseURL = ts.URL` 并 `t.Cleanup(func(){ apiBaseURL = "https://api.notion.com" })`。
- 既有 `notionGet/Post/Patch`、`normalizeNotionID`、`buildCreatePageBody`、`buildMovePageBody`、`buildUpdatePageBody`、`extractTitlePropertyName` 等保持不变（仅 `cmd*` 调用方变化）。

### 命令层（cmd.go，重写）

所有 `cmdXxx` 改为 `func cmdXxx(...) (string, error)`，返回输出字符串；cobra `RunE` 负责 `fmt.Println(out)`。`printPrettyJSON` 改名为 `prettyJSON(map) string`（返回字符串），调用方按需打印。

`NewCommand()` 重建命令树，仅注册 9 条命令。保留 `normalizeProfileArgs`（`-p` 前后兼容）与 `version`/`-v`。

#### 命令规格

**`configure [-p PROFILE]`** — 不变。`cmdConfigure` 已返回 error，调整为返回 `(string, error)`（把各 `fmt.Print` 收集为返回字符串，或保留交互式 stdin 提示直接打印、返回 `""`）。为简化，`configure` 保留直接打印（交互式提示），`RunE` 不额外打印；其余命令统一返回字符串。

**`find [QUERY] [-l] [--json] [--page-size N] [--start-cursor ID] [--sort-timestamp FIELD] [--sort-direction {ascending,descending}]`**
- 等价旧 `search`。请求体同现状（`query`/`sort`/`start_cursor`/`page_size`）。
- 输出：默认 `formatFindCompact(results)`（一行一条 `type id title`）；`-l` → 现有 `formatSearchOutput`（多行块 + metadata）；`--json` → `prettyJSON(data)`。

**`ls PATH [-l] [--json] [--page-size N] [--start-cursor ID] [--filter JSON] [--sorts JSON] [--in-trash] [--result-type TYPE]`**
- `parseResourceRef(PATH)`：
  - `page`：调 `list-block-children` 逻辑（自动分页聚合，沿用 `cmdListBlockChildren` 现有实现）。默认紧凑 `type id title`（块类型如 `child_page`/`child_database`/`paragraph`…，标题取 child_page/child_database 的标题，其他块无标题则空）。`--json` → 聚合后的 list JSON（同现状）。`-l` → 富文本块列表。
  - `database`：`notionGet("/v1/databases/{id}")`，取 `data_sources[]`，紧凑列出 `data_source <id> <name>`；`--json` → 数据库完整 JSON。
  - `data_source`：调 `query-data-source` 逻辑（POST `/v1/data_sources/{id}/query`，支持 `--filter`/`--sorts`/`--in-trash`/`--result-type`/`--page-size`/`--start-cursor`）。默认紧凑 `page <id> <title>`；`--json` → 原始查询响应 JSON。
- `--filter`/`--sorts` 仅对 `data_source` 有效（对 page/database 传则报错）。

**`read PATH [--slice N-M] [-l] [--metadata] [--json]`**
- `parseResourceRef(PATH)`：
  - `page`：`notionGet("/v1/pages/{id}")`（meta）+ `notionGet("/v1/pages/{id}/markdown")`（body）。默认输出 `convertNotionMarkdown(markdown)` 正文（可管道化，无页眉页脚）；`--slice N-M` 切行（沿用 `parseSlice`）；`-l` → 现有 `formatFetchOutput`（标题+URL+正文+metadata 页脚）；`--json` → 页面 meta JSON（pretty）。
  - `database`：`notionGet("/v1/databases/{id}")` → `prettyJSON`。
  - `data_source`：`notionGet("/v1/data_sources/{id}")` → `prettyJSON`。
- `--slice` 仅对 page 有效。

**`mkdb TITLE --parent PAGE_REF [--properties JSON]`**
- 等价旧 `create-database`。`--parent` 经 `parseResourceRef`（须为 page，否则报错）。请求体同现状（`parent.page_id`、`title`、`properties` 默认 `{"Name":{"title":{}}}`）。输出 `prettyJSON(resp)`（含 `data_sources[0].id`）；若响应缺 data_source 则 refetch（沿用现状）。

**`write TITLE --parent REF [--content TEXT] [--content-file PATH]`**
- 等价旧 `create-page`。`--parent` 经 `parseResourceRef`（page 或 data_source）。沿用 `extractTitlePropertyName`（data_source 父级时取 title 属性名）与 `buildCreatePageBody` + `readCreateContent`。输出 `formatCreatePageOutput(pageData)`。

**`edit PAGE_REF [--replace] [--content TEXT] [--content-file PATH] [--old X --new Y]... [--replace-all-matches] [--allow-deleting-content]`**
- 等价旧 `update-page`。`PAGE_REF` 须为 page。沿用 `buildUpdatePageBody`（`--replace` 整页替换 / `--old --new` 搜索替换）。输出 `formatUpdatePageOutput(...)`。

**`mv PAGE_REF --parent REF`**
- 等价旧 `move-page`。`--parent` 经 `parseResourceRef`（page 或 data_source）。沿用 `buildMovePageBody`（含 self-move 校验）。输出 `formatMovePageOutput(...)`。

**`rm PATH`**
- `parseResourceRef(PATH)`：
  - `page` → PATCH `/v1/pages/{id}` `{in_trash:true}` → `formatTrashPageOutput`。
  - `database` → PATCH `/v1/databases/{id}` → `formatTrashDatabaseOutput`。
  - `data_source` → PATCH `/v1/data_sources/{id}` → `formatTrashDataSourceOutput`。

### 输出格式（format.go，改动）

- 新增 `formatFindCompact(data) string`：遍历 `results`，每条 `objType id title`（取 `object`/`page`/`database`、`id`、`extractTitle`），用 `\n` 连接；末尾附 metadata 块（`has_more`/`next_cursor`/`request_id`），与现有 `formatSearchOutput` 一致的 `<!-- metadata -->` 风格。
- 新增 `formatLsCompact(kind string, data) string`：按 kind 渲染紧凑列表：
  - page（block children）：每条 `<blockType> <id> <title?>`，title 仅 child_page/child_database 有。
  - database：每条 `data_source <id> <name>`。
  - data_source：每条 `page <id> <title>`。
- `prettyJSON`：由 `printPrettyJSON` 改名，返回字符串。
- 既有 `formatSearchOutput`/`formatFetchOutput`/`formatCreatePageOutput`/`formatUpdatePageOutput`/`formatMovePageOutput`/`formatTrashXxxOutput`/`convertNotionMarkdown`/`extractTitle`/`extractIcon`/`formatParent`/`renderMetadataBlock` 保持不变（作为 `-l` 富文本与各命令输出复用）。
- `-l` 富文本：`find -l` 复用 `formatSearchOutput`；`ls -l`（page）复用现有块渲染（新增轻量 `formatLsLong` 按 block 类型渲染标题/类型/id）；`read -l` 复用 `formatFetchOutput`。

### 边界与校验

- `--parent` / `PATH` 类型不匹配时给出明确错误（如 `mkdb --parent db:<id>` → "mkdb parent must be a page"）。
- `--filter`/`--sorts`/`--slice` 用在不适用的 kind 时报错。
- 保留现有校验：`--page-size` 1–100、`--slice` `0<=N<=M`、`--old`/`--new` 数量一致、`--replace` 不与 `--old/--new` 混用、`mv` self-move 拦截、`create`/`move` 的「恰好一个父级」语义（由 `--parent` 单 flag + 类型前缀天然满足）。
- ID 校验沿用 `normalizeNotionID`。

### 兼容性说明

- **破坏性变更**：移除全部旧命令名。README 提供迁移表。无旧别名（非目标）。
- config 文件格式、`NOTION_CLI_CONFIG_PATH`、`install.sh`、release 工作流均不变。
- Notion API 版本与请求体结构不变。

---

## 交互设计

### 用户交互流程

- 读页面：`notion read <page_id>` → Markdown 正文；`notion read -l <page_id>` 看富文本。
- 浏览子项：`notion find "release"` 找页面 → `notion ls <page_id>` 看子页面/子数据库。
- 创建：`echo "# body" | notion write "Notes" --parent <page_id>`；`notion mkdb "Tracker" --parent <page_id>`。
- 改内容：`notion edit <page_id> --replace --content-file ./new.md`；`notion edit <page_id> --old Draft --new Published`。
- 移动/删除：`notion mv <page_id> --parent <new_page>`；`notion rm <page_id>` / `notion rm db:<id>`。

### 交互反馈

- 成功：命令输出对应文本/JSON，退出码 0。
- 失败：`cliError` 经 `failf` 打到 stderr，退出码 1（沿用现状）。
- 危险操作：`rm`（回收站）与 `edit --replace` 沿用现有行为（无额外确认，与旧 `trash-*`/`update-page --replace` 一致）。

### 辅助交互

- `--help`/`-h`：cobra 自动生成；每条子命令有 Usage。
- `-p/--profile`：前后均可（`normalizeProfileArgs` 保留）。

---

## 边界情况与异常处理

- **类型前缀缺失**：裸 ID 一律按 page；若实际是 database/data_source，page 端点返回 404，错误信息透传（提示用户加 `db:`/`ds:` 前缀——在错误文案里附带 hint）。
- **`ls` 多类型分发**：`--filter`/`--sorts` 仅 data_source；误用到 page/database 立即报错，不发请求。
- **分页**：`ls <page>` 自动聚合所有块子项（沿用现状）；`ls ds:`/`find` 单页 + `--start-cursor` 翻页（沿用现状）。
- **空结果**：紧凑输出打印空字符串或 `_No results._`（沿用 `formatSearchOutput` 的空态文案思路）。
- **HTTP 注入与并发**：测试中改 `apiBaseURL`/`httpClient` 为非线程安全（测试串行），生产路径不变。

---

## 迁移方案

### Schema 迁移

无数据库，不涉及。

### 数据回填

无本地数据，不涉及。

### 兼容性说明

见上文「兼容性说明」。破坏性变更：旧命令名移除；提供迁移表。

---

## 可观测性

- 沿用现状：错误经 `cliError`/`failf` 输出 stderr；富文本输出含 `<!-- metadata -->` 块（`request_id`/`has_more`/`next_cursor`/`page_id` 等）便于排障。
- `--json` 输出原始 Notion 响应，含 `request_id`。
- 无新增日志/指标。

---

## 测试计划

### 单元测试（`ref_test.go`、`api_test.go` 等，无需凭据）

- `parseResourceRef`：各前缀（`page`/`db`/`database`/`ds`/`data-source`/`data_source`）、裸 ID 默认 page、32-hex 与 UUID、非法前缀、非法 ID。
- `normalizeNotionID`、`parseSlice`、`parseJSONOption`、`buildUpdatePageBody`/`buildMovePageBody`/`buildCreatePageBody` 既有纯函数（补充覆盖）。

### 集成测试（`integration_test.go`，基于 `httptest`，无需凭据）

- 启 `httptest.Server` 模拟 Notion API，`t.Cleanup` 还原 `apiBaseURL`。
- 逐命令验证分发与输出：
  - `read page:<id>` / `read db:<id>` / `read ds:<id>` / `read --json` / `read --slice`。
  - `ls <page>` / `ls db:<id>` / `ls ds:<id> --filter` / `ls --json`。
  - `find` 紧凑/`-l`/`--json`。
  - `mkdb` / `write`（含 stdin）/ `edit --replace` / `edit --old/--new` / `mv` / `rm page:`/`db:`/`ds:`。
- 断言：请求方法/路径/请求体正确；输出含期望字段；校验类错误正确抛出。

### E2E 测试（`e2e_test.go`，真实 Notion API）

- 更新 `setupFixtures` 与各 helper 用新命令（`write`/`mkdb`/`rm`/`ls`）。
- 更新所有用例的命令名与输出断言（如 `# ✅ Created Page`、`page_id:`、`in_trash: true`、`mode: replace_content` 等保持兼容——这些富文本输出沿用现有 `formatXxx`，故断言大多不变；仅命令名与「`ls` 取代 `list-block-children`/`query-data-source`」「`read` 取代 `fetch-*`」「`rm` 取代 `trash-*`」「`find` 取代 `search`」「`write`/`mkdb`/`edit`/`mv`」需改）。
- 覆盖：help 列出 9 条新命令、profile 前后置、参数校验、读/写/移动/回收的 happy path 与错误路径。

### 回归

- release CI 跑 `go test ./...`：本机有凭据则全跑；CI 无凭据则 e2e skip、单元+集成仍跑（保证 CI 绿）。

---

## 实施步骤建议

1. **接缝改造**：`api.go` 加 `apiBaseURL`/`httpClient` 注入；`cmd.go` 把所有 `cmdXxx` 改为返回 `(string, error)`，`printPrettyJSON` → `prettyJSON`，cobra `RunE` 统一 `fmt.Println`。
   - 检查点：`go build` 通过；`go vet ./...` 通过。
2. **ref 解析**：新增 `ref.go`（`parseResourceRef` + `kindLabel`）与 `ref_test.go`。
   - 检查点：`go test -run TestParseResourceRef` 通过。
3. **重写命令树**：`cmd.go` 的 `NewCommand()` 只注册 9 条命令；实现 `cmdFind/cmdLs/cmdRead/cmdMkdb/cmdWrite/cmdEdit/cmdMv/cmdRm`（复用 `api.go` 既有构造函数与 `format.go` 既有富文本函数）。
   - 检查点：`go build`；`./notion-cli --help` 列出 9 条命令、无旧命令。
4. **输出格式**：`format.go` 加 `formatFindCompact`/`formatLsCompact`/`formatLsLong`；接好 `--json`/`-l` 分支。
   - 检查点：`go build`。
5. **集成测试**：新增 `integration_test.go`（httptest 覆盖各命令）。
   - 检查点：`go test -run TestIntegration` 通过（无需凭据）。
6. **更新 E2E**：改 `e2e_test.go` 命令名与断言。
   - 检查点：`go test ./...` 在真实凭据下通过。
7. **README**：重写命令参考 + 旧→新迁移表 + 示例。
   - 检查点：文档自洽。
8. **本地部署验证**：`go build -o notion-cli .`；`./notion-cli --help`、`./notion-cli read --help` 等可用。
   - 检查点：二进制可运行，访问入口为 `./notion-cli`。

---

## 结论

- **关键决策**：9 条 Unix 文件动词命令；类型前缀寻址 + 裸 ID 默认页面；三档输出（紧凑/`-l`/`--json`）；注入式测试接缝 + httptest 集成测试；干净重构不保留旧别名。
- **预期收益**：命令数 14→9，记忆负担与文档量下降；输出默认可管道化，与 Unix 工具链协同；测试不依赖外部凭据即可覆盖命令逻辑。
- **风险提示**：破坏性变更（旧脚本需迁移）；`ls` 多类型分发需文档清晰；裸 ID 默认 page 可能对 database/data_source 产生 404（靠前缀规避）。
- **后续可扩展**：旧命令别名、裸 ID 自动探测、FUSE 挂载、`--zero`/制表符分隔供 `xargs`、恢复回收站。

---

## 问题与待确认事项

- [ ] Q1：是否需要旧命令名作为隐藏废弃别名？（默认：否）
- [ ] Q2：`read` 裸 ID 404 时是否自动回退探测 db/ds？（默认：否）
- [ ] Q3：`ls` 紧凑输出分隔符用空格还是制表符？（默认：空格）

---

## 附录

- 旧→新命令迁移表：

| 旧命令 | 新命令 |
| --- | --- |
| `search` | `find` |
| `fetch-page` | `read` |
| `fetch-database` | `read db:<id>` |
| `fetch-data-source` | `read ds:<id>` |
| `list-block-children` | `ls <page_id>` |
| `query-data-source` | `ls ds:<id>` |
| `create-page` | `write` |
| `create-database` | `mkdb` |
| `update-page` | `edit` |
| `move-page` | `mv` |
| `trash-page` | `rm <id>` |
| `trash-database` | `rm db:<id>` |
| `trash-data-source` | `rm ds:<id>` |
| `configure` | `configure` |
