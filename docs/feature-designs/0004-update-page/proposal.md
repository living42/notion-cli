# Update Page Support

## Proposal

### Research Basis

This proposal is based on:

- `docs/feature-designs/0004-update-page/idea.md`
- the current repository structure (`notion`, `README.md`)
- Notion's API doc for page markdown updates: `https://developers.notion.com/reference/update-page-markdown.md`

The API doc materially changes the design direction from the original rough idea:

- `update_content` is **not** "send new page markdown"
- it is a **search-and-replace operation list**
- full page replacement is done with `replace_content`
- the endpoint returns a `page_markdown` object containing the updated page content

So the CLI proposal needs to model the actual API, not a simplified guess.

---

### Repo Observations

The current codebase is intentionally small:

| Observation | Impact |
|---|---|
| The CLI is a single executable Python script: `notion` | This feature should stay single-file and avoid introducing a larger package structure |
| Commands are flat `argparse` subcommands (`configure`, `search`, `fetch-page`) | `update-page` should be added as one more subcommand |
| HTTP helpers currently exist for `GET` and `POST` only | This feature needs a matching `PATCH` helper |
| Output is Markdown-first and friendly to both humans and LLMs | Write operations should preserve this style |
| README currently describes the tool as read-only | Docs must be updated if this feature lands |

---

## Feature Overview

Add a new command:

```bash
notion update-page PAGE_ID [OPTIONS]
```

This command will target Notion's documented endpoint:

```http
PATCH /v1/pages/{page_id}/markdown
```

and support the two modern request modes documented by Notion:

- `update_content` — targeted search-and-replace operations
- `replace_content` — replace the entire page body

The older API operations are documented by Notion but should stay out of scope for v1:

- `insert_content` (legacy)
- `replace_content_range` (legacy)

---

## Goals

- Add page content editing without changing the repo's lightweight architecture
- Support both recommended API modes: `update_content` and `replace_content`
- Keep the UX simple for shells, scripts, and LLM agents
- Produce readable Markdown output after the update
- Expose important safety controls from the API where needed

## Non-Goals

- Supporting legacy `insert_content`
- Supporting legacy `replace_content_range`
- Rich block-level editing abstractions
- Interactive text editors
- Diffs, previews, or conflict resolution
- Property updates unrelated to page markdown body

---

## What the Notion API Actually Requires

From the official doc:

### `update_content`

The request body must look like:

```json
{
  "type": "update_content",
  "update_content": {
    "content_updates": [
      {
        "old_str": "existing text",
        "new_str": "replacement text"
      }
    ]
  }
}
```

Important behaviors:

- `old_str` must match existing page content exactly
- if `old_str` is not found, Notion returns `validation_error`
- if `old_str` matches multiple places, Notion returns `validation_error` unless `replace_all_matches` is set to `true`
- up to 100 update operations can be sent in one request

### `replace_content`

The request body must look like:

```json
{
  "type": "replace_content",
  "replace_content": {
    "new_str": "# Entirely new page content"
  }
}
```

Important behaviors:

- this replaces the full page markdown body
- deleting child pages or databases is blocked unless `allow_deleting_content: true` is set

### Response shape

The endpoint returns a `page_markdown` object containing:

- `object`
- `id`
- `markdown`
- `truncated`
- `unknown_block_ids`

So unlike page metadata endpoints, this response does **not** primarily return title/url-focused page metadata.

### Permissions and constraints

The doc also notes:

- the integration must have **update content capabilities** or Notion returns `403`
- synced pages cannot be updated
- databases / non-page blocks are invalid targets
- child page/database deletion is protected by default

---

## Proposed CLI Design

A single command should support two modes.

### Command

```bash
notion update-page PAGE_ID [OPTIONS]
```

### Mode A: targeted updates (`update_content`)

For search-and-replace edits.

#### Proposed flags

| Option | Description |
|---|---|
| `--old TEXT` | Existing string to match |
| `--new TEXT` | Replacement string |
| `--replace-all-matches` | Replace all occurrences for each update pair |
| `--allow-deleting-content` | Pass through the API safety override |

#### Usage

```bash
notion update-page <page_id> \
  --old "Status: Draft" \
  --new "Status: Published"
```

For multiple updates:

```bash
notion update-page <page_id> \
  --old "Q1" --new "Q2" \
  --old "draft" --new "final"
```

#### Validation rules

- `--old` and `--new` counts must match
- at least one pair must be provided in update mode
- update mode and replace mode are mutually exclusive

### Mode B: full replacement (`replace_content`)

For replacing the entire page body.

#### Proposed flags

| Option | Description |
|---|---|
| `--replace` | Switch from targeted-update mode to full-replace mode |
| `--content TEXT` | Replacement markdown |
| `--content-file PATH` | Load replacement markdown from a file |
| `--allow-deleting-content` | Pass through the API safety override |

If no `--content` or `--content-file` is given, the command should read from piped `stdin`.

#### Usage

```bash
notion update-page <page_id> --replace --content "# New Title\n\nNew body"
```

```bash
notion update-page <page_id> --replace --content-file ./page.md
```

```bash
cat ./page.md | notion update-page <page_id> --replace
```

### Why split the UX this way

This maps directly to Notion's real API model:

| User intent | API mode | CLI shape |
|---|---|---|
| Change specific text in-place | `update_content` | `--old` / `--new` pairs |
| Rewrite the whole page | `replace_content` | `--replace` + content input |

That makes the CLI intuitive while still being faithful to the documented endpoint.

---

## Request Mapping

### Targeted update request

Example CLI:

```bash
notion update-page <page_id> \
  --old "foo" --new "bar" \
  --old "hello" --new "goodbye" \
  --replace-all-matches
```

Proposed HTTP body:

```json
{
  "type": "update_content",
  "update_content": {
    "content_updates": [
      {
        "old_str": "foo",
        "new_str": "bar",
        "replace_all_matches": true
      },
      {
        "old_str": "hello",
        "new_str": "goodbye",
        "replace_all_matches": true
      }
    ]
  }
}
```

If `--allow-deleting-content` is provided:

```json
{
  "type": "update_content",
  "update_content": {
    "content_updates": [...],
    "allow_deleting_content": true
  }
}
```

### Full replacement request

Example CLI:

```bash
notion update-page <page_id> --replace --content-file ./page.md
```

Proposed HTTP body:

```json
{
  "type": "replace_content",
  "replace_content": {
    "new_str": "...file contents..."
  }
}
```

If `--allow-deleting-content` is provided:

```json
{
  "type": "replace_content",
  "replace_content": {
    "new_str": "...file contents...",
    "allow_deleting_content": true
  }
}
```

---

## Output Proposal

The CLI should **not** print the updated page content.

Even though the endpoint returns a `page_markdown` object, this is still a write command. Its stdout should stay concise and focus on whether the update succeeded.

That is preferable because:

- write commands should return a compact success summary
- large page bodies would create noisy stdout
- callers that want verification can explicitly run `fetch-page`
- it avoids mixing write confirmation with read output

### Proposed output shape

Print a short Markdown summary:

```markdown
# ✅ Updated Page
- **Title:** Engineering Roadmap
- **URL:** https://www.notion.so/Engineering-Roadmap-def456...
- **Page ID:** def456...
- **Mode:** update_content
- **Truncated:** false
- **Unknown block IDs:** []

---

<!-- metadata
page_id: def456...
mode: update_content
truncated: false
unknown_block_ids: []
-->
```

### Metadata note

The page-markdown response is documented to include:

- `id`
- `markdown`
- `truncated`
- `unknown_block_ids`

It does **not** document title or URL.

So if we want a useful human-readable summary with title and URL, the command should also perform:

```http
GET /v1/pages/{page_id}
```

This gives us:

- title
- icon
- URL

while the update endpoint provides update-result metadata such as:

- page id
- truncated
- unknown block ids

### Read-back behavior

If callers want to inspect the resulting markdown, they should run:

```bash
notion fetch-page <page_id>
```

Keeping read-back explicit makes `update-page` easier to compose in scripts.

---

## Error Handling

The current script prints API failures to `stderr` and exits non-zero. `update-page` should follow the same pattern.

### Client-side validation

Reject early for:

- mixing update-mode flags with replace-mode flags
- `--replace` with no content source and no piped stdin
- mismatched numbers of `--old` and `--new`
- missing update operations in update mode
- unreadable `--content-file`

### Relevant server-side errors from the doc

| Status/code | Meaning |
|---|---|
| `400 validation_error` | selection not found, ambiguous match, invalid target, synced page, or protected deletion |
| `403 restricted_resource` | integration lacks update content capability |
| `404 object_not_found` | page missing or not accessible |
| `409 conflict_error` | update conflict |
| `429 rate_limited` | rate limited |

The CLI does not need custom remapping yet; printing Notion's error body is enough.

---

## Implementation Plan

### 1. Add HTTP PATCH helper

In `notion`, add:

```python
def notion_patch(path: str, secret: str, body: dict) -> dict:
    ...
```

It should mirror the behavior of `notion_get()` and `notion_post()`.

### 2. Add argument parsing for `update-page`

Add a new subparser with:

- positional `page_id`
- `--replace`
- `--content`
- `--content-file`
- repeatable `--old`
- repeatable `--new`
- `--replace-all-matches`
- `--allow-deleting-content`

### 3. Add request-building helpers

Suggested helpers:

```python
def read_replace_content(args) -> str:
    ...


def build_update_page_body(args) -> tuple[str, dict]:
    ...
```

Responsibilities:

- decide whether the command is in `update_content` or `replace_content` mode
- validate flag combinations
- build the exact documented request body

### 4. Add command handler

```python
def cmd_update_page(args) -> None:
    ...
```

Responsibilities:

- load config
- build request body
- call `PATCH /v1/pages/{page_id}/markdown`
- fetch page metadata via `GET /v1/pages/{page_id}`
- print a compact success summary rather than the updated markdown body

### 5. Add a dedicated update formatter

The existing code already has useful helpers for page metadata:

- `extract_title(...)`
- `extract_icon(...)`

But `update-page` should use a dedicated formatter, e.g.:

```python
def format_update_page_output(update_data: dict, page_meta: dict, mode: str) -> str:
    ...
```

This keeps write output intentionally different from `fetch-page` and avoids printing the updated markdown body.

---

## Documentation Changes

### `README.md`

Update these sections:

1. **Features**
   - mention page content updates
2. **Quick Start**
   - add one `update-page` example
3. **Commands Reference**
   - document both targeted update mode and replace mode
4. **Limitations**
   - remove the claim that the tool is fully read-only
   - clarify that property/block-level editing is still limited
5. **Troubleshooting**
   - add `403` note about missing update content capabilities

### Example README usage

```bash
# Targeted replacement
notion update-page <page_id> --old "Status: Draft" --new "Status: Published"

# Replace the entire page from a file
notion update-page <page_id> --replace --content-file ./page.md
```

---

## Design Rationale

| Decision | Rationale |
|---|---|
| Support only `update_content` and `replace_content` | These are the current recommended API modes |
| Use `--old` / `--new` pairs for targeted edits | Mirrors the actual `content_updates` structure closely |
| Use `--replace` for whole-page replacement | Keeps destructive behavior explicit |
| Support file / inline / stdin for replacement text | Makes large-content workflows practical |
| Print a compact success summary | Keeps write-command output concise and script-friendly |
| Keep implementation in the single `notion` script | Preserves the repo's simplicity |

---

## Backward Compatibility

This is an additive change:

- `configure`, `search`, and `fetch-page` remain unchanged
- config format stays the same
- existing output formats stay the same

The only functional boundary change is that the CLI becomes capable of writing page content.

---

## Summary

After reviewing the official Notion documentation, the correct design is:

- add `notion update-page PAGE_ID`
- support **targeted search-and-replace** via `update_content`
- support **full page replacement** via `replace_content`
- do **not** model `update_content` as raw replacement markdown
- keep `update-page` output concise by printing a success summary instead of the updated page body

This keeps the CLI aligned with the real API, preserves the repo's lightweight architecture, and adds useful write capability without overcomplicating the implementation.