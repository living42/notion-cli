import argparse
import json
import sys
from pathlib import Path
import re

import httpx

NOTION_VERSION = "2025-09-03"
CONFIG_PATH = Path.home() / ".config" / "notion-cli" / "config.json"


# ---------------------------------------------------------------------------
# Config helpers
# ---------------------------------------------------------------------------

def get_selected_profile(raw_profile: str) -> str:
    profile = raw_profile.strip()
    if not profile:
        print("Profile cannot be empty.", file=sys.stderr)
        sys.exit(1)
    return profile


def load_config(*, required: bool = True) -> dict:
    if not CONFIG_PATH.exists():
        if required:
            print("No config found. Run 'notion configure' first.", file=sys.stderr)
            sys.exit(1)
        return {"profiles": {}}

    try:
        with CONFIG_PATH.open() as f:
            config = json.load(f)
    except json.JSONDecodeError:
        print(
            f"Config file is malformed JSON: {CONFIG_PATH}. Please re-run `notion configure`.",
            file=sys.stderr,
        )
        sys.exit(1)

    if not isinstance(config, dict) or not isinstance(config.get("profiles"), dict):
        print(
            f"Invalid config format in {CONFIG_PATH}. Expected top-level 'profiles' object.",
            file=sys.stderr,
        )
        sys.exit(1)

    return config


def save_config(config: dict) -> None:
    CONFIG_PATH.parent.mkdir(parents=True, exist_ok=True)
    with CONFIG_PATH.open("w") as f:
        json.dump(config, f, indent=2)
        f.write("\n")


def get_profile_secret(config: dict, profile: str) -> str:
    profile_data = config.get("profiles", {}).get(profile)
    if not isinstance(profile_data, dict):
        print(
            f"Profile '{profile}' not configured. Run: notion configure -p {profile}",
            file=sys.stderr,
        )
        sys.exit(1)

    secret = profile_data.get("notion_secret")
    if not isinstance(secret, str) or not secret.strip():
        print(
            f"Profile '{profile}' has no valid notion_secret. Re-run: notion configure -p {profile}",
            file=sys.stderr,
        )
        sys.exit(1)

    return secret.strip()


# ---------------------------------------------------------------------------
# HTTP helpers
# ---------------------------------------------------------------------------

def headers(secret: str) -> dict:
    return {
        "Authorization": f"Bearer {secret}",
        "Notion-Version": NOTION_VERSION,
        "Content-Type": "application/json",
    }


def notion_post(path: str, secret: str, body: dict) -> dict:
    url = f"https://api.notion.com{path}"
    resp = httpx.post(url, headers=headers(secret), json=body)
    if resp.status_code != 200:
        print(f"Error {resp.status_code}: {resp.text}", file=sys.stderr)
        sys.exit(1)
    return resp.json()


def notion_get(path: str, secret: str) -> dict:
    url = f"https://api.notion.com{path}"
    resp = httpx.get(url, headers=headers(secret))
    if resp.status_code != 200:
        print(f"Error {resp.status_code}: {resp.text}", file=sys.stderr)
        sys.exit(1)
    return resp.json()


# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------

def cmd_configure(args) -> None:
    profile = get_selected_profile(args.profile)
    config = {"profiles": {}}

    if CONFIG_PATH.exists():
        try:
            with CONFIG_PATH.open() as f:
                raw = json.load(f)
            if isinstance(raw, dict) and isinstance(raw.get("profiles"), dict):
                config = raw
            else:
                print("Existing config format is invalid; it will be replaced.", file=sys.stderr)
        except json.JSONDecodeError:
            print("Existing config is malformed JSON; it will be replaced.", file=sys.stderr)

    existing_secret = config.get("profiles", {}).get(profile, {}).get("notion_secret")
    if isinstance(existing_secret, str) and existing_secret.strip():
        answer = input(f"Profile '{profile}' already exists. Reconfigure? [y/N] ").strip().lower()
        if answer != "y":
            print("Aborted.")
            return

    secret = input("Enter your Notion integration secret: ").strip()
    if not secret:
        print("Secret cannot be empty.", file=sys.stderr)
        sys.exit(1)

    config.setdefault("profiles", {})[profile] = {"notion_secret": secret}
    save_config(config)
    print(f"Config saved to {CONFIG_PATH} (profile: {profile})")


# ---------------------------------------------------------------------------
# Formatters
# ---------------------------------------------------------------------------

def extract_title(page: dict) -> str:
    """Extract plain-text title from a page or database result."""
    props = page.get("properties", {})
    # Pages store title under a property of type "title"
    for prop in props.values():
        if prop.get("type") == "title":
            parts = prop.get("title", [])
            return "".join(p.get("plain_text", "") for p in parts)
    # Databases expose title at top level
    title_list = page.get("title", [])
    if title_list:
        return "".join(p.get("plain_text", "") for p in title_list)
    return "(untitled)"


def extract_icon(page: dict) -> str:
    """Return emoji icon prefix (with trailing space) or empty string."""
    icon = page.get("icon")
    if icon and icon.get("type") == "emoji":
        return icon["emoji"] + " "
    return ""


def format_parent(parent: dict) -> str:
    if not parent:
        return "unknown"
    ptype = parent.get("type", "unknown")
    if ptype == "workspace":
        return "workspace"
    if ptype == "page_id":
        return f"page `{parent['page_id']}`"
    if ptype == "database_id":
        return f"database `{parent['database_id']}`"
    return ptype


def format_search_result(page: dict) -> str:
    icon = extract_icon(page)
    title = extract_title(page)
    obj_type = page.get("object", "page")
    url = page.get("url", "")
    parent = format_parent(page.get("parent", {}))
    created = page.get("created_time", "")
    edited = page.get("last_edited_time", "")

    lines = [
        f"## {icon}{title}",
        f"- **Type:** {obj_type}",
        f"- **URL:** {url}",
        f"- **Parent:** {parent}",
    ]
    if created:
        lines.append(f"- **Created:** {created}")
    if edited:
        lines.append(f"- **Last edited:** {edited}")
    return "\n".join(lines)


def format_search_output(data: dict) -> str:
    results = data.get("results", [])
    sections = [format_search_result(r) for r in results]
    body = "\n\n".join(sections)

    # Metadata block
    meta_lines = []
    if data.get("has_more"):
        meta_lines.append("has_more: true")
    next_cursor = data.get("next_cursor")
    if next_cursor:
        meta_lines.append(f"next_cursor: {next_cursor}")
    request_id = data.get("request_id", "")
    if request_id:
        meta_lines.append(f"request_id: {request_id}")

    if meta_lines:
        meta = "<!-- metadata\n" + "\n".join(meta_lines) + "\n-->"
        return body + "\n\n---\n\n" + meta
    return body


def convert_notion_markdown(raw: str) -> str:
    """Convert Notion-flavoured XML tags in the markdown string to standard Markdown."""
    # <page url="URL">Title</page>  →  [Title](URL)
    raw = re.sub(r'<page url="([^"]+)">([^<]*)</page>', r'[\2](\1)', raw)
    # strip <empty-block/>
    raw = re.sub(r'<empty-block\s*/>', '', raw)
    # clean up excess blank lines left by stripped tags
    raw = re.sub(r'\n{3,}', '\n\n', raw)
    return raw.strip()


def format_fetch_output(data: dict, page_meta: dict, slice_range: tuple[int, int] | None = None) -> str:
    # Header: title + URL
    icon = extract_icon(page_meta)
    title = extract_title(page_meta)
    url = page_meta.get("url", "")
    header_lines = [f"# {icon}{title}"]
    if url:
        header_lines.append(f"**URL:** {url}")
    header = "\n".join(header_lines)

    raw_md = data.get("markdown", "")
    body = convert_notion_markdown(raw_md)

    # Apply line slice to body only
    if slice_range is not None:
        start, end = slice_range
        body_lines = body.splitlines()
        body = "\n".join(body_lines[start:end])

    unknown = data.get("unknown_block_ids", [])
    meta_lines = [
        f"page_id: {data.get('id', '')}",
        f"truncated: {str(data.get('truncated', False)).lower()}",
        f"unknown_block_ids: {', '.join(unknown) if unknown else '[]'}",
        f"request_id: {data.get('request_id', '')}",
    ]
    if slice_range is not None:
        meta_lines.append(f"slice: {slice_range[0]}-{slice_range[1]}")
    meta = "<!-- metadata\n" + "\n".join(meta_lines) + "\n-->"
    return header + "\n\n" + body + "\n\n---\n\n" + meta


# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------

def cmd_search(args) -> None:
    profile = get_selected_profile(args.profile)
    config = load_config()
    secret = get_profile_secret(config, profile)

    body: dict = {}

    if args.query:
        body["query"] = args.query

    if args.sort_timestamp or args.sort_direction:
        body["sort"] = {
            "timestamp": args.sort_timestamp or "last_edited_time",
            "direction": args.sort_direction or "descending",
        }

    if args.start_cursor:
        body["start_cursor"] = args.start_cursor

    if args.page_size is not None:
        body["page_size"] = args.page_size

    result = notion_post("/v1/search", secret, body)
    print(format_search_output(result))


def parse_slice(value: str) -> tuple[int, int]:
    """Parse 'n-m' into (n, m). Raises argparse-friendly ValueError on bad input."""
    parts = value.split("-")
    if len(parts) != 2:
        raise argparse.ArgumentTypeError(f"--slice must be in the form N-M, got: {value!r}")
    try:
        start, end = int(parts[0]), int(parts[1])
    except ValueError:
        raise argparse.ArgumentTypeError(f"--slice values must be integers, got: {value!r}")
    if start < 0 or end < start:
        raise argparse.ArgumentTypeError(f"--slice requires 0 <= N <= M, got: {value!r}")
    return (start, end)


def cmd_fetch_page(args) -> None:
    profile = get_selected_profile(args.profile)
    config = load_config()
    secret = get_profile_secret(config, profile)

    # Fetch page metadata and markdown content
    page_meta = notion_get(f"/v1/pages/{args.page_id}", secret)
    data = notion_get(f"/v1/pages/{args.page_id}/markdown", secret)

    slice_range = parse_slice(args.slice) if args.slice else None
    print(format_fetch_output(data, page_meta, slice_range))


# ---------------------------------------------------------------------------
# Argument parser
# ---------------------------------------------------------------------------

def add_profile_argument(parser: argparse.ArgumentParser) -> None:
    parser.add_argument(
        "-p",
        "--profile",
        default="default",
        metavar="PROFILE",
        help="Config profile to use (default: default).",
    )


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="notion",
        description="Lightweight Notion CLI for searching and reading pages.",
    )
    add_profile_argument(parser)
    sub = parser.add_subparsers(dest="command", metavar="<command>")

    # configure
    sub.add_parser(
        "configure",
        help="Set up or update your Notion integration secret.",
    )

    # search
    search_p = sub.add_parser(
        "search",
        help="Search Notion pages and databases.",
    )
    search_p.add_argument(
        "query",
        nargs="?",
        default=None,
        help="Search query string.",
    )
    search_p.add_argument(
        "--sort-timestamp",
        default="last_edited_time",
        metavar="FIELD",
        help="Timestamp field to sort by (default: last_edited_time).",
    )
    search_p.add_argument(
        "--sort-direction",
        default="descending",
        choices=["ascending", "descending"],
        help="Sort direction (default: descending).",
    )
    search_p.add_argument(
        "--start-cursor",
        default=None,
        metavar="UUID",
        help="Pagination cursor (UUID) from a previous response.",
    )
    search_p.add_argument(
        "--page-size",
        type=int,
        default=10,
        metavar="N",
        help="Number of results to return, max 100 (default: 10).",
    )

    # fetch-page
    fetch_p = sub.add_parser(
        "fetch-page",
        help="Fetch a Notion page as Markdown.",
    )
    fetch_p.add_argument(
        "page_id",
        help="The UUID of the Notion page to fetch.",
    )
    fetch_p.add_argument(
        "--slice",
        default=None,
        metavar="N-M",
        help="Output only lines N through M of the page body (0-indexed, e.g. --slice 0-20).",
    )

    return parser


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

def normalize_profile_args(argv: list[str]) -> list[str]:
    """Allow -p/--profile before or after subcommand by normalizing argv."""
    normalized: list[str] = []
    profile_value: str | None = None

    i = 0
    while i < len(argv):
        token = argv[i]
        if token == "-p" or token == "--profile":
            if i + 1 >= len(argv):
                normalized.append(token)
                i += 1
                continue
            profile_value = argv[i + 1]
            i += 2
            continue
        if token.startswith("--profile="):
            profile_value = token.split("=", 1)[1]
            i += 1
            continue

        normalized.append(token)
        i += 1

    if profile_value is not None:
        return ["--profile", profile_value, *normalized]
    return normalized


def main() -> None:
    parser = build_parser()
    args = parser.parse_args(normalize_profile_args(sys.argv[1:]))

    if args.command == "configure":
        cmd_configure(args)
    elif args.command == "search":
        cmd_search(args)
    elif args.command == "fetch-page":
        cmd_fetch_page(args)
    else:
        parser.print_help()


if __name__ == "__main__":
    main()
