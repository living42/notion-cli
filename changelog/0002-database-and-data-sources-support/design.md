# Feature Design: Database + Data Source Support

Date: 2026-03-11
Status: Implemented

---

## Summary

Added three new read-only commands to `notion-cli`:

1. `notion fetch-database <database_id>`
2. `notion fetch-data-source <data_source_id>`
3. `notion query-data-source <data_source_id> [OPTIONS]`

All three commands now return **formatted JSON** (pretty-printed), not Markdown.

---

## Goals

- Fetch one database object.
- Fetch one data source object.
- Query data source entries with pagination and optional filters/sorts.
- Reuse existing profile/auth flow (`-p/--profile`).

## Non-goals

- No mutation APIs.
- No schema editing helpers.

---

## CLI UX

### 1) `notion fetch-database DATABASE_ID`

Fetch one database object and print formatted JSON.

```bash
notion fetch-database 2f0f7f20-5d8b-4a1a-bf88-8f5fa9cfaa10
notion -p work fetch-database 2f0f7f20-5d8b-4a1a-bf88-8f5fa9cfaa10
```

API:
- `GET /v1/databases/{database_id}`

---

### 2) `notion fetch-data-source DATA_SOURCE_ID`

Fetch one data source object and print formatted JSON.

```bash
notion fetch-data-source 1d2e3f44-aaaa-bbbb-cccc-1234567890ab
notion -p work fetch-data-source 1d2e3f44-aaaa-bbbb-cccc-1234567890ab
```

API:
- `GET /v1/data_sources/{data_source_id}`

---

### 3) `notion query-data-source DATA_SOURCE_ID [OPTIONS]`

Query a data source and print formatted JSON.

```bash
# basic query
notion query-data-source 1d2e3f44-aaaa-bbbb-cccc-1234567890ab

# pagination
notion query-data-source 1d2e3f44-aaaa-bbbb-cccc-1234567890ab --page-size 20 --start-cursor <cursor>

# sort + filter
notion query-data-source 1d2e3f44-aaaa-bbbb-cccc-1234567890ab \
  --sorts '[{"property":"Due","direction":"ascending"}]' \
  --filter '{"property":"Done","checkbox":{"equals":false}}'
```

Options:

| Option | Default | Description |
|---|---|---|
| `--sorts JSON` | _(none)_ | Raw Notion `sorts` array JSON |
| `--filter JSON` | _(none)_ | Raw Notion `filter` object JSON |
| `--start-cursor UUID` | _(none)_ | Cursor for pagination |
| `--page-size N` | `10` | Number of results (max 100) |
| `--in-trash` | `false` | Include trashed entries |
| `--result-type TYPE` | _(none)_ | Optional Notion `result_type` |

API:
- `POST /v1/data_sources/{data_source_id}/query`

---

## Shared behavior

- Uses global `-p/--profile`.
- Accepts hyphenated or compact Notion IDs (normalizes before API call).
- Returns non-zero on API/auth/config errors.
- `--sorts` / `--filter` must be valid JSON (`array` and `object` respectively).

---

## Output design

For all three commands, output is:

```json
{
  "...": "raw Notion response, pretty-printed"
}
```

Implementation detail:
- Uses `json.dumps(data, indent=2, ensure_ascii=False)`.

---

## Error handling

1. No config / profile missing -> existing profile-aware messages.
2. Invalid IDs -> `Invalid <object> ID: ...`.
3. Invalid JSON for `--sorts` / `--filter` -> explicit parse/type error.
4. HTTP errors -> existing `Error <status>: <response_text>` behavior.
5. `--page-size` outside `1..100` -> validation error.

---

## Internal changes implemented

### Parser
Added subcommands:
- `fetch-database`
- `fetch-data-source`
- `query-data-source`

Added query options:
- `--sorts`
- `--filter`
- `--start-cursor`
- `--page-size`
- `--in-trash`
- `--result-type`

### Command handlers
Added:
- `cmd_fetch_database(args)`
- `cmd_fetch_data_source(args)`
- `cmd_query_data_source(args)`

### Helpers
Added:
- ID normalization for UUID/compact IDs
- JSON option parser for `--sorts` / `--filter`
- Pretty JSON printer helper

---

## Backward compatibility

- Existing commands unchanged:
  - `configure`
  - `search`
  - `fetch-page`
- Config schema unchanged.
- New commands are additive.

---

## Test checklist

1. `notion --help` lists all three commands.
2. `fetch-database` prints pretty JSON.
3. `fetch-data-source` prints pretty JSON.
4. `query-data-source` prints pretty JSON.
5. `--sorts` / `--filter` pass through correctly.
6. `--start-cursor` / `--page-size` work.
7. compact/hyphenated IDs both work.
8. invalid JSON and invalid IDs fail with actionable errors.
