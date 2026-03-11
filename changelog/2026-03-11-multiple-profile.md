# Feature Design: Multiple Config Profiles (`-p/--profile`)

Date: 2026-03-11
Status: Plan

---

## Summary

Add profile support so users can keep multiple Notion integration secrets and choose one at runtime with a global `-p/--profile` flag.

- `default` profile is used when no profile is specified.
- `notion configure` configures the `default` profile.
- `notion configure -p work` configures an additional profile (`work`).
- `notion search` / `notion fetch-page` read secret from the selected profile.

---

## Goals

1. Support multiple named profiles, each with its own `notion_secret`.
2. Keep UX simple: default profile works exactly as today.
3. Make profile selection consistent across all commands.

## Non-goals

- No profile deletion/list/rename command in this iteration.
- No per-profile options beyond `notion_secret`.
- No environment-variable override design in this change.

---

## CLI UX

### Global option

Add to top-level parser (applies to all subcommands):

- `-p PROFILE`, `--profile PROFILE`
- default: `default`

Examples:

```bash
# configure default profile
notion configure

# configure a named profile
notion configure -p work

# use named profile for API calls
notion -p work search "roadmap"
notion --profile personal fetch-page <page_id>
```

Notes:
- Global args should be parsed before subcommand execution.
- Recommended usage remains `notion -p work <command> ...` for clarity.

---

## Config file design

Path remains unchanged:

`~/.config/notion-cli/config.json`

Canonical schema:

```json
{
  "profiles": {
    "default": {
      "notion_secret": "secret_xxx"
    },
    "work": {
      "notion_secret": "secret_yyy"
    }
  }
}
```

---

## Command behavior

### `configure`

Behavior with profile awareness:

1. Determine selected profile from `args.profile` (default `default`).
2. Load config if present.
3. If selected profile already has a secret, prompt:
   - `Profile '<name>' already exists. Reconfigure? [y/N]`
4. Prompt for secret and validate non-empty.
5. Save updated config.
6. Print confirmation including profile name.

Example output:

- `Config saved to ~/.config/notion-cli/config.json (profile: work)`

### `search` / `fetch-page`

1. Resolve selected profile.
2. Load config.
3. Fetch `notion_secret` from `profiles[profile]`.
4. If missing, fail with actionable message:
   - `Profile 'work' not configured. Run: notion configure -p work`
5. Continue API flow unchanged.

---

## Validation and errors

Profile name rules (minimal for v1):
- non-empty after trim
- case-sensitive
- no extra restrictions initially

Error cases:

1. Config file missing:
   - `No config found. Run 'notion configure' first.`

2. Profile missing:
   - `Profile '<name>' not configured. Run: notion configure -p <name>`

3. Profile exists but empty/invalid secret:
   - `Profile '<name>' has no valid notion_secret. Re-run: notion configure -p <name>`

4. Malformed config JSON:
   - fail with clear error to re-run configure

---

## Internal design changes (planned)

### Parser

- Add global `-p/--profile` argument on root parser before subparsers.

### Config helpers

Introduce helper responsibilities:

- `load_config() -> dict`
  - Read canonical config.

- `get_profile_secret(config, profile) -> str`
  - Resolve and validate selected profile secret.
  - Produce profile-aware errors.

- `save_config(config)`
  - Save canonical format.

### Command handlers

- `cmd_configure(args)` should read/write selected profile.
- `cmd_search(args)` and `cmd_fetch_page(args)` should call `get_profile_secret(..., args.profile)`.

---

## Test plan

1. **Configure default profile**
   - `notion configure` writes config with `profiles.default`.

2. **Configure additional profile**
   - `notion configure -p work` adds/updates `profiles.work` without removing `default`.

3. **Runtime profile selection**
   - `notion -p work search ...` uses `work` secret.

4. **Missing profile error**
   - `notion -p unknown search ...` fails with actionable message.

5. **Reconfigure prompt scope**
   - Prompt appears only when selected profile already has a secret.
