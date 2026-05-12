# Feature Design: Create Page Under Data Source

## Problem

`create-page` currently only supports `--parent-page-id`, but Notion also supports creating pages under a data source parent.

## Proposal

Extend `create-page` to accept exactly one of:

- `--parent-page-id UUID`
- `--parent-data-source-id UUID`

When `--parent-data-source-id` is used:

1. Fetch the data source (`GET /v1/data_sources/{id}`).
2. Detect the data source title property key (property with `type == "title"`).
3. Build `POST /v1/pages` with:
   - `parent: { "type": "data_source_id", "data_source_id": ... }`
   - `properties` keyed by the detected title property name.
4. Keep Markdown input behavior unchanged (`--content`, `--content-file`, stdin, blank).

## CLI UX

```bash
notion create-page "Release Notes" --parent-data-source-id <data_source_id>
```

## Validation Rules

- Reject when both parent flags are provided.
- Reject when neither parent flag is provided.
- Keep existing content input validation and ID normalization behavior.

## Documentation Changes

- Update README command synopsis/examples for `create-page`.

