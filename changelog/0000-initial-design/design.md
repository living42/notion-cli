# notion-cli

A lightweight command-line tool to search and read Notion pages, designed primarily for use by LLM harnesses (AI agents such as Claude Code, OpenCode, etc.).

---

## Proposal

### Overview

`notion-cli` is a single-file Python CLI that wraps the Notion REST API, exposing two core operations — **search** and **fetch-page** — as easy-to-invoke shell commands. Because it is intended to be called by AI agents, the output is clean, machine-readable, and avoids unnecessary UI chrome.

The script uses [PEP 723 inline script metadata](https://peps.python.org/pep-0723/) so that `uv` can resolve and install dependencies automatically on first run — no `pip install`, no virtual-env setup required by the user.

---

### Design Decisions

| Decision | Rationale |
|---|---|
| Single Python script | Zero install friction; easy to drop into any environment |
| `uv run --script` shebang | Auto-resolves dependencies declared inline; no manual env setup |
| Config stored in `~/.config/notion-cli/config.json` | Follows XDG convention; writable without root |
| Markdown output for `fetch-page` | LLMs consume Markdown natively; uses Notion's `/markdown` endpoint |
| All Notion API options surfaced as flags | Lets callers (human or agent) control pagination, sorting, etc. |

---

### Configuration

Config is stored at `$HOME/.config/notion-cli/config.json`:

```json
{
  "notion_secret": "secret_xxxxxxxxxxxxxxxxxxxx"
}
```

The secret is a Notion Internal Integration Token obtained from [notion.so/my-integrations](https://www.notion.so/my-integrations).

---

### Commands

#### `notion`

Prints a help message listing all available commands and their options.

---

#### `notion configure`

Interactive setup wizard.

- If a config already exists, asks the user to confirm before overwriting.
- Prompts for the Notion integration secret (`notion_secret`).
- Writes the config to `$HOME/.config/notion-cli/config.json`, creating parent directories if needed.

---

#### `notion search [OPTIONS] <query>`

Searches Notion pages and databases via the `/v1/search` endpoint.

**Options:**

| Flag | Default | Description |
|---|---|---|
| `--sort-timestamp` | `last_edited_time` | Field to sort by |
| `--sort-direction` | `descending` | `ascending` or `descending` |
| `--start-cursor` | _(none)_ | UUID cursor for pagination |
| `--page-size` | `10` | Number of results to return (max 100) |

**Example:**

```bash
notion search --page-size 5 "project roadmap"
```

**Underlying API call:**

```bash
curl --request POST \
  --url https://api.notion.com/v1/search \
  --header 'Authorization: Bearer <token>' \
  --header 'Content-Type: application/json' \
  --header 'Notion-Version: 2025-09-03' \
  --data '{
    "query": "project roadmap",
    "sort": {
      "timestamp": "last_edited_time",
      "direction": "descending"
    },
    "page_size": 5
  }'
```

**Output:** JSON array of matching pages/databases printed to stdout.

---

#### `notion fetch-page <page_id>`

Retrieves a single Notion page in Markdown format via the `/v1/pages/{page_id}/markdown` endpoint.

**Example:**

```bash
notion fetch-page 3c90c3cc-0d44-4b50-8888-8dd25736052a
```

**Underlying API call:**

```bash
curl --request GET \
  --url https://api.notion.com/v1/pages/{page_id}/markdown \
  --header 'Authorization: Bearer <token>' \
  --header 'Notion-Version: 2025-09-03'
```

**Output:** Raw Markdown content of the page printed to stdout.

---

### File Structure

```
notion-cli/
├── README.md
└── notion          # single executable Python script
```

The script `notion` begins with:

```python
#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.11"
# dependencies = [
#   "httpx",
# ]
# ///
```

This allows it to be invoked directly (`./notion search "..."`) without any prior installation step, as long as `uv` is available.

---

### Dependencies

| Package | Purpose |
|---|---|
| [`httpx`](https://www.python-httpx.org/) | HTTP client for Notion API requests |

Standard-library modules (`argparse`, `json`, `os`, `sys`, `pathlib`) cover everything else.

---

### Usage by AI Agents

AI agents can invoke `notion-cli` as a shell tool:

```
# Discover relevant pages
notion search "sprint planning"

# Read a specific page
notion fetch-page <page_id>
```

Because both commands write clean output to stdout and errors to stderr with non-zero exit codes, they integrate naturally into any tool-calling harness.

---

### Out of Scope (v1)

- Creating or editing pages
- Managing databases
- Handling block-level operations
- OAuth / multi-user auth (single integration token only)

---

## Proposal: Human-Readable Markdown Output

### Problem

Both `search` and `fetch-page` currently dump raw JSON to stdout. This is hard to read for humans and adds noise for LLMs that must parse deeply-nested structures to extract basic information (title, URL, dates, page content).

**Current `search` output (raw JSON, abridged):**
```json
{
  "object": "list",
  "results": [
    {
      "object": "page",
      "id": "2afd4d83-8b76-8095-9ef6-ed75cd5c579c",
      "created_time": "2025-11-18T03:02:00.000Z",
      "last_edited_time": "2026-03-05T14:22:00.000Z",
      "icon": { "type": "emoji", "emoji": "💼" },
      "parent": { "type": "workspace", "workspace": true },
      "properties": {
        "title": {
          "title": [{ "plain_text": "Work" }]
        }
      },
      "url": "https://www.notion.so/Work-2afd4d83..."
      ...
    }
  ],
  "next_cursor": "2afd4d83-8b76-807e-a556-cae88e10b8a8",
  "has_more": true,
  "request_id": "3a29bcd0-..."
}
```

**Current `fetch-page` output (raw JSON):**
```json
{
  "object": "page_markdown",
  "id": "2afd4d83-8b76-8095-9ef6-ed75cd5c579c",
  "markdown": "<page url=\"https://www.notion.so/...\">2026</page>\n<page url=\"...\">2025</page>\n---\n<empty-block/>",
  "truncated": false,
  "unknown_block_ids": [],
  "request_id": "cde0c99f-..."
}
```

---

### Proposed Output Format

Both commands will output clean Markdown. Metadata (pagination state, IDs, flags) is appended at the bottom after a `---` separator, so the main content is easy to read while all machine-useful fields are still accessible.

---

#### `notion search` — Proposed Output

Each result is rendered as a Markdown section. The metadata block at the end captures pagination state.


```markdown
## 💼 Work
- **Type:** page
- **URL:** https://www.notion.so/Work-2afd4d838b7680959ef6ed75cd5c579c
- **Parent:** workspace
- **Created:** 2025-11-18T03:02:00.000Z
- **Last edited:** 2026-03-05T14:22:00.000Z

## 💼 2026
- **Type:** page
- **URL:** https://www.notion.so/2026-2ddd4d838b7680ffb5c9cf7a7f39cb91
- **Parent:** https://www.notion.so/Work-2afd4d838b7680959ef6ed75cd5c579c %% using url that is
- **Created:** 2026-01-03T15:53:00.000Z
- **Last edited:** 2026-03-05T10:36:00.000Z

---

<!-- metadata
has_more: true
next_cursor: 2afd4d83-8b76-807e-a556-cae88e10b8a8
request_id: 3a29bcd0-3fde-41eb-851a-c6c2d8314787
-->
```

**Extraction rules:**
| Field | Source in API response |
|---|---|
| Heading | `icon.emoji` (if present) + `properties.title[].plain_text` joined |
| Type | `object` field (`page` or `database`) |
| URL | `url` |
| Parent | `parent.type`; if `page_id`, append the UUID |
| Created / Last edited | `created_time` / `last_edited_time` |
| Metadata block | `has_more`, `next_cursor` (omitted when `null`), `request_id` |

---

#### `notion fetch-page` — Proposed Output

The `markdown` string from the API response is rendered directly as the page body. Inline Notion-flavoured XML tags (`<page url="...">Title</page>`) are converted to standard Markdown links. The metadata block appended at the end captures page-level flags.

**Conversion rule for inline tags:**
`<page url="URL">Title</page>` → `[Title](URL)`

```markdown
[2026](https://www.notion.so/2ddd4d838b7680ffb5c9cf7a7f39cb91)
[2025](https://www.notion.so/2afd4d838b76807ea556cae88e10b8a8)

---
[Work(deprecated)](https://www.notion.so/0a5dac86e6034f769d2485a1fd71fe0b)

---

<!-- metadata
page_id: 2afd4d83-8b76-8095-9ef6-ed75cd5c579c
truncated: false
unknown_block_ids: []
request_id: cde0c99f-768f-4a4d-8072-4edb25171034
-->
```

**Extraction rules:**
| Field | Source in API response |
|---|---|
| Page body | `markdown` string, with `<page ...>` tags converted to `[title](url)` and `<empty-block/>` tags stripped |
| `page_id` | `id` |
| `truncated` | `truncated` |
| `unknown_block_ids` | `unknown_block_ids` (comma-separated, or `[]` if empty) |
| `request_id` | `request_id` |

---

### Design Notes

- **Metadata in HTML comments** — `<!-- metadata ... -->` is invisible when rendered in a Markdown viewer but still parseable as plain text; it does not interrupt the readable content.
- **No flags needed** — The formatted output is always the default. Raw JSON is not exposed as a flag; the tool is opinionated toward readability.
- **Empty-block cleanup** — `<empty-block/>` tags in the Notion markdown output carry no content and are stripped entirely.
- **Graceful icon fallback** — If a page has no `icon` or the icon type is not `emoji` (e.g. it is a file/external image), the heading is rendered without a prefix icon.
