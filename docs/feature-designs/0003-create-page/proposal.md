# Create Page Support

## Proposal

### Research Basis

This proposal is based on:

- `docs/feature-designs/0003-create-page/idea.md`
- the current repository structure (`notion`, `README.md`)
- Notion's API doc for page creation: `https://developers.notion.com/reference/post-page.md`
- the API reference fetched via `curl`, as requested in the idea doc

The Notion API doc adds a few important constraints:

- page creation is done via `POST /v1/pages`
- the API supports multiple parent types: `page_id`, `database_id`, `data_source_id`, and `workspace`
- the API accepts page content as `markdown` or `children`
- `markdown` is mutually exclusive with `children`
- workspace-level page creation is only available for **public integrations**
- for **internal integrations**, a page or data source parent is required

That means this feature should stay faithful to the real API, but only expose the subset that fits this repo's current scope.

---

### Repo Observations

The current codebase is intentionally lightweight:

| Observation | Impact |
|---|---|
| The CLI is a single executable Python script: `notion` | This feature should stay single-file and avoid introducing a package refactor |
| Commands are flat `argparse` subcommands (`configure`, `search`, `fetch-page`, `update-page`) | `create-page` should be added as one more subcommand |
| The tool is Markdown-first | Creating a page from a Markdown string fits the existing UX well |
| The config model is a single Notion secret in `~/.config/notion-cli/config.json` | The command should reuse the same auth flow |
| The README still lists page creation as unsupported | Docs must be updated if this feature lands |
| The current tool does not model databases or data sources deeply | Database/data-source creation should stay out of scope for this feature |

---

## Feature Overview

Add a new command:

```bash
notion create-page TITLE [OPTIONS]
```

This command will target Notion's documented endpoint:

```http
POST /v1/pages
```

and support the scoped parent mode for v1:

- create a page under an existing page

The new page body will be supplied as Markdown.

### Why workspace support is out of scope

The Notion API does support workspace-level page creation, but only for **public integrations**.

This repo currently uses a single integration-secret model and is not designed around public-integration/OAuth workflows. So even though the API supports workspace parents, exposing that mode now would create a misleading CLI surface for most users.

Therefore, workspace-level creation should stay out of scope for this feature.

---

## Goals

- Add a simple `create-page` command without changing the repo's lightweight architecture
- Support creating a child page under a parent page
- Accept Markdown content directly, from a file, or from `stdin`
- Return concise, human-readable output suitable for scripts and LLM agents

## Non-Goals

- Creating workspace-level pages
- Creating pages under databases
- Creating pages under data sources
- Setting arbitrary page properties beyond the title needed for page-parent creation
- Building block trees via `children`
- Template-based page creation
- Property editing after creation
- Rich interactive editors or prompts

---

## What the Notion API Actually Requires

From the official doc:

### Endpoint

```http
POST /v1/pages
```

### Parent options

The API supports these parent shapes:

```json
{ "type": "page_id", "page_id": "..." }
```

```json
{ "type": "database_id", "database_id": "..." }
```

```json
{ "type": "data_source_id", "data_source_id": "..." }
```

```json
{ "type": "workspace", "workspace": true }
```

For this feature, only this parent type will be in scope:

- `page_id`

### Title/property rules

The API doc states:

- if the new page is a child of an existing page, `title` is the only valid property in `properties`
- if the new page is a child of a data source, property keys must match the data source schema

Since database/data-source support is out of scope here, this proposal should only construct the minimal `title` property payload.

### Content options

The API supports page body creation using:

- `children`
- `markdown`
- `template`

Important documented behavior:

- `markdown` is supported directly on page creation
- `markdown` is mutually exclusive with `children`
- newlines must be encoded correctly as `\n` in JSON
- using files or `stdin` is safer than shell-escaping long multiline strings

### Permissions and constraints

The doc also notes:

- the integration must have **Insert Content capabilities** on the target parent
- unsupported generated properties (`rollup`, `created_by`, etc.) cannot be set
- workspace parent creation is only available for public integrations
- internal integrations require a page or data source parent

Those last two points are additional justification for keeping workspace support out of scope in this CLI feature.

### Response shape

The endpoint returns a standard page object, not a page-markdown object.

That means the response naturally contains fields useful for a success summary, such as:

- `id`
- `url`
- `parent`
- `created_time`
- `last_edited_time`
- `properties`

But it does **not** return the created Markdown body in the same shape as `fetch-page`.

---

## Proposed CLI Design

### Command

```bash
notion create-page TITLE --parent-page-id UUID [OPTIONS]
```

### Arguments and flags

| Option | Description |
|---|---|
| `TITLE` | Title of the new page |
| `--parent-page-id UUID` | Create the new page under an existing page |
| `--content TEXT` | Inline Markdown body |
| `--content-file PATH` | Read Markdown body from a file |

If neither `--content` nor `--content-file` is given, the command should:

- read from piped `stdin` if available
- otherwise create a blank page

### Why this shape

This is a good fit for the current CLI because:

| Requirement | CLI choice |
|---|---|
| Parent page support | `--parent-page-id` |
| Markdown-first UX | `--content`, `--content-file`, or `stdin` |
| Simple invocation from shells/agents | Title as a positional argument |

### Parent validation rules

Require exactly one parent mode for v1:

- `--parent-page-id`

Reject early if it is missing.

This keeps the CLI explicit and avoids ambiguous defaults.

---

## Request Mapping

### Create page under another page

Example CLI:

```bash
notion create-page "Release Notes" \
  --parent-page-id 3c90c3cc-0d44-4b50-8888-8dd25736052a \
  --content-file ./release-notes.md
```

Proposed HTTP body:

```json
{
  "parent": {
    "type": "page_id",
    "page_id": "3c90c3cc-0d44-4b50-8888-8dd25736052a"
  },
  "properties": {
    "title": {
      "title": [
        {
          "text": {
            "content": "Release Notes"
          }
        }
      ]
    }
  },
  "markdown": "# Release Notes\n\nInitial draft"
}
```

### Blank page creation

If no content source is provided and no stdin is piped, omit `markdown` entirely:

```json
{
  "parent": {
    "type": "page_id",
    "page_id": "..."
  },
  "properties": {
    "title": {
      "title": [
        {
          "text": {
            "content": "New Page"
          }
        }
      ]
    }
  }
}
```

This keeps the command flexible while staying faithful to the API.

---

## Content Input Design

To stay consistent with `update-page --replace`, the command should support three body-input paths.

### Inline text

```bash
notion create-page "Draft" \
  --parent-page-id <page_id> \
  --content "# Draft\n\nHello"
```

### File input

```bash
notion create-page "Draft" \
  --parent-page-id <page_id> \
  --content-file ./draft.md
```

### Standard input

```bash
cat ./draft.md | notion create-page "Draft" --parent-page-id <page_id>
```

### Validation rules

- `--content` and `--content-file` are mutually exclusive
- when both are absent:
  - use `stdin` if present
  - otherwise create a blank page
- unreadable `--content-file` should fail early with a clear message

This gives the best ergonomics for both humans and automation.

---

## Output Proposal

Like `update-page`, `create-page` is a write command.

It should **not** print the page body after creation.

That is preferable because:

- write commands should stay concise
- the create endpoint returns a page object, not page markdown
- printing large content would create noisy stdout
- callers can explicitly run `notion fetch-page <page_id>` if they want to verify the body

### Proposed output shape

```markdown
# ✅ Created Page
- **Title:** Release Notes
- **URL:** https://www.notion.so/Release-Notes-abc123...
- **Page ID:** abc123...
- **Parent:** page `3c90c3cc-0d44-4b50-8888-8dd25736052a`
- **Created:** 2026-03-29T10:15:00.000Z
- **Last edited:** 2026-03-29T10:15:00.000Z

---

<!-- metadata
page_id: abc123...
parent: page_id:3c90c3cc-0d44-4b50-8888-8dd25736052a
request_id: req-123...
-->
```

### Metadata fields

| Field | Source |
|---|---|
| `Title` | page `properties` via existing title extractor |
| `URL` | `url` |
| `Page ID` | `id` |
| `Parent` | `parent` |
| `Created` | `created_time` |
| `Last edited` | `last_edited_time` |
| `request_id` | `request_id` |

---

## Error Handling

The current script prints API failures to `stderr` and exits non-zero. `create-page` should follow the same pattern.

### Client-side validation

Reject early for:

- missing `--parent-page-id`
- using both `--content` and `--content-file`
- unreadable `--content-file`
- empty title after trimming whitespace

### Relevant server-side errors from the doc

| Status/code | Meaning |
|---|---|
| `400 validation_error` | invalid parent, invalid properties, or invalid markdown |
| `403 restricted_resource` | integration lacks insert content capability |
| `404 object_not_found` | target parent missing or not accessible |
| `429 rate_limited` | rate limited |

The CLI does not need custom remapping yet; printing Notion's error body is sufficient.

---

## Implementation Plan

### 1. Add content reader helper

Add a helper similar to `read_replace_content()` but with different fallback behavior:

```python
def read_create_content(args) -> str | None:
    ...
```

Responsibilities:

- reject `--content` + `--content-file`
- return inline content if provided
- return file contents if provided
- return piped stdin if available
- return `None` if no content source is given

### 2. Add parent builder helper

Suggested helper:

```python
def build_create_page_parent(args) -> dict:
    ...
```

Responsibilities:

- validate that `--parent-page-id` is present
- build the `page_id` parent object

### 3. Add request builder

Suggested helper:

```python
def build_create_page_body(args) -> dict:
    ...
```

Responsibilities:

- build `parent`
- build the minimal `properties.title` payload
- include `markdown` only when content exists

### 4. Add command handler

```python
def cmd_create_page(args) -> None:
    ...
```

Responsibilities:

- load config
- build request body
- call `POST /v1/pages`
- print formatted success output

No follow-up fetch is required because the create response already includes the fields needed for a success summary.

### 5. Add formatter

Suggested formatter:

```python
def format_create_page_output(page_data: dict) -> str:
    ...
```

It should reuse existing helpers where appropriate:

- `extract_title(...)`
- `format_parent(...)`
- `extract_icon(...)` is probably unnecessary for write output but available if desired

### 6. Extend argument parsing

Add a new subparser:

- positional `title`
- `--parent-page-id`
- `--content`
- `--content-file`

---

## Documentation Changes

### `README.md`

Update these sections:

1. **Overview / Features**
   - mention page creation
2. **Quick Start**
   - add a `create-page` example
3. **Commands Reference**
   - document title, parent, and content options
4. **Use Cases**
   - include a scriptable page-creation example
5. **Limitations**
   - remove the claim that the tool cannot create pages
   - clarify that workspace/database/data-source creation is still out of scope

### Example README usage

```bash
# Create a blank child page
notion create-page "Scratchpad" --parent-page-id <page_id>

# Create a Markdown page from a file
notion create-page "Release Notes" --parent-page-id <page_id> --content-file ./release-notes.md
```

---

## Design Rationale

| Decision | Rationale |
|---|---|
| Use `POST /v1/pages` directly | Matches the official API exactly |
| Support only page parent creation in v1 | Matches the requested core use case while keeping scope tight |
| Keep workspace support out of scope | It depends on public integrations and would be misleading in the current CLI model |
| Use Markdown input, not block `children` | Better fit for the CLI's Markdown-first design |
| Accept file / inline / stdin content | Makes multiline content practical in shell workflows |
| Keep title as a positional argument | Simple and natural for CLI and LLM invocation |
| Print a compact success summary | Consistent with `update-page` and better for scripts |

---

## Backward Compatibility

This is an additive change:

- `configure`, `search`, `fetch-page`, and `update-page` remain unchanged
- config format stays the same
- existing output formats stay the same

The main product boundary change is that the CLI becomes able to create child pages.

---

## Summary

After reviewing the official Notion documentation, the correct v1 design is:

- add `notion create-page TITLE`
- require `--parent-page-id` for child-page creation
- keep workspace, database, and data-source creation out of scope
- accept Markdown via `--content`, `--content-file`, or `stdin`
- omit `markdown` entirely when creating a blank page
- print a concise success summary instead of the created body

This keeps the feature aligned with the real API, preserves the repo's lightweight architecture, and adds page creation in a way that is practical for both shell users and LLM agents.
