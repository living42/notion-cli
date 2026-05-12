package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const notionVersion = "2025-09-03"

var (
	uuidRe  = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	hex32Re = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
)

type cliError struct{ msg string }

func (e cliError) Error() string { return e.msg }

func failf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func configPath() string {
	h, err := os.UserHomeDir()
	if err != nil {
		failf("Unable to resolve home directory: %v", err)
	}
	return filepath.Join(h, ".config", "notion-cli", "config.json")
}

func getSelectedProfile(rawProfile string) (string, error) {
	profile := strings.TrimSpace(rawProfile)
	if profile == "" {
		return "", cliError{"Profile cannot be empty."}
	}
	return profile, nil
}

func loadConfig(required bool) (map[string]any, error) {
	path := configPath()
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if required {
				return nil, cliError{"No config found. Run `notion configure` first."}
			}
			return map[string]any{"profiles": map[string]any{}}, nil
		}
		return nil, err
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, cliError{fmt.Sprintf("Config file is malformed JSON: %s. Please re-run `notion configure`.", path)}
	}

	if cfg == nil {
		return nil, cliError{fmt.Sprintf("Invalid config format in %s. Expected a JSON object.", path)}
	}

	if profiles, ok := cfg["profiles"].(map[string]any); ok {
		_ = profiles
		return cfg, nil
	}

	if legacySecret, ok := cfg["notion_secret"].(string); ok && strings.TrimSpace(legacySecret) != "" {
		return map[string]any{
			"profiles": map[string]any{
				"default": map[string]any{"notion_secret": strings.TrimSpace(legacySecret)},
			},
		}, nil
	}

	return nil, cliError{fmt.Sprintf("Invalid config format in %s. Expected top-level 'profiles' object.", path)}
}

func saveConfig(config map[string]any) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

func getProfileSecret(config map[string]any, profile string) (string, error) {
	profiles, _ := config["profiles"].(map[string]any)
	profileDataAny, ok := profiles[profile]
	if !ok {
		return "", cliError{fmt.Sprintf("Profile '%s' not configured. Run: notion configure -p %s", profile, profile)}
	}
	profileData, ok := profileDataAny.(map[string]any)
	if !ok {
		return "", cliError{fmt.Sprintf("Profile '%s' not configured. Run: notion configure -p %s", profile, profile)}
	}
	secretAny, ok := profileData["notion_secret"]
	secret, ok2 := secretAny.(string)
	if !ok || !ok2 || strings.TrimSpace(secret) == "" {
		return "", cliError{fmt.Sprintf("Profile '%s' has no valid notion_secret. Re-run: notion configure -p %s", profile, profile)}
	}
	return strings.TrimSpace(secret), nil
}

func normalizeNotionID(rawID, objectLabel string) (string, error) {
	value := strings.TrimSpace(rawID)
	if uuidRe.MatchString(value) {
		return strings.ToLower(value), nil
	}
	if hex32Re.MatchString(value) {
		value = strings.ToLower(value)
		return fmt.Sprintf("%s-%s-%s-%s-%s", value[0:8], value[8:12], value[12:16], value[16:20], value[20:32]), nil
	}
	return "", cliError{fmt.Sprintf("Invalid %s ID: %s", objectLabel, rawID)}
}

func validatePageSize(pageSize int) (int, error) {
	if pageSize < 1 || pageSize > 100 {
		return 0, cliError{"--page-size must be between 1 and 100."}
	}
	return pageSize, nil
}

func headers(secret string) map[string]string {
	return map[string]string{
		"Authorization":  "Bearer " + secret,
		"Notion-Version": notionVersion,
		"Content-Type":   "application/json",
	}
}

func notionRequest(method, path, secret string, body map[string]any) (map[string]any, error) {
	url := "https://api.notion.com" + path
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}
	for k, v := range headers(secret) {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, cliError{fmt.Sprintf("Error %d: %s", resp.StatusCode, string(raw))}
	}

	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func notionGet(path, secret string) (map[string]any, error) {
	return notionRequest(http.MethodGet, path, secret, nil)
}
func notionPost(path, secret string, body map[string]any) (map[string]any, error) {
	return notionRequest(http.MethodPost, path, secret, body)
}
func notionPatch(path, secret string, body map[string]any) (map[string]any, error) {
	return notionRequest(http.MethodPatch, path, secret, body)
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func asSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

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

func parseSlice(value string) ([2]int, error) {
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		return [2]int{}, cliError{fmt.Sprintf("--slice must be in the form N-M, got: %q", value)}
	}
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return [2]int{}, cliError{fmt.Sprintf("--slice values must be integers, got: %q", value)}
	}
	end, err := strconv.Atoi(parts[1])
	if err != nil {
		return [2]int{}, cliError{fmt.Sprintf("--slice values must be integers, got: %q", value)}
	}
	if start < 0 || end < start {
		return [2]int{}, cliError{fmt.Sprintf("--slice requires 0 <= N <= M, got: %q", value)}
	}
	return [2]int{start, end}, nil
}

func stdinIsTTY() bool {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (st.Mode() & os.ModeCharDevice) != 0
}

func readReplaceContent(content, contentFile string) (string, error) {
	if content != "" && contentFile != "" {
		return "", cliError{"Use only one of --content or --content-file."}
	}
	if content != "" {
		return content, nil
	}
	if contentFile != "" {
		b, err := os.ReadFile(contentFile)
		if err != nil {
			return "", cliError{fmt.Sprintf("Unable to read --content-file %s: %v", contentFile, err)}
		}
		return string(b), nil
	}
	if stdinIsTTY() {
		return "", cliError{"Replace mode requires --content, --content-file, or piped stdin."}
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	if len(b) == 0 {
		return "", cliError{"Replace mode requires --content, --content-file, or piped stdin."}
	}
	return string(b), nil
}

func readCreateContent(content, contentFile string) (*string, error) {
	if content != "" && contentFile != "" {
		return nil, cliError{"Use only one of --content or --content-file."}
	}
	if content != "" {
		v := content
		return &v, nil
	}
	if contentFile != "" {
		b, err := os.ReadFile(contentFile)
		if err != nil {
			return nil, cliError{fmt.Sprintf("Unable to read --content-file %s: %v", contentFile, err)}
		}
		v := string(b)
		return &v, nil
	}
	if stdinIsTTY() {
		return nil, nil
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, nil
	}
	v := string(b)
	return &v, nil
}

func buildCreatePageParent(parentPageID, parentDataSourceID string) (map[string]any, error) {
	hasPageParent := strings.TrimSpace(parentPageID) != ""
	hasDataSourceParent := strings.TrimSpace(parentDataSourceID) != ""
	if hasPageParent == hasDataSourceParent {
		return nil, cliError{"create-page requires exactly one of --parent-page-id or --parent-data-source-id (not both, not neither)."}
	}
	if hasPageParent {
		norm, err := normalizeNotionID(parentPageID, "page")
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "page_id", "page_id": norm}, nil
	}
	norm, err := normalizeNotionID(parentDataSourceID, "data source")
	if err != nil {
		return nil, err
	}
	return map[string]any{"type": "data_source_id", "data_source_id": norm}, nil
}

func extractTitlePropertyName(dataSource map[string]any) (string, error) {
	props := asMap(dataSource["properties"])
	if len(props) == 0 {
		return "", cliError{"Selected data source has no properties."}
	}
	for name, propAny := range props {
		if asString(asMap(propAny)["type"]) == "title" {
			return name, nil
		}
	}
	return "", cliError{"Selected data source has no title property."}
}

func buildCreatePageBody(title, parentPageID, parentDataSourceID, content, contentFile, dataSourceTitleProp string) (map[string]any, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, cliError{"Title cannot be empty."}
	}
	parent, err := buildCreatePageParent(parentPageID, parentDataSourceID)
	if err != nil {
		return nil, err
	}
	titleProp := "title"
	if asString(parent["type"]) == "data_source_id" {
		if dataSourceTitleProp == "" {
			return nil, cliError{"Unable to determine title property for the selected data source."}
		}
		titleProp = dataSourceTitleProp
	}
	body := map[string]any{
		"parent": parent,
		"properties": map[string]any{
			titleProp: map[string]any{
				"title": []any{map[string]any{"text": map[string]any{"content": title}}},
			},
		},
	}
	createContent, err := readCreateContent(content, contentFile)
	if err != nil {
		return nil, err
	}
	if createContent != nil {
		body["markdown"] = *createContent
	}
	return body, nil
}

func buildMovePageBody(parentPageID, parentDataSourceID, pageID string) (map[string]any, map[string]any, error) {
	if parentPageID != "" {
		parentID, err := normalizeNotionID(parentPageID, "page")
		if err != nil {
			return nil, nil, err
		}
		if parentID == pageID {
			return nil, nil, cliError{"--parent-page-id must be different from the page being moved."}
		}
		parent := map[string]any{"type": "page_id", "page_id": parentID}
		return parent, map[string]any{"parent": parent}, nil
	}
	if parentDataSourceID != "" {
		dataSourceID, err := normalizeNotionID(parentDataSourceID, "data source")
		if err != nil {
			return nil, nil, err
		}
		parent := map[string]any{"type": "data_source_id", "data_source_id": dataSourceID}
		return parent, map[string]any{"parent": parent}, nil
	}
	return nil, nil, cliError{"move-page requires one of --parent-page-id or --parent-data-source-id."}
}

func buildUpdatePageBody(replace bool, content, contentFile string, olds, news []string, replaceAll, allowDeleting bool) (string, map[string]any, error) {
	if replace {
		if len(olds) > 0 || len(news) > 0 {
			return "", nil, cliError{"Do not mix --replace with --old/--new."}
		}
		newContent, err := readReplaceContent(content, contentFile)
		if err != nil {
			return "", nil, err
		}
		replaceContent := map[string]any{"new_str": newContent}
		if allowDeleting {
			replaceContent["allow_deleting_content"] = true
		}
		return "replace_content", map[string]any{"type": "replace_content", "replace_content": replaceContent}, nil
	}

	if content != "" || contentFile != "" {
		return "", nil, cliError{"--content and --content-file are only valid with --replace."}
	}
	if len(olds) != len(news) {
		return "", nil, cliError{"--old and --new must be provided the same number of times."}
	}
	if len(olds) == 0 {
		return "", nil, cliError{"Update mode requires at least one --old/--new pair, or use --replace."}
	}

	updates := make([]any, 0, len(olds))
	for i := range olds {
		update := map[string]any{"old_str": olds[i], "new_str": news[i]}
		if replaceAll {
			update["replace_all_matches"] = true
		}
		updates = append(updates, update)
	}
	updateContent := map[string]any{"content_updates": updates}
	if allowDeleting {
		updateContent["allow_deleting_content"] = true
	}
	return "update_content", map[string]any{"type": "update_content", "update_content": updateContent}, nil
}

func cmdConfigure(profile string) error {
	selectedProfile, err := getSelectedProfile(profile)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(false)
	if err != nil {
		return err
	}

	profiles := asMap(cfg["profiles"])
	existingSecret := strings.TrimSpace(asString(asMap(profiles[selectedProfile])["notion_secret"]))
	reader := bufio.NewReader(os.Stdin)
	if existingSecret != "" {
		fmt.Printf("Profile '%s' already exists. Reconfigure? [y/N] ", selectedProfile)
		answer, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	fmt.Print("Enter your Notion integration secret: ")
	secret, _ := reader.ReadString('\n')
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return cliError{"Secret cannot be empty."}
	}

	if cfg["profiles"] == nil {
		cfg["profiles"] = map[string]any{}
	}
	profiles = asMap(cfg["profiles"])
	profiles[selectedProfile] = map[string]any{"notion_secret": secret}
	cfg["profiles"] = profiles
	if err := saveConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("Config saved to %s (profile: %s)\n", configPath(), selectedProfile)
	return nil
}

func selectedSecret(profile string) (string, error) {
	selectedProfile, err := getSelectedProfile(profile)
	if err != nil {
		return "", err
	}
	cfg, err := loadConfig(true)
	if err != nil {
		return "", err
	}
	return getProfileSecret(cfg, selectedProfile)
}

func cmdSearch(profile, query, sortTimestamp, sortDirection, startCursor string, pageSize int) error {
	secret, err := selectedSecret(profile)
	if err != nil {
		return err
	}
	if _, err := validatePageSize(pageSize); err != nil {
		return err
	}
	body := map[string]any{}
	if query != "" {
		body["query"] = query
	}
	if sortTimestamp != "" || sortDirection != "" {
		body["sort"] = map[string]any{"timestamp": ifEmpty(sortTimestamp, "last_edited_time"), "direction": ifEmpty(sortDirection, "descending")}
	}
	if startCursor != "" {
		body["start_cursor"] = startCursor
	}
	body["page_size"] = pageSize

	result, err := notionPost("/v1/search", secret, body)
	if err != nil {
		return err
	}
	fmt.Println(formatSearchOutput(result))
	return nil
}

func ifEmpty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func cmdFetchPage(profile, pageID, sliceRaw string) error {
	secret, err := selectedSecret(profile)
	if err != nil {
		return err
	}
	normPageID, err := normalizeNotionID(pageID, "page")
	if err != nil {
		return err
	}
	pageMeta, err := notionGet("/v1/pages/"+normPageID, secret)
	if err != nil {
		return err
	}
	data, err := notionGet("/v1/pages/"+normPageID+"/markdown", secret)
	if err != nil {
		return err
	}
	var sliceRange *[2]int
	if strings.TrimSpace(sliceRaw) != "" {
		v, err := parseSlice(sliceRaw)
		if err != nil {
			return err
		}
		sliceRange = &v
	}
	fmt.Println(formatFetchOutput(data, pageMeta, sliceRange))
	return nil
}

func cmdFetchDatabase(profile, databaseID string) error {
	secret, err := selectedSecret(profile)
	if err != nil {
		return err
	}
	normID, err := normalizeNotionID(databaseID, "database")
	if err != nil {
		return err
	}
	data, err := notionGet("/v1/databases/"+normID, secret)
	if err != nil {
		return err
	}
	printPrettyJSON(data)
	return nil
}

func cmdFetchDataSource(profile, dataSourceID string) error {
	secret, err := selectedSecret(profile)
	if err != nil {
		return err
	}
	normID, err := normalizeNotionID(dataSourceID, "data source")
	if err != nil {
		return err
	}
	data, err := notionGet("/v1/data_sources/"+normID, secret)
	if err != nil {
		return err
	}
	printPrettyJSON(data)
	return nil
}

func parseJSONOption(raw string, expected string, flagName string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, cliError{fmt.Sprintf("Invalid JSON for %s: %v", flagName, err)}
	}
	switch expected {
	case "array":
		if _, ok := v.([]any); !ok {
			return nil, cliError{fmt.Sprintf("Invalid %s: expected JSON array.", flagName)}
		}
	case "object":
		if _, ok := v.(map[string]any); !ok {
			return nil, cliError{fmt.Sprintf("Invalid %s: expected JSON object.", flagName)}
		}
	}
	return v, nil
}

func cmdQueryDataSource(profile, dataSourceID, sortsRaw, filterRaw, startCursor string, pageSize int, inTrash bool, resultType string) error {
	secret, err := selectedSecret(profile)
	if err != nil {
		return err
	}
	normID, err := normalizeNotionID(dataSourceID, "data source")
	if err != nil {
		return err
	}
	if _, err := validatePageSize(pageSize); err != nil {
		return err
	}
	payload := map[string]any{}
	if sortsRaw != "" {
		sorts, err := parseJSONOption(sortsRaw, "array", "--sorts")
		if err != nil {
			return err
		}
		payload["sorts"] = sorts
	}
	if filterRaw != "" {
		filter, err := parseJSONOption(filterRaw, "object", "--filter")
		if err != nil {
			return err
		}
		payload["filter"] = filter
	}
	if startCursor != "" {
		payload["start_cursor"] = startCursor
	}
	payload["page_size"] = pageSize
	if inTrash {
		payload["in_trash"] = true
	}
	if resultType != "" {
		payload["result_type"] = resultType
	}
	resp, err := notionPost("/v1/data_sources/"+normID+"/query", secret, payload)
	if err != nil {
		return err
	}
	printPrettyJSON(resp)
	return nil
}

func cmdUpdatePage(profile, pageID string, replace bool, content, contentFile string, olds, news []string, replaceAll, allowDeleting bool) error {
	secret, err := selectedSecret(profile)
	if err != nil {
		return err
	}
	normPageID, err := normalizeNotionID(pageID, "page")
	if err != nil {
		return err
	}
	mode, body, err := buildUpdatePageBody(replace, content, contentFile, olds, news, replaceAll, allowDeleting)
	if err != nil {
		return err
	}
	updateData, err := notionPatch("/v1/pages/"+normPageID+"/markdown", secret, body)
	if err != nil {
		return err
	}
	pageMeta, err := notionGet("/v1/pages/"+normPageID, secret)
	if err != nil {
		return err
	}
	fmt.Println(formatUpdatePageOutput(updateData, pageMeta, mode))
	return nil
}

func cmdCreatePage(profile, title, parentPageID, parentDataSourceID, content, contentFile string) error {
	secret, err := selectedSecret(profile)
	if err != nil {
		return err
	}
	titleProp := ""
	if parentDataSourceID != "" {
		normDSID, err := normalizeNotionID(parentDataSourceID, "data source")
		if err != nil {
			return err
		}
		dataSource, err := notionGet("/v1/data_sources/"+normDSID, secret)
		if err != nil {
			return err
		}
		titleProp, err = extractTitlePropertyName(dataSource)
		if err != nil {
			return err
		}
	}
	body, err := buildCreatePageBody(title, parentPageID, parentDataSourceID, content, contentFile, titleProp)
	if err != nil {
		return err
	}
	pageData, err := notionPost("/v1/pages", secret, body)
	if err != nil {
		return err
	}
	fmt.Println(formatCreatePageOutput(pageData))
	return nil
}

func cmdMovePage(profile, pageID, parentPageID, parentDataSourceID string) error {
	secret, err := selectedSecret(profile)
	if err != nil {
		return err
	}
	normPageID, err := normalizeNotionID(pageID, "page")
	if err != nil {
		return err
	}
	parent, body, err := buildMovePageBody(parentPageID, parentDataSourceID, normPageID)
	if err != nil {
		return err
	}
	pageData, err := notionPatch("/v1/pages/"+normPageID, secret, body)
	if err != nil {
		return err
	}
	fmt.Println(formatMovePageOutput(pageData, parent))
	return nil
}

func normalizeProfileArgs(argv []string) []string {
	normalized := make([]string, 0, len(argv))
	profileValue := ""
	hasProfile := false
	for i := 0; i < len(argv); i++ {
		token := argv[i]
		switch {
		case token == "-p" || token == "--profile":
			if i+1 >= len(argv) {
				normalized = append(normalized, token)
				continue
			}
			profileValue = argv[i+1]
			hasProfile = true
			i++
		case strings.HasPrefix(token, "--profile="):
			profileValue = strings.SplitN(token, "=", 2)[1]
			hasProfile = true
		default:
			normalized = append(normalized, token)
		}
	}
	if hasProfile {
		return append([]string{"--profile", profileValue}, normalized...)
	}
	return normalized
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, ",") }
func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func globalUsage() {
	fmt.Println(`usage: notion [-h] [-p PROFILE] <command> ...

Lightweight Notion CLI for searching, reading, creating, moving, and updating
pages, databases, and data sources.

commands:
  configure
  search
  fetch-page
  fetch-database
  fetch-data-source
  query-data-source
  create-page
  move-page
  update-page`)
}

func run(argv []string) error {
	normalized := normalizeProfileArgs(argv)
	global := flag.NewFlagSet("notion", flag.ContinueOnError)
	global.SetOutput(io.Discard)
	profile := global.String("profile", "default", "")
	global.StringVar(profile, "p", "default", "")
	if err := global.Parse(normalized); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			globalUsage()
			return nil
		}
		return err
	}
	args := global.Args()
	if len(args) == 0 {
		globalUsage()
		return nil
	}
	cmd := args[0]

	switch cmd {
	case "configure":
		fs := flag.NewFlagSet("configure", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return cmdConfigure(*profile)
	case "search":
		fs := flag.NewFlagSet("search", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		sortTimestamp := fs.String("sort-timestamp", "last_edited_time", "")
		sortDirection := fs.String("sort-direction", "descending", "")
		startCursor := fs.String("start-cursor", "", "")
		pageSize := fs.Int("page-size", 10, "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		query := ""
		if fs.NArg() > 0 {
			query = fs.Arg(0)
		}
		if *sortDirection != "ascending" && *sortDirection != "descending" {
			return cliError{"--sort-direction must be one of: ascending, descending."}
		}
		return cmdSearch(*profile, query, *sortTimestamp, *sortDirection, *startCursor, *pageSize)
	case "fetch-page":
		fs := flag.NewFlagSet("fetch-page", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		slice := fs.String("slice", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return cliError{"fetch-page requires PAGE_ID."}
		}
		return cmdFetchPage(*profile, fs.Arg(0), *slice)
	case "fetch-database":
		fs := flag.NewFlagSet("fetch-database", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return cliError{"fetch-database requires DATABASE_ID."}
		}
		return cmdFetchDatabase(*profile, fs.Arg(0))
	case "fetch-data-source":
		fs := flag.NewFlagSet("fetch-data-source", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return cliError{"fetch-data-source requires DATA_SOURCE_ID."}
		}
		return cmdFetchDataSource(*profile, fs.Arg(0))
	case "query-data-source":
		fs := flag.NewFlagSet("query-data-source", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		sorts := fs.String("sorts", "", "")
		filter := fs.String("filter", "", "")
		startCursor := fs.String("start-cursor", "", "")
		pageSize := fs.Int("page-size", 10, "")
		inTrash := fs.Bool("in-trash", false, "")
		resultType := fs.String("result-type", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return cliError{"query-data-source requires DATA_SOURCE_ID."}
		}
		return cmdQueryDataSource(*profile, fs.Arg(0), *sorts, *filter, *startCursor, *pageSize, *inTrash, *resultType)
	case "create-page":
		fs := flag.NewFlagSet("create-page", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		parentPageID := fs.String("parent-page-id", "", "")
		parentDataSourceID := fs.String("parent-data-source-id", "", "")
		content := fs.String("content", "", "")
		contentFile := fs.String("content-file", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return cliError{"create-page requires TITLE."}
		}
		return cmdCreatePage(*profile, fs.Arg(0), *parentPageID, *parentDataSourceID, *content, *contentFile)
	case "move-page":
		fs := flag.NewFlagSet("move-page", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		parentPageID := fs.String("parent-page-id", "", "")
		parentDataSourceID := fs.String("parent-data-source-id", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return cliError{"move-page requires PAGE_ID."}
		}
		hasPage := strings.TrimSpace(*parentPageID) != ""
		hasDataSource := strings.TrimSpace(*parentDataSourceID) != ""
		if hasPage == hasDataSource {
			return cliError{"move-page requires exactly one of --parent-page-id or --parent-data-source-id."}
		}
		return cmdMovePage(*profile, fs.Arg(0), *parentPageID, *parentDataSourceID)
	case "update-page":
		fs := flag.NewFlagSet("update-page", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		replace := fs.Bool("replace", false, "")
		content := fs.String("content", "", "")
		contentFile := fs.String("content-file", "", "")
		replaceAll := fs.Bool("replace-all-matches", false, "")
		allowDeleting := fs.Bool("allow-deleting-content", false, "")
		var olds stringSliceFlag
		var news stringSliceFlag
		fs.Var(&olds, "old", "")
		fs.Var(&news, "new", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return cliError{"update-page requires PAGE_ID."}
		}
		return cmdUpdatePage(*profile, fs.Arg(0), *replace, *content, *contentFile, []string(olds), []string(news), *replaceAll, *allowDeleting)
	default:
		globalUsage()
		return nil
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		failf("%s", err)
	}
}
