package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

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
	// Validate the parent choice up front; otherwise we'd issue a real API
	// request with a fake ID when the user passes both flags.
	if hasPage := strings.TrimSpace(parentPageID) != ""; hasPage == (strings.TrimSpace(parentDataSourceID) != "") {
		return cliError{"create-page requires exactly one of --parent-page-id or --parent-data-source-id (not both, not neither)."}
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

func NewCommand() *cobra.Command {
	var profile string

	rootCmd := &cobra.Command{
		Use:           "notion",
		Short:         "Lightweight Notion CLI for searching, reading, creating, moving, and updating pages, databases, and data sources.",
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
			return cmdConfigure(profile)
		},
	}

	var (
		searchSortTimestamp string
		searchSortDirection string
		searchStartCursor   string
		searchPageSize      int
	)
	searchCmd := &cobra.Command{
		Use:   "search [QUERY]",
		Short: "Search Notion",
		RunE: func(cmd *cobra.Command, args []string) error {
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			if searchSortDirection != "ascending" && searchSortDirection != "descending" {
				return cliError{"--sort-direction must be one of: ascending, descending."}
			}
			return cmdSearch(profile, query, searchSortTimestamp, searchSortDirection, searchStartCursor, searchPageSize)
		},
	}
	searchCmd.Flags().StringVar(&searchSortTimestamp, "sort-timestamp", "last_edited_time", "")
	searchCmd.Flags().StringVar(&searchSortDirection, "sort-direction", "descending", "")
	searchCmd.Flags().StringVar(&searchStartCursor, "start-cursor", "", "")
	searchCmd.Flags().IntVar(&searchPageSize, "page-size", 10, "")

	var fetchSlice string
	fetchPageCmd := &cobra.Command{
		Use:   "fetch-page PAGE_ID",
		Short: "Fetch a Notion page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdFetchPage(profile, args[0], fetchSlice)
		},
	}
	fetchPageCmd.Flags().StringVar(&fetchSlice, "slice", "", "")

	fetchDatabaseCmd := &cobra.Command{
		Use:   "fetch-database DATABASE_ID",
		Short: "Fetch a Notion database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdFetchDatabase(profile, args[0])
		},
	}

	fetchDataSourceCmd := &cobra.Command{
		Use:   "fetch-data-source DATA_SOURCE_ID",
		Short: "Fetch a Notion data source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdFetchDataSource(profile, args[0])
		},
	}

	var (
		querySorts      string
		queryFilter     string
		queryCursor     string
		queryPageSize   int
		queryInTrash    bool
		queryResultType string
	)
	queryDataSourceCmd := &cobra.Command{
		Use:   "query-data-source DATA_SOURCE_ID",
		Short: "Query a Notion data source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdQueryDataSource(profile, args[0], querySorts, queryFilter, queryCursor, queryPageSize, queryInTrash, queryResultType)
		},
	}
	queryDataSourceCmd.Flags().StringVar(&querySorts, "sorts", "", "")
	queryDataSourceCmd.Flags().StringVar(&queryFilter, "filter", "", "")
	queryDataSourceCmd.Flags().StringVar(&queryCursor, "start-cursor", "", "")
	queryDataSourceCmd.Flags().IntVar(&queryPageSize, "page-size", 10, "")
	queryDataSourceCmd.Flags().BoolVar(&queryInTrash, "in-trash", false, "")
	queryDataSourceCmd.Flags().StringVar(&queryResultType, "result-type", "", "")

	var (
		createParentPageID       string
		createParentDataSourceID string
		createContent            string
		createContentFile        string
	)
	createPageCmd := &cobra.Command{
		Use:   "create-page TITLE",
		Short: "Create a new Notion page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdCreatePage(profile, args[0], createParentPageID, createParentDataSourceID, createContent, createContentFile)
		},
	}
	createPageCmd.Flags().StringVar(&createParentPageID, "parent-page-id", "", "")
	createPageCmd.Flags().StringVar(&createParentDataSourceID, "parent-data-source-id", "", "")
	createPageCmd.Flags().StringVar(&createContent, "content", "", "")
	createPageCmd.Flags().StringVar(&createContentFile, "content-file", "", "")

	var (
		moveParentPageID       string
		moveParentDataSourceID string
	)
	movePageCmd := &cobra.Command{
		Use:   "move-page PAGE_ID",
		Short: "Move a Notion page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hasPage := strings.TrimSpace(moveParentPageID) != ""
			hasDataSource := strings.TrimSpace(moveParentDataSourceID) != ""
			if hasPage == hasDataSource {
				return cliError{"move-page requires exactly one of --parent-page-id or --parent-data-source-id."}
			}
			return cmdMovePage(profile, args[0], moveParentPageID, moveParentDataSourceID)
		},
	}
	movePageCmd.Flags().StringVar(&moveParentPageID, "parent-page-id", "", "")
	movePageCmd.Flags().StringVar(&moveParentDataSourceID, "parent-data-source-id", "", "")

	var (
		updateReplace       bool
		updateContent       string
		updateContentFile   string
		updateReplaceAll    bool
		updateAllowDeleting bool
		updateOlds          []string
		updateNews          []string
	)
	updatePageCmd := &cobra.Command{
		Use:   "update-page PAGE_ID",
		Short: "Update a Notion page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmdUpdatePage(profile, args[0], updateReplace, updateContent, updateContentFile, updateOlds, updateNews, updateReplaceAll, updateAllowDeleting)
		},
	}
	updatePageCmd.Flags().BoolVar(&updateReplace, "replace", false, "")
	updatePageCmd.Flags().StringVar(&updateContent, "content", "", "")
	updatePageCmd.Flags().StringVar(&updateContentFile, "content-file", "", "")
	updatePageCmd.Flags().BoolVar(&updateReplaceAll, "replace-all-matches", false, "")
	updatePageCmd.Flags().BoolVar(&updateAllowDeleting, "allow-deleting-content", false, "")
	updatePageCmd.Flags().StringArrayVar(&updateOlds, "old", nil, "")
	updatePageCmd.Flags().StringArrayVar(&updateNews, "new", nil, "")

	rootCmd.AddCommand(
		configureCmd,
		searchCmd,
		fetchPageCmd,
		fetchDatabaseCmd,
		fetchDataSourceCmd,
		queryDataSourceCmd,
		createPageCmd,
		movePageCmd,
		updatePageCmd,
	)

	return rootCmd
}
