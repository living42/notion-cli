# notion-cli

A lightweight, zero-setup command-line tool to search and read your Notion workspace. Ideal for scripts, LLM workflows, and automation.

```bash
# Search your Notion workspace (pretty-printed JSON)
notion search "project roadmap"

# Read a page as Markdown
notion fetch-page 3c90c3cc-0d44-4b50-8888-8dd25736052a
```

---

## Features

- **One-liner installation** — No `pip install`, no virtual environments. Just run it.
- **Pretty-printed JSON output** — `search`, `fetch-database`, `fetch-data-source`, and `query-data-source` return pretty-printed JSON.
- **Markdown page output** — `fetch-page` converts Notion XML tags to readable Markdown links.
- **Pagination support** — Walk through large result sets with cursors.
- **Slice large pages** — View only the lines you need with `fetch-page --slice`.
- **Multi-profile config** — Use different Notion tokens with `-p/--profile`.

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

Get your Notion integration token from [https://www.notion.so/profile/integrations/internal](https://www.notion.so/profile/integrations/internal).

```bash
notion configure
```

You'll be prompted to paste your secret. It's saved to `~/.config/notion-cli/config.json` and never uploaded.

Use `-p/--profile` to configure additional profiles:

```bash
notion configure -p work
notion -p work search "meeting notes"
```

### 2. Search Your Workspace

```bash
# Basic search
notion search "meeting notes"

# Control results
notion search --page-size 5 "meeting notes"

# Pagination: use next_cursor from the JSON response
notion search --start-cursor 2afd4d83-8b76-807e-a556-cae88e10b8a8 "meeting notes"
```

**Output** — Pretty-printed JSON from Notion's search endpoint.

```json
{
  "object": "list",
  "results": [
    {
      "object": "page",
      "id": "2afd4d83-8b76-8095-9ef6-ed75cd5c579c",
      "url": "https://www.notion.so/..."
    }
  ],
  "has_more": true,
  "next_cursor": "xyz789...",
  "request_id": "req-001..."
}
```

### 3. Read a Page

```bash
# Fetch a full page
notion fetch-page <page_id>

# View only the first 30 lines
notion fetch-page <page_id> --slice 0-30
```


**Output** — The page title and URL appear at the top, followed by the Markdown body and a metadata block.

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

---

## Commands Reference

### `notion` or `notion --help`

Print help and list all available commands.

Global option:

- `-p, --profile PROFILE` — choose config profile (default: `default`)

### `notion configure`

Set up or reconfigure your Notion integration secret for a profile.

- Default profile: `notion configure`
- Named profile: `notion configure -p work`

```bash
notion configure
notion configure -p work
```

### `notion search [OPTIONS] [QUERY]`

Search pages and databases in your Notion workspace. Output is pretty-printed JSON.

| Option | Default | Description |
|---|---|---|
| `-p, --profile PROFILE` | `default` | Config profile to use |
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
| `-p, --profile PROFILE` | Config profile to use (default: `default`) |
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

### `notion fetch-database DATABASE_ID`

Fetch a database object and print pretty-printed JSON.

```bash
notion fetch-database 2f0f7f20-5d8b-4a1a-bf88-8f5fa9cfaa10
```

### `notion fetch-data-source DATA_SOURCE_ID`

Fetch a data source object and print pretty-printed JSON.

```bash
notion fetch-data-source 1d2e3f44-aaaa-bbbb-cccc-1234567890ab
```

### `notion query-data-source DATA_SOURCE_ID [OPTIONS]`

Query a data source and print pretty-printed JSON.

| Option | Default | Description |
|---|---|---|
| `-p, --profile PROFILE` | `default` | Config profile to use |
| `--sorts JSON` | _(none)_ | JSON array for Notion query `sorts` |
| `--filter JSON` | _(none)_ | JSON object for Notion query `filter` |
| `--start-cursor UUID` | _(none)_ | Pagination cursor from previous response |
| `--page-size N` | `10` | Number of results (max 100) |
| `--in-trash` | `false` | Include trashed entries |
| `--result-type TYPE` | _(none)_ | Optional Notion `result_type` |

```bash
# Basic query
notion query-data-source 1d2e3f44-aaaa-bbbb-cccc-1234567890ab

# With filters and sorting
notion query-data-source 1d2e3f44-aaaa-bbbb-cccc-1234567890ab \
  --sorts '[{"property":"Due","direction":"ascending"}]' \
  --filter '{"property":"Done","checkbox":{"equals":false}}'
```

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
