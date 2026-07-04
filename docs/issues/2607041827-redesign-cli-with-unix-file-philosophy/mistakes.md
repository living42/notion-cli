# 实施过程记录（mistakes.md）

本文件记录 build 阶段遇到的问题与修正，按 tech-design.md 实施步骤推进。

## Step 1：接缝改造
- 无问题。api.go 注入 `apiBaseURL`/`httpClient`，`go build` + `go vet` 通过。

## Step 2：ref 解析
- 无问题。`ref.go` + `ref_test.go`，11 个用例通过。

## Step 3+4：重写命令树 + 输出格式
- **问题 1：`buildCreatePageBody` 重复声明。**
  - 原因：用三段 edit 替换 api.go 时，第一段把 `buildCreatePageParent` 替换成了新的 `buildCreatePageBody`，第二段又把旧的 `buildCreatePageBody` 替换成新的 `buildCreatePageBody`，导致两份同名函数。
  - 修正：删除 `extractTitlePropertyName` 与 `buildMovePageBody` 之间那份重复的 `buildCreatePageBody`。
- **问题 2：`firstDataSourceID` 未定义。**
  - 原因：该函数原在旧 cmd.go 中，重写 cmd.go 时未保留；但 `cmdMkdir` 仍引用它。
  - 修正：把 `firstDataSourceID` 移到 api.go（与 `asString` 等辅助函数放一起）。
- **问题 3：`cmdConfigure` 返回值数量不匹配。**
  - 原因：config.go 的 `cmdConfigure` 仍返回 `error`，但新 cmd.go 按 `(string, error)` 调用。
  - 修正：把 `cmdConfigure` 签名改为 `(string, error)`，内部交互式打印不变、返回 `""`。
- 无设计偏离；上述均为实现层修正。

## Step 5：集成测试
- **问题 4：`TestIntegration_EditReplace` 断言「expected PATCH, got GET」。**
  - 原因：handler 对每个请求都记录 `rec`，edit 流程在 PATCH 之后还会 GET `/v1/pages/{id}` 取 pageMeta，于是 `rec` 被最后一次 GET 覆盖。
  - 修正：只在 `/v1/pages/{id}/markdown` 路径上记录 `rec`。
- **问题 5：`TestIntegration_MvPostsParent` 报「Invalid page ID」。**
  - 原因：测试用的父级 ID `pppppppp-...` 含非十六进制字符 `p`，`normalizeNotionID` 拒绝。
  - 修正：改用合法十六进制 UUID `44444444-4444-4444-4444-444444444444`。

## Step 6：更新 E2E
- 无问题。重写 e2e_test.go：setup helper 与所有用例改用新命令（`write`/`mkdir`/`rm`/`ls`/`read`/`edit`/`mv`/`find`），删除两个因「单 --parent flag」而失效的用例（CreatePageBothParents、MovePageBothParents）。真实 Notion API 全量通过。

## Step 7：README
- 无问题。重写命令参考 + 旧→新迁移表 + flag 变更说明。

## Step 8：本地部署验证
- 无问题。`go build -o notion-cli .` 成功；`--help`/子命令 `--help` 正常；真实 Notion 冒烟 `find e2e` 返回正确紧凑输出；旧命令名全部返回 "unknown command"。

## 收尾
- `gofmt -l` 曾报 cmd.go、integration_test.go 未规范（struct 变量组对齐），已 `gofmt -w` 修正。
- 最终：`go vet ./...` clean；`go test ./...` 通过（77 通过 / 0 失败 / 0 跳过，~54–62s 真实 Notion）。
