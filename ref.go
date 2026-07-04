package main

import (
	"fmt"
	"strings"
)

// parseResourceRef parses a resource reference of the form "[type:]id".
//
// A bare id (no colon) defaults to kind "page" — the most common resource a
// user addresses. Recognized type prefixes:
//
//	page, db/database, ds/data-source/data_source
//
// The returned id is normalized to the canonical lowercase UUID form via
// normalizeNotionID. kind is one of "page", "database", "data_source".
func parseResourceRef(raw string) (kind, id string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", cliError{"resource reference cannot be empty."}
	}
	if idx := strings.Index(raw, ":"); idx > 0 {
		prefix := strings.ToLower(strings.TrimSpace(raw[:idx]))
		rest := strings.TrimSpace(raw[idx+1:])
		switch prefix {
		case "page":
			kind = "page"
		case "db", "database":
			kind = "database"
		case "ds", "data-source", "data_source":
			kind = "data_source"
		default:
			return "", "", cliError{fmt.Sprintf("Unknown resource type prefix: %s", prefix)}
		}
		normID, err := normalizeNotionID(rest, kindLabel(kind))
		if err != nil {
			return "", "", err
		}
		return kind, normID, nil
	}
	// Bare id → default to page.
	normID, err := normalizeNotionID(raw, "page")
	if err != nil {
		return "", "", err
	}
	return "page", normID, nil
}

// kindLabel returns a human-readable label for a resource kind, suitable for
// error messages (matches the wording used by normalizeNotionID).
func kindLabel(kind string) string {
	switch kind {
	case "database":
		return "database"
	case "data_source":
		return "data source"
	default:
		return "page"
	}
}
