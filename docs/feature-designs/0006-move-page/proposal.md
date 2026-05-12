# Move Page Support

## Proposal

Add a new command:

```bash
notion move-page PAGE_ID --parent-page-id UUID
notion move-page PAGE_ID --parent-data-source-id UUID
```

This command moves an existing Notion page under a new parent page by calling:

```http
PATCH /v1/pages/{page_id}
```

with:

```json
{
  "parent": {
    "type": "page_id",
    "page_id": "..."
  }
}
```

or:

```json
{
  "parent": {
    "type": "data_source_id",
    "data_source_id": "..."
  }
}
```

## Goals

- Keep implementation lightweight in the existing single-file CLI
- Support moving pages under another page or a data source
- Provide concise, markdown-friendly output like other write commands

## Non-Goals

- Moving pages to workspace root
- Moving pages under databases
- Property edits combined with move operations

## CLI Design

### Command

```bash
notion move-page PAGE_ID (--parent-page-id UUID | --parent-data-source-id UUID)
```

### Validation

- `PAGE_ID` must be a valid page ID
- exactly one of `--parent-page-id` or `--parent-data-source-id` must be provided
- provided destination parent ID must be valid
- when moving under a page, source page and destination parent must be different

### Output

Print a compact success summary:

```markdown
# ✅ Moved Page
- **Title:** ...
- **URL:** ...
- **Page ID:** ...
- **Parent:** page `...`
- **Last edited:** ...
```

plus metadata block including `page_id`, `parent`, and `request_id` (when present).

## Implementation Plan

1. Add `move-page` subcommand in argparse.
2. Add a request builder for parent payload and validation.
3. Add `cmd_move_page` handler that calls `PATCH /v1/pages/{page_id}`.
4. Add formatter for compact move result output.
5. Update README command list and examples.
