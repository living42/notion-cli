package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

func extractTitle(page map[string]any) string {
	props := asMap(page["properties"])
	for _, propAny := range props {
		prop := asMap(propAny)
		if asString(prop["type"]) == "title" {
			parts := asSlice(prop["title"])
			var b strings.Builder
			for _, p := range parts {
				part := asMap(p)
				b.WriteString(asString(part["plain_text"]))
			}
			title := strings.TrimSpace(b.String())
			if title != "" {
				return title
			}
		}
	}

	titleList := asSlice(page["title"])
	if len(titleList) > 0 {
		var b strings.Builder
		for _, p := range titleList {
			part := asMap(p)
			b.WriteString(asString(part["plain_text"]))
		}
		title := strings.TrimSpace(b.String())
		if title != "" {
			return title
		}
	}

	name := strings.TrimSpace(asString(page["name"]))
	if name != "" {
		return name
	}
	return "(untitled)"
}

func extractIcon(page map[string]any) string {
	icon := asMap(page["icon"])
	if asString(icon["type"]) == "emoji" {
		emoji := asString(icon["emoji"])
		if emoji != "" {
			return emoji + " "
		}
	}
	return ""
}

func formatParent(parent map[string]any) string {
	if len(parent) == 0 {
		return "unknown"
	}
	ptype := asString(parent["type"])
	if ptype == "" {
		ptype = "unknown"
	}
	switch ptype {
	case "workspace":
		return "workspace"
	case "page_id":
		return fmt.Sprintf("page `%s`", asString(parent["page_id"]))
	case "database_id":
		return fmt.Sprintf("database `%s`", asString(parent["database_id"]))
	case "data_source_id":
		return fmt.Sprintf("data_source `%s`", asString(parent["data_source_id"]))
	default:
		return ptype
	}
}

func renderMetadataBlock(lines []string) string {
	return "<!-- metadata\n" + strings.Join(lines, "\n") + "\n-->"
}

func printPrettyJSON(data map[string]any) {
	b, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(b))
}

func formatSearchResult(page map[string]any) string {
	icon := extractIcon(page)
	title := extractTitle(page)
	objType := asString(page["object"])
	if objType == "" {
		objType = "page"
	}
	url := asString(page["url"])
	parent := formatParent(asMap(page["parent"]))
	created := asString(page["created_time"])
	edited := asString(page["last_edited_time"])

	lines := []string{fmt.Sprintf("## %s%s", icon, title), fmt.Sprintf("- **Type:** %s", objType)}
	if url != "" {
		lines = append(lines, "- **URL:** "+url)
	}
	lines = append(lines, "- **Parent:** "+parent)
	if created != "" {
		lines = append(lines, "- **Created:** "+created)
	}
	if edited != "" {
		lines = append(lines, "- **Last edited:** "+edited)
	}
	return strings.Join(lines, "\n")
}

func formatSearchOutput(data map[string]any) string {
	results := asSlice(data["results"])
	sections := make([]string, 0, len(results))
	for _, r := range results {
		sections = append(sections, formatSearchResult(asMap(r)))
	}
	body := "_No results._"
	if len(sections) > 0 {
		body = strings.Join(sections, "\n\n")
	}
	meta := []string{}
	if hasMore, ok := data["has_more"].(bool); ok && hasMore {
		meta = append(meta, "has_more: true")
	}
	if next := asString(data["next_cursor"]); next != "" {
		meta = append(meta, "next_cursor: "+next)
	}
	if reqID := asString(data["request_id"]); reqID != "" {
		meta = append(meta, "request_id: "+reqID)
	}
	if len(meta) > 0 {
		return body + "\n\n---\n\n" + renderMetadataBlock(meta)
	}
	return body
}

func convertNotionMarkdown(raw string) string {
	rePage := regexp.MustCompile(`<page url="([^"]+)">([^<]*)</page>`)
	reEmpty := regexp.MustCompile(`<empty-block\s*/>`)
	reNewlines := regexp.MustCompile(`\n{3,}`)
	raw = rePage.ReplaceAllString(raw, `[$2]($1)`)
	raw = reEmpty.ReplaceAllString(raw, "")
	raw = reNewlines.ReplaceAllString(raw, "\n\n")
	return strings.TrimSpace(raw)
}

func formatFetchOutput(data, pageMeta map[string]any, sliceRange *[2]int) string {
	icon := extractIcon(pageMeta)
	title := extractTitle(pageMeta)
	url := asString(pageMeta["url"])
	headerLines := []string{fmt.Sprintf("# %s%s", icon, title)}
	if url != "" {
		headerLines = append(headerLines, "**URL:** "+url)
	}
	header := strings.Join(headerLines, "\n")

	body := convertNotionMarkdown(asString(data["markdown"]))
	if sliceRange != nil {
		lines := strings.Split(body, "\n")
		start, end := sliceRange[0], sliceRange[1]
		if start > len(lines) {
			start = len(lines)
		}
		if end > len(lines) {
			end = len(lines)
		}
		body = strings.Join(lines[start:end], "\n")
	}

	unknown := asSlice(data["unknown_block_ids"])
	unknownStr := "[]"
	if len(unknown) > 0 {
		parts := make([]string, 0, len(unknown))
		for _, u := range unknown {
			parts = append(parts, asString(u))
		}
		unknownStr = strings.Join(parts, ", ")
	}
	truncated := false
	if t, ok := data["truncated"].(bool); ok {
		truncated = t
	}
	meta := []string{
		"page_id: " + asString(data["id"]),
		fmt.Sprintf("truncated: %t", truncated),
		"unknown_block_ids: " + unknownStr,
		"request_id: " + asString(data["request_id"]),
	}
	if sliceRange != nil {
		meta = append(meta, fmt.Sprintf("slice: %d-%d", sliceRange[0], sliceRange[1]))
	}
	return header + "\n\n" + body + "\n\n---\n\n" + renderMetadataBlock(meta)
}

func formatUpdatePageOutput(updateData, pageMeta map[string]any, mode string) string {
	title := extractTitle(pageMeta)
	url := asString(pageMeta["url"])
	pageID := asString(updateData["id"])
	if pageID == "" {
		pageID = asString(pageMeta["id"])
	}
	unknown := asSlice(updateData["unknown_block_ids"])
	unknownStr := "[]"
	if len(unknown) > 0 {
		parts := make([]string, 0, len(unknown))
		for _, u := range unknown {
			parts = append(parts, asString(u))
		}
		unknownStr = strings.Join(parts, ", ")
	}
	truncated := false
	if t, ok := updateData["truncated"].(bool); ok {
		truncated = t
	}
	lines := []string{
		"# ✅ Updated Page",
		"- **Title:** " + title,
		"- **URL:** " + url,
		"- **Page ID:** " + pageID,
		"- **Mode:** " + mode,
		fmt.Sprintf("- **Truncated:** %t", truncated),
		"- **Unknown block IDs:** " + unknownStr,
	}
	meta := []string{
		"page_id: " + pageID,
		"mode: " + mode,
		fmt.Sprintf("truncated: %t", truncated),
		"unknown_block_ids: " + unknownStr,
	}
	if reqID := asString(updateData["request_id"]); reqID != "" {
		meta = append(meta, "request_id: "+reqID)
	}
	return strings.Join(lines, "\n") + "\n\n---\n\n" + renderMetadataBlock(meta)
}

func formatCreatePageOutput(pageData map[string]any) string {
	title := extractTitle(pageData)
	url := asString(pageData["url"])
	pageID := asString(pageData["id"])
	parent := asMap(pageData["parent"])
	created := asString(pageData["created_time"])
	edited := asString(pageData["last_edited_time"])

	lines := []string{
		"# ✅ Created Page",
		"- **Title:** " + title,
		"- **URL:** " + url,
		"- **Page ID:** " + pageID,
		"- **Parent:** " + formatParent(parent),
	}
	if created != "" {
		lines = append(lines, "- **Created:** "+created)
	}
	if edited != "" {
		lines = append(lines, "- **Last edited:** "+edited)
	}

	parentMeta := "parent: " + asString(parent["type"])
	if asString(parent["type"]) == "page_id" {
		parentMeta = "parent: page_id:" + asString(parent["page_id"])
	}
	meta := []string{"page_id: " + pageID, parentMeta}
	if reqID := asString(pageData["request_id"]); reqID != "" {
		meta = append(meta, "request_id: "+reqID)
	}
	return strings.Join(lines, "\n") + "\n\n---\n\n" + renderMetadataBlock(meta)
}

func formatMovePageOutput(pageData, parent map[string]any) string {
	title := extractTitle(pageData)
	url := asString(pageData["url"])
	pageID := asString(pageData["id"])
	edited := asString(pageData["last_edited_time"])

	lines := []string{
		"# ✅ Moved Page",
		"- **Title:** " + title,
		"- **URL:** " + url,
		"- **Page ID:** " + pageID,
		"- **Parent:** " + formatParent(parent),
	}
	if edited != "" {
		lines = append(lines, "- **Last edited:** "+edited)
	}

	parentType := asString(parent["type"])
	parentMeta := "parent: " + parentType
	if parentType == "page_id" {
		parentMeta = "parent: page_id:" + asString(parent["page_id"])
	} else if parentType == "data_source_id" {
		parentMeta = "parent: data_source_id:" + asString(parent["data_source_id"])
	}
	meta := []string{"page_id: " + pageID, parentMeta}
	if reqID := asString(pageData["request_id"]); reqID != "" {
		meta = append(meta, "request_id: "+reqID)
	}
	return strings.Join(lines, "\n") + "\n\n---\n\n" + renderMetadataBlock(meta)
}
