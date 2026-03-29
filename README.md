# notion-cli

A lightweight, zero-setup command-line tool to search, read, create, and update your Notion pages. Perfect for scripting, LLM integrations, and automation.

```bash
# Search your Notion workspace
notion search "project roadmap"

# Read a page as clean Markdown
notion fetch-page 3c90c3cc-0d44-4b50-8888-8dd25736052a

# Create a child page from Markdown
notion create-page "Release Notes" --parent-page-id 3c90c3cc-0d44-4b50-8888-8dd25736052a --content-file ./release-notes.md

# Update page content
notion update-page 3c90c3cc-0d44-4b50-8888-8dd25736052a --old "Draft" --new "Published"
```

---

## Features

- **One-liner installation** — No `pip install`, no virtual environments. Just run it.
- **Clean Markdown output** — Notion XML is auto-converted to readable Markdown links
- **Human & machine-friendly** — Output is both easy to read and easy to parse
- **Page creation** — Create child pages from inline Markdown, files, or stdin
- **Page content updates** — Search-and-replace or fully replace page markdown
- **Pagination support** — Walk through large result sets with cursors
- **Slice large pages** — View only the lines you need with `--slice`
- **Metadata preserved** — API metadata in hidden HTML comments for traceability

---

## Installation

You need `uv` (the Python script runner). [Install it here](https://docs.astral.sh/uv/).

Then download and install the script with:

```bash
mkdir -p ~/.local/bin
curl -fSL -o ~/.local/bin/notion https://raw.githubusercontent.com/living42/notion-cli/refs/heads/main/notion
chmod +x ~/.local/bin/notion
```

> **Note:** Make sure `~/.local/bin` is in your `PATH`. You can add `export PATH="$HOME/.local/bin:$PATH"` to your shell profile (e.g. `~/.bashrc` or `~/.zshrc`) if it isn't already.

That's it!

---

## Quick Start

### 1. Configure Your Secret

Get your Notion integration token from [https://www.notion.so/profile/integrations](https://www.notion.so/profile/integrations).

```bash
notion configure
```

You'll be prompted to paste your secret. It's saved to `~/.config/notion-cli/config.json` and never uploaded.

### 2. Search Your Workspace

```bash
# Basic search
notion search "meeting notes"

# Control results
notion search --page-size 5 "meeting notes"

# Pagination: use next_cursor from the metadata block
notion search --start-cursor 2afd4d83-8b76-807e-a556-cae88e10b8a8 "meeting notes"
```

**Output** — Each result is a tidy Markdown section with title, type, URL, parent, and timestamps. Pagination state is in the metadata block at the bottom.

```markdown
## 💼 Q1 Planning
- **Type:** page
- **URL:** https://www.notion.so/Q1-Planning-abc123...
- **Parent:** workspace
- **Created:** 2025-01-15T10:30:00.000Z
- **Last edited:** 2026-03-05T14:22:00.000Z

## 📋 Engineering Roadmap
- **Type:** page
- **URL:** https://www.notion.so/Engineering-Roadmap-def456...
- **Parent:** page `abc123...`
- **Created:** 2026-02-01T09:00:00.000Z
- **Last edited:** 2026-03-04T16:45:00.000Z

---

<!-- metadata
has_more: true
next_cursor: xyz789...
request_id: req-001...
-->
```

### 3. Read a Page

```bash
# Fetch a full page
notion fetch-page abc123def456abc123def456abc123de

# View only the first 30 lines
notion fetch-page abc123def456abc123def456abc123de --slice 0-30
```

**Output** — The page title and URL at the top, followed by its Markdown body, with metadata at the bottom.

```markdown
# 📋 Engineering Roadmap
**URL:** https://www.notion.so/Engineering-Roadmap-def456...

## Q1 Goals
- [ ] Improve performance
- [ ] Refactor auth module
- [ ] Add telemetry

## Q2 Goals
- [ ] Mobile app preview
...

---

<!-- metadata
page_id: def456...
truncated: false
unknown_block_ids: []
request_id: req-002...
-->
```

### 4. Create a Page

```bash
# Create a blank child page
notion create-page "Scratchpad" --parent-page-id abc123def456abc123def456abc123de

# Create a page from a file
notion create-page "Release Notes" --parent-page-id abc123def456abc123def456abc123de --content-file ./release-notes.md
```

**Output** — A compact success summary with page info and creation metadata.

```markdown
# ✅ Created Page
- **Title:** Release Notes
- **URL:** https://www.notion.so/Release-Notes-abc123...
- **Page ID:** abc123...
- **Parent:** page `abc123def456abc123def456abc123de`
- **Created:** 2026-03-29T10:15:00.000Z
- **Last edited:** 2026-03-29T10:15:00.000Z

---

<!-- metadata
page_id: abc123...
parent: page_id:abc123def456abc123def456abc123de
request_id: req-003...
-->
```

### 5. Update a Page

```bash
# Replace one string with another
notion update-page abc123def456abc123def456abc123de --old "Status: Draft" --new "Status: Published"

# Replace the entire page from a file
notion update-page abc123def456abc123def456abc123de --replace --content-file ./page.md
```

**Output** — A compact success summary with page info and update metadata.

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

---

## Commands Reference

### `notion` or `notion --help`

Print help and list all available commands.

### `notion configure`

Set up or reconfigure your Notion integration secret. Prompts for confirmation if a config already exists.

```bash
notion configure
```

### `notion search [OPTIONS] [QUERY]`

Search pages and databases in your Notion workspace.

| Option | Default | Description |
|---|---|---|
| `<query>` | _(none)_ | Search term (optional; empty returns all pages) |
| `--sort-timestamp FIELD` | `last_edited_time` | Timestamp to sort by (`created_time` or `last_edited_time`) |
| `--sort-direction {ascending, descending}` | `descending` | Sort direction |
| `--start-cursor UUID` | _(none)_ | Pagination cursor from a previous response |
| `--page-size N` | `10` | Number of results (max 100) |

**Examples:**

```bash
# Simple search
notion search "Q1 planning"

# 20 results, sorted by creation time
notion search --page-size 20 --sort-timestamp created_time "Q1 planning"

# Paginate: get the next batch
notion search --start-cursor "2afd4d83-8b76-807e..." "Q1 planning"
```

### `notion fetch-page PAGE_ID [OPTIONS]`

Retrieve a single Notion page as Markdown.

| Option | Description |
|---|---|
| `<page_id>` | The UUID of the page to fetch (required) |
| `--slice N-M` | Show only lines N through M (0-indexed, e.g. `0-50`) |

**Examples:**

```bash
# Fetch a full page
notion fetch-page 3c90c3cc-0d44-4b50-8888-8dd25736052a

# Get only the first 20 lines
notion fetch-page 3c90c3cc-0d44-4b50-8888-8dd25736052a --slice 0-20

# Skip the first 50 lines, show next 30
notion fetch-page 3c90c3cc-0d44-4b50-8888-8dd25736052a --slice 50-80
```

### `notion create-page TITLE --parent-page-id UUID [OPTIONS]`

Create a new child page under an existing Notion page.

| Option | Description |
|---|---|
| `TITLE` | Title of the new page |
| `--parent-page-id UUID` | Parent page ID for the new child page |
| `--content TEXT` | Inline Markdown body |
| `--content-file PATH` | Read Markdown body from a file |

If neither `--content` nor `--content-file` is provided, content is read from `stdin` when piped; otherwise a blank page is created.

**Examples:**

```bash
# Create a blank child page
notion create-page "Scratchpad" --parent-page-id 3c90c3cc-0d44-4b50-8888-8dd25736052a

# Create a page from inline Markdown
notion create-page "Draft" --parent-page-id 3c90c3cc-0d44-4b50-8888-8dd25736052a --content "# Draft\n\nHello"

# Create a page from a file
notion create-page "Release Notes" --parent-page-id 3c90c3cc-0d44-4b50-8888-8dd25736052a --content-file ./release-notes.md

# Create a page from stdin
cat ./release-notes.md | notion create-page "Release Notes" --parent-page-id 3c90c3cc-0d44-4b50-8888-8dd25736052a
```

### `notion update-page PAGE_ID [OPTIONS]`

Update a page's Markdown content.

**Targeted update mode:**

| Option | Description |
|---|---|
| `<page_id>` | The UUID of the page to update (required) |
| `--old TEXT` | Existing string to find; repeat for multiple updates |
| `--new TEXT` | Replacement string; repeat for multiple updates |
| `--replace-all-matches` | Replace all matches for each `--old` value |
| `--allow-deleting-content` | Allow operations that delete child pages or databases |

**Replace mode:**

| Option | Description |
|---|---|
| `--replace` | Replace the entire page content |
| `--content TEXT` | Replacement Markdown content |
| `--content-file PATH` | Read replacement Markdown from a file |
| `--allow-deleting-content` | Allow operations that delete child pages or databases |

If `--replace` is used without `--content` or `--content-file`, content is read from `stdin`.

**Examples:**

```bash
# Replace one string
notion update-page 3c90c3cc-0d44-4b50-8888-8dd25736052a --old "Draft" --new "Published"

# Apply multiple targeted replacements
notion update-page 3c90c3cc-0d44-4b50-8888-8dd25736052a \
  --old "Q1" --new "Q2" \
  --old "draft" --new "final"

# Replace the entire page from inline content
notion update-page 3c90c3cc-0d44-4b50-8888-8dd25736052a --replace --content "# New Title\n\nNew body"

# Replace the entire page from a file
notion update-page 3c90c3cc-0d44-4b50-8888-8dd25736052a --replace --content-file ./page.md
```

---

## How It Works

- **No cloud upload** — Your secret stays on your machine in `~/.config/notion-cli/config.json`
- **Direct API calls** — Uses the official [Notion REST API](https://developers.notion.com/) with your integration token
- **Markdown output** — Converts Notion's internal XML markup to standard Markdown links and strips empty blocks
- **PEP 723 metadata** — Inline script header auto-resolves the `httpx` dependency via `uv`

---

## Use Cases

**LLM Agents & Automation:**
```bash
# Fetch and pipe into an LLM
notion search "bugs" | llm -m gpt-4 "summarize these Notion search results"

# Read a page and analyze it
notion fetch-page <page_id> | llm -m claude "extract todos from this page"

# Create a page from generated Markdown
notion create-page "Weekly Summary" --parent-page-id <page_id> --content-file ./summary.md

# Update a page after generating revised text
notion update-page <page_id> --replace --content-file ./rewritten-page.md
```

**Scripting & Data Export:**
```bash
# Export all pages matching a query
notion search --page-size 100 | jq '.[] | .url'

# Get the first 100 lines of a large page
notion fetch-page <page_id> --slice 0-100 > page_excerpt.md
```

**Integration with Other Tools:**
```bash
# Watch for changes via cron job
0 * * * * ~/notion search "status updates" > /tmp/updates.md
```

---

## Troubleshooting

**"No config found"**
Run `notion configure` to set up your integration secret.

**"Error 401: Unauthorized"**
Your secret is invalid or expired. Run `notion configure` again.

**"Error 403: restricted_resource"**
Your integration does not have the required Notion capability for the target operation. Use **Insert Content** for `create-page` and **Update Content** for `update-page`, then enable the needed capability in the integration settings.

**"Error 404: Not found"**
The page UUID doesn't exist or your integration doesn't have access to it. Check the ID and page permissions in Notion.

**"uv: command not found"**
Install `uv` from [docs.astral.sh/uv](https://docs.astral.sh/uv/).

---

## Advanced: Config File

Config is stored in plain JSON at `~/.config/notion-cli/config.json`:

```json
{
  "notion_secret": "secret_xxxxxxxxxxxxxxxxxxxx"
}
```

You can edit it directly if needed, but `notion configure` is safer.

---

## Limitations

This tool supports searching, reading, creating child pages, and Markdown page-content updates. It cannot:
- Create workspace-level pages
- Create pages under databases or data sources
- Manage databases or arbitrary page properties
- Handle general block-level operations (create/update/delete blocks outside the page-markdown endpoint)
- Use OAuth (requires a single Internal Integration token)

---

## License

MIT License

Copyright (c) 2025

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
