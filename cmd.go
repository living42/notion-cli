package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// --- command handlers ------------------------------------------------------
//
// Each cmdXxx returns the output to print (and an error). The cobra RunE
// wrappers do the actual fmt.Println so handlers stay testable in-process
// (integration tests call them directly against an httptest server).

func cmdFind(profile, query, sortTimestamp, sortDirection, startCursor string, pageSize int, long, jsonOut bool) (string, error) {
	secret, err := selectedSecret(profile)
	if err != nil {
		return "", err
	}
	if sortDirection != "ascending" && sortDirection != "descending" {
		return "", cliError{"--sort-direction must be one of: ascending, descending."}
	}
	if _, err := validatePageSize(pageSize); err != nil {
		return "", err
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
		return "", err
	}
	if jsonOut {
		return prettyJSON(result), nil
	}
	if long {
		return formatSearchOutput(result), nil
	}
	return formatFindCompact(result), nil
}

func cmdLs(profile, ref string, long, jsonOut bool, pageSize int, startCursor, filterRaw, sortsRaw string, inTrash bool, resultType string) (string, error) {
	secret, err := selectedSecret(profile)
	if err != nil {
		return "", err
	}
	kind, normID, err := parseResourceRef(ref)
	if err != nil {
		return "", err
	}
	if _, err := validatePageSize(pageSize); err != nil {
		return "", err
	}
	if (filterRaw != "" || sortsRaw != "") && kind != "data_source" {
		return "", cliError{"--filter/--sorts are only valid for data sources (use: ls ds:<id>)."}
	}
	switch kind {
	case "page":
		return lsPage(secret, normID, long, jsonOut, pageSize, startCursor)
	case "database":
		return lsDatabase(secret, normID, long, jsonOut)
	case "data_source":
		return lsDataSource(secret, normID, long, jsonOut, pageSize, startCursor, filterRaw, sortsRaw, inTrash, resultType)
	}
	return "", cliError{"unsupported resource type: " + kind}
}

// lsPage lists a page's block children, auto-paginating through all results
// (mirrors the legacy list-block-children behavior).
func lsPage(secret, blockID string, long, jsonOut bool, pageSize int, startCursor string) (string, error) {
	all := make([]any, 0)
	cursor := startCursor
	var hasMore bool
	var nextCursor, requestID string

	for {
		path := "/v1/blocks/" + blockID + "/children"
		if cursor != "" {
			path += "?start_cursor=" + cursor
		}
		resp, err := notionGet(path, secret)
		if err != nil {
			return "", err
		}
		results := asSlice(resp["results"])
		all = append(all, results...)
		hasMore, _ = resp["has_more"].(bool)
		nextCursor = asString(resp["next_cursor"])
		requestID = asString(resp["request_id"])
		if !hasMore || nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	out := map[string]any{
		"object":   "list",
		"results":  all,
		"has_more": hasMore,
		"next_cursor": func() any {
			if nextCursor == "" {
				return nil
			}
			return nextCursor
		}(),
	}
	if requestID != "" {
		out["request_id"] = requestID
	}
	if jsonOut {
		return prettyJSON(out), nil
	}
	if long {
		return formatLsLong("page", out), nil
	}
	return formatLsCompact("page", out), nil
}

// lsDatabase lists the data sources of a database.
func lsDatabase(secret, dbID string, long, jsonOut bool) (string, error) {
	data, err := notionGet("/v1/databases/"+dbID, secret)
	if err != nil {
		return "", err
	}
	if jsonOut || long {
		return prettyJSON(data), nil
	}
	return formatLsCompact("database", data), nil
}

// lsDataSource queries a data source's rows (mirrors the legacy
// query-data-source behavior, including --filter/--sorts/--in-trash/--result-type).
func lsDataSource(secret, dsID string, long, jsonOut bool, pageSize int, startCursor, filterRaw, sortsRaw string, inTrash bool, resultType string) (string, error) {
	payload := map[string]any{}
	if sortsRaw != "" {
		sorts, err := parseJSONOption(sortsRaw, "array", "--sorts")
		if err != nil {
			return "", err
		}
		payload["sorts"] = sorts
	}
	if filterRaw != "" {
		filter, err := parseJSONOption(filterRaw, "object", "--filter")
		if err != nil {
			return "", err
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
	resp, err := notionPost("/v1/data_sources/"+dsID+"/query", secret, payload)
	if err != nil {
		return "", err
	}
	if jsonOut || long {
		return prettyJSON(resp), nil
	}
	return formatLsCompact("data_source", resp), nil
}

func cmdRead(profile, ref, sliceRaw string, long, metadata, jsonOut bool) (string, error) {
	secret, err := selectedSecret(profile)
	if err != nil {
		return "", err
	}
	kind, normID, err := parseResourceRef(ref)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(sliceRaw) != "" && kind != "page" {
		return "", cliError{"--slice is only valid for pages."}
	}
	switch kind {
	case "page":
		return readPage(secret, normID, sliceRaw, long, metadata, jsonOut)
	case "database":
		data, err := notionGet("/v1/databases/"+normID, secret)
		if err != nil {
			return "", err
		}
		return prettyJSON(data), nil
	case "data_source":
		data, err := notionGet("/v1/data_sources/"+normID, secret)
		if err != nil {
			return "", err
		}
		return prettyJSON(data), nil
	}
	return "", cliError{"unsupported resource type: " + kind}
}

// readPage fetches a page. Default output is the markdown body only
// (pipe-friendly); --metadata appends a metadata footer; -l renders the full
// title+URL+body+metadata block; --json returns the raw page object.
func readPage(secret, pageID, sliceRaw string, long, metadata, jsonOut bool) (string, error) {
	pageMeta, err := notionGet("/v1/pages/"+pageID, secret)
	if err != nil {
		return "", err
	}
	if jsonOut {
		return prettyJSON(pageMeta), nil
	}
	data, err := notionGet("/v1/pages/"+pageID+"/markdown", secret)
	if err != nil {
		return "", err
	}
	var sliceRange *[2]int
	if strings.TrimSpace(sliceRaw) != "" {
		v, err := parseSlice(sliceRaw)
		if err != nil {
			return "", err
		}
		sliceRange = &v
	}
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
	if long {
		return formatFetchOutput(data, pageMeta, sliceRange), nil
	}
	if metadata {
		return body + "\n\n---\n\n" + renderMetadataBlock(pageMetadataLines(data, sliceRange)), nil
	}
	return body, nil
}

func cmdMkdb(profile, title, parentRef, propertiesRaw string) (string, error) {
	secret, err := selectedSecret(profile)
	if err != nil {
		return "", err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return "", cliError{"Title cannot be empty."}
	}
	pKind, pParentID, err := parseResourceRef(parentRef)
	if err != nil {
		return "", err
	}
	if pKind != "page" {
		return "", cliError{"mkdb parent must be a page."}
	}

	properties := map[string]any{
		"Name": map[string]any{"title": map[string]any{}},
	}
	if strings.TrimSpace(propertiesRaw) != "" {
		parsed, err := parseJSONOption(propertiesRaw, "object", "--properties")
		if err != nil {
			return "", err
		}
		properties = asMap(parsed)
	}

	body := map[string]any{
		"parent":     map[string]any{"type": "page_id", "page_id": pParentID},
		"title":      []any{map[string]any{"text": map[string]any{"content": title}}},
		"properties": properties,
	}

	resp, err := notionPost("/v1/databases", secret, body)
	if err != nil {
		return "", err
	}
	if firstDataSourceID(resp) == "" {
		// Some server versions only attach data sources after a follow-up read.
		dbID := asString(resp["id"])
		if dbID != "" {
			refetched, err := notionGet("/v1/databases/"+dbID, secret)
			if err == nil {
				resp = refetched
			}
		}
	}
	return prettyJSON(resp), nil
}

// cmdWrite creates a page (legacy create-page). --parent is a page or data
// source (use the ds: prefix for the latter); content comes from --content,
// --content-file, or piped stdin.
func cmdWrite(profile, title, parentRef, content, contentFile string) (string, error) {
	secret, err := selectedSecret(profile)
	if err != nil {
		return "", err
	}
	pKind, pParentID, err := parseResourceRef(parentRef)
	if err != nil {
		return "", err
	}
	if pKind != "page" && pKind != "data_source" {
		return "", cliError{"write parent must be a page or data source."}
	}
	titleProp := ""
	if pKind == "data_source" {
		dataSource, err := notionGet("/v1/data_sources/"+pParentID, secret)
		if err != nil {
			return "", err
		}
		titleProp, err = extractTitlePropertyName(dataSource)
		if err != nil {
			return "", err
		}
	}
	body, err := buildCreatePageBody(title, pKind, pParentID, content, contentFile, titleProp)
	if err != nil {
		return "", err
	}
	pageData, err := notionPost("/v1/pages", secret, body)
	if err != nil {
		return "", err
	}
	return formatCreatePageOutput(pageData), nil
}

// cmdEdit modifies a page's content (legacy update-page). Either --replace
// (whole-page replacement) or one or more --old/--new pairs (search-replace).
func cmdEdit(profile, ref string, replace bool, content, contentFile string, olds, news []string, replaceAll, allowDeleting bool) (string, error) {
	secret, err := selectedSecret(profile)
	if err != nil {
		return "", err
	}
	kind, normID, err := parseResourceRef(ref)
	if err != nil {
		return "", err
	}
	if kind != "page" {
		return "", cliError{"edit only supports pages."}
	}
	mode, body, err := buildUpdatePageBody(replace, content, contentFile, olds, news, replaceAll, allowDeleting)
	if err != nil {
		return "", err
	}
	updateData, err := notionPatch("/v1/pages/"+normID+"/markdown", secret, body)
	if err != nil {
		return "", err
	}
	pageMeta, err := notionGet("/v1/pages/"+normID, secret)
	if err != nil {
		return "", err
	}
	return formatUpdatePageOutput(updateData, pageMeta, mode), nil
}

func cmdMv(profile, ref, parentRef string) (string, error) {
	secret, err := selectedSecret(profile)
	if err != nil {
		return "", err
	}
	kind, normID, err := parseResourceRef(ref)
	if err != nil {
		return "", err
	}
	if kind != "page" {
		return "", cliError{"mv only supports pages."}
	}
	pKind, pParentID, err := parseResourceRef(parentRef)
	if err != nil {
		return "", err
	}
	if pKind != "page" && pKind != "data_source" {
		return "", cliError{"mv parent must be a page or data source."}
	}
	parent, body, err := buildMovePageBody(pKind, pParentID, normID)
	if err != nil {
		return "", err
	}
	pageData, err := notionPatch("/v1/pages/"+normID, secret, body)
	if err != nil {
		return "", err
	}
	return formatMovePageOutput(pageData, parent), nil
}

func cmdRm(profile, ref string) (string, error) {
	secret, err := selectedSecret(profile)
	if err != nil {
		return "", err
	}
	kind, normID, err := parseResourceRef(ref)
	if err != nil {
		return "", err
	}
	switch kind {
	case "page":
		resp, err := notionPatch("/v1/pages/"+normID, secret, map[string]any{"in_trash": true})
		if err != nil {
			return "", err
		}
		return formatTrashPageOutput(resp, normID), nil
	case "database":
		resp, err := notionPatch("/v1/databases/"+normID, secret, map[string]any{"in_trash": true})
		if err != nil {
			return "", err
		}
		return formatTrashDatabaseOutput(resp, normID), nil
	case "data_source":
		resp, err := notionPatch("/v1/data_sources/"+normID, secret, map[string]any{"in_trash": true})
		if err != nil {
			return "", err
		}
		return formatTrashDataSourceOutput(resp, normID), nil
	}
	return "", cliError{"unsupported resource type: " + kind}
}

// --- profile arg normalization --------------------------------------------

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

// --- command tree ----------------------------------------------------------

func NewCommand() *cobra.Command {
	var profile string

	rootCmd := &cobra.Command{
		Use:           "notion",
		Short:         "Unix-style Notion CLI: find, ls, read, mkdb, write, edit, mv, and rm your Notion pages, databases, and data sources.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	rootCmd.Version = version
	rootCmd.SetVersionTemplate("notion version {{.Version}}\n")
	rootCmd.PersistentFlags().StringVarP(&profile, "profile", "p", "default", "Profile to use")

	configureCmd := &cobra.Command{
		Use:   "configure",
		Short: "Configure a Notion profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := cmdConfigure(profile)
			if err != nil {
				return err
			}
			if out != "" {
				fmt.Println(out)
			}
			return nil
		},
	}

	// find
	var (
		findSortTimestamp string
		findSortDirection string
		findStartCursor   string
		findPageSize      int
		findLong          bool
		findJSON          bool
	)
	findCmd := &cobra.Command{
		Use:   "find [QUERY]",
		Short: "Search your Notion workspace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			out, err := cmdFind(profile, query, findSortTimestamp, findSortDirection, findStartCursor, findPageSize, findLong, findJSON)
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		},
	}
	findCmd.Flags().StringVar(&findSortTimestamp, "sort-timestamp", "last_edited_time", "Sort timestamp: created_time or last_edited_time")
	findCmd.Flags().StringVar(&findSortDirection, "sort-direction", "descending", "Sort direction: ascending or descending")
	findCmd.Flags().StringVar(&findStartCursor, "start-cursor", "", "Pagination cursor")
	findCmd.Flags().IntVar(&findPageSize, "page-size", 10, "Number of results (1-100)")
	findCmd.Flags().BoolVarP(&findLong, "long", "l", false, "Rich, multi-line output")
	findCmd.Flags().BoolVar(&findJSON, "json", false, "Raw Notion JSON output")

	// ls
	var (
		lsPageSize    int
		lsStartCursor string
		lsFilter      string
		lsSorts       string
		lsInTrash     bool
		lsResultType  string
		lsLong        bool
		lsJSON        bool
	)
	lsCmd := &cobra.Command{
		Use:   "ls PATH",
		Short: "List the children of a page, database, or data source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := cmdLs(profile, args[0], lsLong, lsJSON, lsPageSize, lsStartCursor, lsFilter, lsSorts, lsInTrash, lsResultType)
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		},
	}
	lsCmd.Flags().IntVar(&lsPageSize, "page-size", 100, "Page size (1-100)")
	lsCmd.Flags().StringVar(&lsStartCursor, "start-cursor", "", "Pagination cursor")
	lsCmd.Flags().StringVar(&lsFilter, "filter", "", "Filter JSON (data sources only)")
	lsCmd.Flags().StringVar(&lsSorts, "sorts", "", "Sorts JSON array (data sources only)")
	lsCmd.Flags().BoolVar(&lsInTrash, "in-trash", false, "Include trashed entries (data sources only)")
	lsCmd.Flags().StringVar(&lsResultType, "result-type", "", "Notion result_type (data sources only)")
	lsCmd.Flags().BoolVarP(&lsLong, "long", "l", false, "Rich, multi-line output")
	lsCmd.Flags().BoolVar(&lsJSON, "json", false, "Raw Notion JSON output")

	// read
	var (
		readSlice string
		readLong  bool
		readMeta  bool
		readJSON  bool
	)
	readCmd := &cobra.Command{
		Use:   "read PATH",
		Short: "Read a page (Markdown), database, or data source (JSON)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := cmdRead(profile, args[0], readSlice, readLong, readMeta, readJSON)
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		},
	}
	readCmd.Flags().StringVar(&readSlice, "slice", "", "Show only lines N-M (pages only)")
	readCmd.Flags().BoolVarP(&readLong, "long", "l", false, "Rich output with title/URL/metadata (pages)")
	readCmd.Flags().BoolVar(&readMeta, "metadata", false, "Append a metadata footer (pages)")
	readCmd.Flags().BoolVar(&readJSON, "json", false, "Raw Notion JSON output")

	// mkdb
	var (
		mkdbParent     string
		mkdbProperties string
	)
	mkdbCmd := &cobra.Command{
		Use:   "mkdb TITLE",
		Short: "Create a database under a page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := cmdMkdb(profile, args[0], mkdbParent, mkdbProperties)
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		},
	}
	mkdbCmd.Flags().StringVar(&mkdbParent, "parent", "", "Parent page ID (page:<id> or bare ID)")
	if err := mkdbCmd.MarkFlagRequired("parent"); err != nil {
		panic(err)
	}
	mkdbCmd.Flags().StringVar(&mkdbProperties, "properties", "", "Properties schema JSON")

	// write (create page)
	var (
		writeParent      string
		writeContent     string
		writeContentFile string
	)
	writeCmd := &cobra.Command{
		Use:   "write TITLE",
		Short: "Create a page under a page or data source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := cmdWrite(profile, args[0], writeParent, writeContent, writeContentFile)
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		},
	}
	writeCmd.Flags().StringVar(&writeParent, "parent", "", "Parent page or data source (page:<id>, ds:<id>, or bare ID)")
	if err := writeCmd.MarkFlagRequired("parent"); err != nil {
		panic(err)
	}
	writeCmd.Flags().StringVar(&writeContent, "content", "", "Inline Markdown body")
	writeCmd.Flags().StringVar(&writeContentFile, "content-file", "", "Read Markdown body from a file")

	// edit (update page)
	var (
		editReplace       bool
		editContent       string
		editContentFile   string
		editReplaceAll    bool
		editAllowDeleting bool
		editOlds          []string
		editNews          []string
	)
	editCmd := &cobra.Command{
		Use:   "edit PAGE_REF",
		Short: "Update a page's content (replace or search-replace)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := cmdEdit(profile, args[0], editReplace, editContent, editContentFile, editOlds, editNews, editReplaceAll, editAllowDeleting)
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		},
	}
	editCmd.Flags().BoolVar(&editReplace, "replace", false, "Replace the entire page content")
	editCmd.Flags().StringVar(&editContent, "content", "", "Replacement Markdown content (with --replace)")
	editCmd.Flags().StringVar(&editContentFile, "content-file", "", "Read replacement Markdown from a file (with --replace)")
	editCmd.Flags().BoolVar(&editReplaceAll, "replace-all-matches", false, "Replace all matches for each --old")
	editCmd.Flags().BoolVar(&editAllowDeleting, "allow-deleting-content", false, "Allow operations that delete child pages or databases")
	editCmd.Flags().StringArrayVar(&editOlds, "old", nil, "Existing string to find (repeatable)")
	editCmd.Flags().StringArrayVar(&editNews, "new", nil, "Replacement string (repeatable)")

	// mv
	var mvParent string
	mvCmd := &cobra.Command{
		Use:   "mv PAGE_REF",
		Short: "Move a page under a different parent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := cmdMv(profile, args[0], mvParent)
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		},
	}
	mvCmd.Flags().StringVar(&mvParent, "parent", "", "Destination parent page or data source (page:<id>, ds:<id>, or bare ID)")
	if err := mvCmd.MarkFlagRequired("parent"); err != nil {
		panic(err)
	}

	// rm
	rmCmd := &cobra.Command{
		Use:   "rm PATH",
		Short: "Move a page, database, or data source to trash",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := cmdRm(profile, args[0])
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		},
	}

	rootCmd.AddCommand(
		configureCmd,
		findCmd,
		lsCmd,
		readCmd,
		mkdbCmd,
		writeCmd,
		editCmd,
		mvCmd,
		rmCmd,
	)

	return rootCmd
}
