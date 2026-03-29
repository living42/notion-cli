# notion-cli

A lightweight, zero-setup command-line tool to search, read, create, and update your Notion pages, plus inspect databases and data sources. It is designed to work well for both humans and automation.

```bash
# Search your Notion workspace
notion search "project roadmap"

# Read a page as Markdown
notion fetch-page 3c90c3cc-0d44-4b50-8888-8dd25736052a

# Create a child page from Markdown
notion create-page "Release Notes" --parent-page-id 3c90c3cc-0d44-4b50-8888-8dd25736052a --content-file ./release-notes.md

# Update page content
notion update-page 3c90c3cc-0d44-4b50-8888-8dd25736052a --old "Draft" --new "Published"

# Inspect a database or data source
notion fetch-database 2f0f7f20-5d8b-4a1a-bf88-8f5fa9cfaa10
notion query-data-source 1d2e3f44-aaaa-bbbb-cccc-1234567890ab
```

---

## Features

- **One-liner installation** — No `pip install`, no virtual environments. Just run it.
- **Readable Markdown output** — `search`, `fetch-page`, `create-page`, and `update-page` produce compact Markdown output.
- **Database + data source support** — Fetch databases, fetch data sources, and query data sources.
- **Pretty JSON where it helps** — `fetch-database`, `fetch-data-source`, and `query-data-source` return pretty-printed JSON.
- **Page creation** — Create child pages from inline Markdown, files, or stdin.
- **Page content updates** — Search-and-replace or fully replace page markdown.
- **Multi-profile config** — Use different Notion tokens with `-p/--profile`.
- **Pagination support** — Walk through large result sets with cursors.
- **Slice large pages** — View only the lines you need with `fetch-page --slice`.
- **Compact or hyphenated IDs** — Commands accept both 32-char and UUID-style Notion IDs.

---

## Installation

You need `uv` (the Python script runner). [Install it here](https://docs.astral.sh/uv/).

Then download and install the script with:

```bash
mkdir -p ~/.local/bin
curl -fSL -o ~/.local/bin/notion https://raw.githubusercontent.com/living42/notion-cli/refs/heads/main/notion
chmod +x ~/.local/bin/notion
```

> Make sure `~/.local/bin` is in your `PATH`.

---

## Quick Start

### 1. Configure your secret

Get your Notion integration token from [https://www.notion.so/profile/integrations](https://www.notion.so/profile/integrations).

```bash
# default profile
notion configure

# named profile
notion configure -p work
```

Use a named profile on any command:

```bash
notion -p work search "meeting notes"
notion fetch-page -p work 3c90c3cc0d444b5088888dd25736052a
```

### 2. Search your workspace

```bash
notion search "meeting notes"
notion search --page-size 5 "meeting notes"
notion search --start-cursor 2afd4d83-8b76-807e-a556-cae88e10b8a8 "meeting notes"
```

Example output:

```markdown
## 💼 Q1 Planning
- **Type:** page
- **URL:** https://www.notion.so/Q1-Planning-abc123...
- **Parent:** workspace
- **Created:** 2025-01-15T10:30:00.000Z
- **Last edited:** 2026-03-05T14:22:00.000Z

---

<!-- metadata
has_more: true
next_cursor: xyz789...
request_id: req-001...
-->
```

### 3. Read a page

```bash
notion fetch-page abc123def456abc123def456abc123de
notion fetch-page abc123def456abc123def456abc123de --slice 0-30
```

### 4. Create a page

```bash
# blank child page
notion create-page "Scratchpad" --parent-page-id abc123def456abc123def456abc123de

# from a file
notion create-page "Release Notes" --parent-page-id abc123def456abc123def456abc123de --content-file ./release-notes.md

# from stdin
cat ./release-notes.md | notion create-page "Release Notes" --parent-page-id abc123def456abc123def456abc123de
```

### 5. Update a page

```bash
# targeted replacement
notion update-page abc123def456abc123def456abc123de --old "Status: Draft" --new "Status: Published"

# replace the whole page from a file
notion update-page abc123def456abc123def456abc123de --replace --content-file ./page.md
```

### 6. Inspect databases and data sources

```bash
notion fetch-database 2f0f7f20-5d8b-4a1a-bf88-8f5fa9cfaa10
notion fetch-data-source 1d2e3f44-aaaa-bbbb-cccc-1234567890ab
notion query-data-source 1d2e3f44-aaaa-bbbb-cccc-1234567890ab --page-size 20
```

These commands print pretty JSON.

---

## Commands Reference

### `notion` or `notion --help`

Print help and list all available commands.

Global option:

- `-p, --profile PROFILE` — choose config profile (default: `default`)

### `notion configure`

Set up or reconfigure your Notion integration secret for a profile.

```bash
notion configure
notion configure -p work
```

### `notion search [OPTIONS] [QUERY]`

Search pages, databases, and data sources in your Notion workspace.

| Option | Default | Description |
|---|---|---|
| `-p, --profile PROFILE` | `default` | Config profile to use |
| `<query>` | _(none)_ | Search term |
| `--sort-timestamp FIELD` | `last_edited_time` | `created_time` or `last_edited_time` |
| `--sort-direction {ascending,descending}` | `descending` | Sort direction |
| `--start-cursor UUID` | _(none)_ | Pagination cursor |
| `--page-size N` | `10` | Number of results (max 100) |

### `notion fetch-page PAGE_ID [OPTIONS]`

Retrieve a single Notion page as Markdown.

| Option | Description |
|---|---|
| `-p, --profile PROFILE` | Config profile to use |
| `<page_id>` | Hyphenated or compact page ID |
| `--slice N-M` | Show only lines N through M |

### `notion create-page TITLE --parent-page-id UUID [OPTIONS]`

Create a new child page under an existing page.

| Option | Description |
|---|---|
| `TITLE` | Title of the new page |
| `--parent-page-id UUID` | Parent page ID |
| `--content TEXT` | Inline Markdown body |
| `--content-file PATH` | Read Markdown body from a file |

If neither `--content` nor `--content-file` is provided, content is read from `stdin` when piped; otherwise a blank page is created.

### `notion update-page PAGE_ID [OPTIONS]`

Update a page's Markdown content.

Targeted mode:

| Option | Description |
|---|---|
| `<page_id>` | Page ID |
| `--old TEXT` | Existing string to find; repeatable |
| `--new TEXT` | Replacement string; repeatable |
| `--replace-all-matches` | Replace all matches for each `--old` |
| `--allow-deleting-content` | Allow operations that delete child pages or databases |

Replace mode:

| Option | Description |
|---|---|
| `--replace` | Replace the entire page content |
| `--content TEXT` | Replacement Markdown content |
| `--content-file PATH` | Read replacement Markdown from a file |
| `--allow-deleting-content` | Allow operations that delete child pages or databases |

### `notion fetch-database DATABASE_ID`

Fetch a database object and print pretty JSON.

### `notion fetch-data-source DATA_SOURCE_ID`

Fetch a data source object and print pretty JSON.

### `notion query-data-source DATA_SOURCE_ID [OPTIONS]`

Query a data source and print pretty JSON.

| Option | Default | Description |
|---|---|---|
| `-p, --profile PROFILE` | `default` | Config profile to use |
| `--sorts JSON` | _(none)_ | Notion query `sorts` array |
| `--filter JSON` | _(none)_ | Notion query `filter` object |
| `--start-cursor UUID` | _(none)_ | Pagination cursor |
| `--page-size N` | `10` | Number of results (max 100) |
| `--in-trash` | `false` | Include trashed entries |
| `--result-type TYPE` | _(none)_ | Optional Notion `result_type` |

---

## Troubleshooting

**"No config found"**
Run `notion configure`.

**"Profile 'work' not configured"**
Run `notion configure -p work`.

**"Error 401: Unauthorized"**
Your secret is invalid or expired. Re-run `notion configure`.

**"Error 403: restricted_resource"**
Your integration may be missing the required Notion capability. Use **Insert Content** for `create-page` and **Update Content** for `update-page`.

**"Error 404: Not found"**
Check the ID and make sure your integration has access.

**"uv: command not found"**
Install `uv` from [docs.astral.sh/uv](https://docs.astral.sh/uv/).

---

## Config File

Config is stored at `~/.config/notion-cli/config.json`:

```json
{
  "profiles": {
    "default": {
      "notion_secret": "secret_xxxxxxxxxxxxxxxxxxxx"
    },
    "work": {
      "notion_secret": "secret_yyyyyyyyyyyyyyyyyyyy"
    }
  }
}
```

Legacy single-secret configs are also accepted and will continue to work.

---

## Limitations

This tool supports searching, reading, creating child pages, updating page markdown, and read-only inspection of databases and data sources. It does not:

- Create workspace-level pages
- Create pages under databases or data sources
- Manage arbitrary page properties
- Perform general block-level CRUD outside the page-markdown endpoint
- Use OAuth

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
