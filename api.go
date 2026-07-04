package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const notionVersion = "2025-09-03"

var (
	uuidRe  = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	hex32Re = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
)

// Test seams: apiBaseURL and httpClient are overridable so tests can point
// the CLI at a local httptest server instead of the real Notion API.
var (
	apiBaseURL = "https://api.notion.com"
	httpClient = &http.Client{}
)

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
	url := apiBaseURL + path
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

	resp, err := httpClient.Do(req)
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

// firstDataSourceID returns the id of the first data source attached to a
// database object, or "" if there is none.
func firstDataSourceID(obj map[string]any) string {
	dss, ok := obj["data_sources"].([]any)
	if !ok || len(dss) == 0 {
		return ""
	}
	first, ok := dss[0].(map[string]any)
	if !ok {
		return ""
	}
	return asString(first["id"])
}

func buildCreatePageBody(title, parentKind, parentID, content, contentFile, dataSourceTitleProp string) (map[string]any, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, cliError{"Title cannot be empty."}
	}
	var parent map[string]any
	switch parentKind {
	case "page":
		parent = map[string]any{"type": "page_id", "page_id": parentID}
	case "data_source":
		parent = map[string]any{"type": "data_source_id", "data_source_id": parentID}
	default:
		return nil, cliError{"Parent must be a page or data source."}
	}
	titleProp := "title"
	if parentKind == "data_source" {
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

func buildMovePageBody(parentKind, parentID, pageID string) (map[string]any, map[string]any, error) {
	switch parentKind {
	case "page":
		if parentID == pageID {
			return nil, nil, cliError{"--parent must be different from the page being moved."}
		}
		parent := map[string]any{"type": "page_id", "page_id": parentID}
		return parent, map[string]any{"parent": parent}, nil
	case "data_source":
		parent := map[string]any{"type": "data_source_id", "data_source_id": parentID}
		return parent, map[string]any{"parent": parent}, nil
	default:
		return nil, nil, cliError{"Parent must be a page or data source."}
	}
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
