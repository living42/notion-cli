package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Two env vars gate the suite; everything else is created (and torn down) here.
//
//	NOTION_CLI_E2E_TESTING_SECRET            Notion integration secret.
//	NOTION_CLI_E2E_TESTING_ROOT_PAGE_LINK    A page the integration has access to;
//	                                         the test creates all fixtures under it
//	                                         and archives every child at teardown.
const (
	e2eSecretEnv       = "NOTION_CLI_E2E_TESTING_SECRET"
	e2eRootPageLinkEnv = "NOTION_CLI_E2E_TESTING_ROOT_PAGE_LINK"
	e2eConfigPathEnv   = "NOTION_CLI_CONFIG_PATH"

	e2eProfile = "e2e"
)

// Shared fixtures, populated by TestMain and consumed by tests.
var (
	binaryPath    string
	rootPageID    string
	scratchPageID string
	testDBID      string
	testDSID      string
)

func TestMain(m *testing.M) {
	secret := os.Getenv(e2eSecretEnv)
	rootLink := os.Getenv(e2eRootPageLinkEnv)

	if secret != "" && rootLink != "" {
		var err error
		if binaryPath, err = buildBinary(); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: build: %v\n", err)
			os.Exit(1)
		}

		if rootPageID, err = pageIDFromLink(rootLink); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: parse root link: %v\n", err)
			os.Exit(1)
		}

		if err := setupFixtures(secret); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: setup: %v\n", err)
			os.Exit(1)
		}
	}

	code := m.Run()

	// Always run cleanup if we got far enough to set rootPageID.
	if rootPageID != "" {
		if err := cleanupRoot(secret, rootPageID); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: cleanup: %v\n", err)
		}
	}

	os.Exit(code)
}

// --- binary build & config setup -------------------------------------------

func buildBinary() (string, error) {
	tmpDir, err := os.MkdirTemp("", "notion-cli-e2e-")
	if err != nil {
		return "", err
	}
	bin := filepath.Join(tmpDir, "notion-cli")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		os.RemoveAll(tmpDir)
		return "", err
	}
	return bin, nil
}

// pageIDFromLink accepts a Notion share URL, a bare 32-char hex, or a dashed
// UUID, and returns the canonical dashed lowercase form.
func pageIDFromLink(link string) (string, error) {
	re := regexp.MustCompile(`([0-9a-fA-F]{32}|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})`)
	m := re.FindStringSubmatch(link)
	if len(m) < 2 {
		return "", fmt.Errorf("no page ID found in: %s", link)
	}
	return normalizeNotionID(m[1], "page")
}

// setupE2E (per-test) writes a temp config so the child process authenticates
// with the test secret and never touches the user's real config.
func setupE2E(t *testing.T) {
	t.Helper()
	if os.Getenv(e2eSecretEnv) == "" || os.Getenv(e2eRootPageLinkEnv) == "" {
		t.Skipf("set %s and %s to enable e2e tests", e2eSecretEnv, e2eRootPageLinkEnv)
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	t.Setenv(e2eConfigPathEnv, configPath)

	cfg := Config{
		Profiles: map[string]Profile{
			e2eProfile: {NotionSecret: os.Getenv(e2eSecretEnv)},
		},
	}
	if err := cfg.save(); err != nil {
		t.Fatalf("write test config: %v", err)
	}
}

// --- fixture creation (TestMain-time) --------------------------------------

// setupFixtures creates the shared resources under root.
//   - scratch page: built via the CLI (the thing under test)
//   - test database + data source: built via the CLI's create-database command
func setupFixtures(secret string) error {
	ts := time.Now().UnixNano()

	scratchID, err := createPageViaCLISetup(secret, fmt.Sprintf("e2e-scratch-%d", ts), rootPageID, "")
	if err != nil {
		return fmt.Errorf("create scratch: %w", err)
	}
	scratchPageID = scratchID

	dbID, dsID, err := createDatabaseViaCLI(secret, rootPageID, fmt.Sprintf("e2e-test-db-%d", ts))
	if err != nil {
		_ = trashPageViaCLI(secret, scratchID)
		return fmt.Errorf("create test database: %w", err)
	}
	testDBID = dbID
	testDSID = dsID

	return nil
}

// createPageViaCLISetup runs the CLI as a subprocess from TestMain, where no
// per-test temp config exists yet. It writes its own temp config and passes
// the path through NOTION_CLI_CONFIG_PATH.
func createPageViaCLISetup(secret, title, parentPageID, content string) (string, error) {
	configPath, cleanup, err := writeTempConfig(secret)
	if err != nil {
		return "", err
	}
	defer cleanup()

	args := []string{"-p", e2eProfile, "create-page", title, "--parent-page-id", parentPageID}
	if content != "" {
		args = append(args, "--content", content)
	}

	cmd := exec.Command(binaryPath, args...)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		e2eConfigPathEnv + "=" + configPath,
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("create page via cli: %w\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	return extractPageID(stdout.String())
}

func writeTempConfig(secret string) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "notion-cli-e2e-cfg-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	configPath := filepath.Join(tmpDir, "config.json")
	cfg := Config{
		Profiles: map[string]Profile{
			e2eProfile: {NotionSecret: secret},
		},
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	b = append(b, '\n')
	if err := os.WriteFile(configPath, b, 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	return configPath, cleanup, nil
}

// createDatabaseViaCLI runs the create-database command as a subprocess and
// returns the database ID and its first data source ID.
func createDatabaseViaCLI(secret, parentPageID, title string) (string, string, error) {
	configPath, cleanup, err := writeTempConfig(secret)
	if err != nil {
		return "", "", err
	}
	defer cleanup()

	cmd := exec.Command(binaryPath, "-p", e2eProfile, "create-database", title, "--parent-page-id", parentPageID)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		e2eConfigPathEnv + "=" + configPath,
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("create database via cli: %w\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &data); err != nil {
		return "", "", fmt.Errorf("create database: parse output: %w\noutput: %s", err, stdout.String())
	}
	dbID := asString(data["id"])
	if dbID == "" {
		return "", "", fmt.Errorf("create database: no id in response")
	}
	dss, _ := data["data_sources"].([]any)
	if len(dss) == 0 {
		return dbID, "", fmt.Errorf("create database: no data sources in response")
	}
	first, _ := dss[0].(map[string]any)
	dsID := asString(first["id"])
	if dsID == "" {
		return dbID, "", fmt.Errorf("create database: data source has no id")
	}
	return dbID, dsID, nil
}

// trashPageViaCLI runs the trash-page command as a subprocess. It is used
// only on the setup-failure path where we need best-effort cleanup.
func trashPageViaCLI(secret, pageID string) error {
	configPath, cleanup, err := writeTempConfig(secret)
	if err != nil {
		return err
	}
	defer cleanup()

	cmd := exec.Command(binaryPath, "-p", e2eProfile, "trash-page", pageID)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		e2eConfigPathEnv + "=" + configPath,
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("trash page via cli: %w\nstderr: %s", err, stderr.String())
	}
	return nil
}

func extractPageID(output string) (string, error) {
	re := regexp.MustCompile(`page_id:\s*([0-9a-fA-F-]{36})`)
	m := re.FindStringSubmatch(output)
	if len(m) < 2 {
		return "", fmt.Errorf("no page_id in output:\n%s", output)
	}
	return m[1], nil
}

// --- cleanup ---------------------------------------------------------------

// cleanupRoot moves every direct child of the root page to trash via the CLI.
// Notion's trash is recursive: trashing a page also trashes its subpages and
// trashing a database also trashes its data sources, so a single pass is enough.
func cleanupRoot(secret, parentID string) error {
	configPath, cleanup, err := writeTempConfig(secret)
	if err != nil {
		return err
	}
	defer cleanup()

	children, err := listBlockChildrenViaCLI(configPath, parentID)
	if err != nil {
		return err
	}
	var errs []string
	for _, child := range children {
		id, _ := child["id"].(string)
		blockType, _ := child["type"].(string)
		if id == "" {
			continue
		}
		var trashErr error
		switch blockType {
		case "child_page":
			trashErr = runNotionCmd(configPath, "trash-page", id)
		case "child_database":
			trashErr = runNotionCmd(configPath, "trash-database", id)
		default:
			continue
		}
		if trashErr != nil {
			errs = append(errs, fmt.Sprintf("%s %s: %v", blockType, id, trashErr))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// listBlockChildrenViaCLI runs list-block-children and returns the results as
// parsed maps. It is used only by cleanupRoot; the per-test list test calls
// the binary directly via runNotion to assert on raw output.
func listBlockChildrenViaCLI(configPath, blockID string) ([]map[string]any, error) {
	cmd := exec.Command(binaryPath, "-p", e2eProfile, "list-block-children", blockID, "--page-size", "100")
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		e2eConfigPathEnv + "=" + configPath,
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("list-block-children via cli: %w\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &data); err != nil {
		return nil, fmt.Errorf("list-block-children: parse output: %w", err)
	}
	raw := asSlice(data["results"])
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		if m, ok := r.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// runNotionCmd is a thin subprocess runner used by the TestMain-time cleanup
// path; it returns the first non-empty error message rather than aborting.
func runNotionCmd(configPath string, args ...string) error {
	full := append([]string{"-p", e2eProfile}, args...)
	cmd := exec.Command(binaryPath, full...)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		e2eConfigPathEnv + "=" + configPath,
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return fmt.Errorf("%w: %s", err, msg)
	}
	return nil
}

// --- subprocess runner -----------------------------------------------------

type runResult struct {
	stdout   string
	stderr   string
	exitCode int
}

func (r runResult) mustSucceed(t *testing.T) runResult {
	t.Helper()
	if r.exitCode != 0 {
		t.Errorf("expected exit 0, got %d\nstdout: %s\nstderr: %s", r.exitCode, r.stdout, r.stderr)
	}
	return r
}

func (r runResult) mustFail(t *testing.T) runResult {
	t.Helper()
	if r.exitCode == 0 {
		t.Errorf("expected non-zero exit, got 0\nstdout: %s\nstderr: %s", r.stdout, r.stderr)
	}
	return r
}

func runNotion(t *testing.T, args ...string) runResult {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := runResult{stdout: stdout.String(), stderr: stderr.String()}
	if err == nil {
		return res
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.exitCode = exitErr.ExitCode()
		return res
	}
	t.Fatalf("run notion %v: %v\nstdout: %s\nstderr: %s", args, err, stdout.String(), stderr.String())
	return res
}

// --- version & help ---------------------------------------------------------

func TestE2E_Version(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "--version").mustSucceed(t)
	if !strings.Contains(r.stdout, "notion version") {
		t.Errorf("expected 'notion version' in output, got: %q", r.stdout)
	}
}

func TestE2E_ShortVersion(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-v").mustSucceed(t)
	if !strings.Contains(r.stdout, "notion version") {
		t.Errorf("expected version string, got: %q", r.stdout)
	}
}

func TestE2E_Help(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "--help").mustSucceed(t)
	for _, cmd := range []string{
		"configure", "search", "fetch-page", "fetch-database", "fetch-data-source",
		"query-data-source", "create-page", "create-database", "list-block-children",
		"move-page", "update-page", "trash-page", "trash-database", "trash-data-source",
	} {
		if !strings.Contains(r.stdout, cmd) {
			t.Errorf("--help missing %q", cmd)
		}
	}
}

func TestE2E_SubcommandHelp(t *testing.T) {
	setupE2E(t)
	for _, sub := range []string{
		"search", "fetch-page", "create-page", "create-database", "list-block-children",
		"update-page", "move-page", "query-data-source",
		"trash-page", "trash-database", "trash-data-source",
	} {
		t.Run(sub, func(t *testing.T) {
			r := runNotion(t, sub, "--help").mustSucceed(t)
			if !strings.Contains(r.stdout, "Usage:") {
				t.Errorf("%s --help missing 'Usage:', got: %q", sub, r.stdout)
			}
		})
	}
}

// --- profile handling -------------------------------------------------------

func TestE2E_ProfileAfterSubcommand(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "search", "-p", e2eProfile, "e2e", "--page-size", "1").mustSucceed(t)
	if r.stdout == "" {
		t.Errorf("expected non-empty search output")
	}
}

func TestE2E_ProfileEqualsForm(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "search", "--profile="+e2eProfile, "e2e", "--page-size", "1").mustSucceed(t)
	if r.stdout == "" {
		t.Errorf("expected non-empty search output")
	}
}

func TestE2E_ProfileNotConfigured(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", "nonexistent-profile", "search", "e2e").mustFail(t)
	if !strings.Contains(r.stderr, "not configured") {
		t.Errorf("expected 'not configured' error, got: %s", r.stderr)
	}
}

// --- argument validation ----------------------------------------------------

func TestE2E_InvalidPageID(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "fetch-page", "not-a-uuid").mustFail(t)
	if !strings.Contains(r.stderr, "Invalid page ID") {
		t.Errorf("expected 'Invalid page ID', got: %s", r.stderr)
	}
}

func TestE2E_InvalidPageSize(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "search", "e2e", "--page-size", "999").mustFail(t)
	if !strings.Contains(r.stderr, "page-size") {
		t.Errorf("expected page-size error, got: %s", r.stderr)
	}
}

func TestE2E_NegativePageSize(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "search", "e2e", "--page-size", "0").mustFail(t)
	if !strings.Contains(r.stderr, "page-size") {
		t.Errorf("expected page-size error, got: %s", r.stderr)
	}
}

func TestE2E_InvalidSlice(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "fetch-page", scratchPageID, "--slice", "bad").mustFail(t)
	if !strings.Contains(r.stderr, "--slice") {
		t.Errorf("expected --slice error, got: %s", r.stderr)
	}
}

func TestE2E_SliceStartAfterEnd(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "fetch-page", scratchPageID, "--slice", "5-2").mustFail(t)
	if !strings.Contains(r.stderr, "0 <= N <= M") {
		t.Errorf("expected range error, got: %s", r.stderr)
	}
}

func TestE2E_MissingArg(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "fetch-page").mustFail(t)
	if r.stderr == "" {
		t.Errorf("expected cobra arg error on stderr")
	}
}

func TestE2E_InvalidSortDirection(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "search", "e2e", "--sort-direction", "sideways").mustFail(t)
	if !strings.Contains(r.stderr, "sort-direction") {
		t.Errorf("expected sort-direction error, got: %s", r.stderr)
	}
}

// --- create-page validation -------------------------------------------------

func TestE2E_CreatePageNoParent(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "create-page", "T").mustFail(t)
	if !strings.Contains(r.stderr, "parent") {
		t.Errorf("expected parent error, got: %s", r.stderr)
	}
}

func TestE2E_CreatePageBothParents(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "create-page", "T",
		"--parent-page-id", "00000000000000000000000000000001",
		"--parent-data-source-id", "00000000000000000000000000000002",
	).mustFail(t)
	if !strings.Contains(r.stderr, "exactly one") {
		t.Errorf("expected 'exactly one' error, got: %s", r.stderr)
	}
}

func TestE2E_CreatePageEmptyTitle(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "create-page", "   ",
		"--parent-page-id", "00000000000000000000000000000001",
	).mustFail(t)
	if !strings.Contains(r.stderr, "Title") && !strings.Contains(r.stderr, "empty") {
		t.Errorf("expected empty title error, got: %s", r.stderr)
	}
}

// --- move-page validation ---------------------------------------------------

func TestE2E_MovePageNoParent(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "move-page", scratchPageID).mustFail(t)
	if !strings.Contains(r.stderr, "parent") {
		t.Errorf("expected parent error, got: %s", r.stderr)
	}
}

func TestE2E_MovePageBothParents(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "move-page", scratchPageID,
		"--parent-page-id", "00000000000000000000000000000001",
		"--parent-data-source-id", "00000000000000000000000000000002",
	).mustFail(t)
	if !strings.Contains(r.stderr, "exactly one") {
		t.Errorf("expected 'exactly one' error, got: %s", r.stderr)
	}
}

func TestE2E_MovePageSelf(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "move-page", scratchPageID,
		"--parent-page-id", scratchPageID,
	).mustFail(t)
	if !strings.Contains(r.stderr, "different") {
		t.Errorf("expected 'different' error, got: %s", r.stderr)
	}
}

// --- update-page validation -------------------------------------------------

func TestE2E_UpdatePageNoContent(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "update-page", scratchPageID).mustFail(t)
	if !strings.Contains(r.stderr, "--old") && !strings.Contains(r.stderr, "--replace") {
		t.Errorf("expected --old/--new or --replace error, got: %s", r.stderr)
	}
}

func TestE2E_UpdatePageMismatchedOldNew(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "update-page", scratchPageID,
		"--old", "foo", "--new", "bar", "--old", "baz",
	).mustFail(t)
	if !strings.Contains(r.stderr, "same number") {
		t.Errorf("expected count mismatch error, got: %s", r.stderr)
	}
}

func TestE2E_UpdatePageReplaceWithOldNew(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "update-page", scratchPageID,
		"--replace", "--old", "foo", "--new", "bar",
	).mustFail(t)
	if !strings.Contains(r.stderr, "Do not mix") {
		t.Errorf("expected 'Do not mix' error, got: %s", r.stderr)
	}
}

func TestE2E_UpdatePageContentWithoutReplace(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "update-page", scratchPageID,
		"--content", "hello",
	).mustFail(t)
	if !strings.Contains(r.stderr, "--replace") {
		t.Errorf("expected --replace error, got: %s", r.stderr)
	}
}

// --- query-data-source validation -------------------------------------------

func TestE2E_QueryDataSourceInvalidSorts(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "query-data-source", testDSID, "--sorts", "not-json").mustFail(t)
	if !strings.Contains(r.stderr, "JSON") {
		t.Errorf("expected JSON error, got: %s", r.stderr)
	}
}

func TestE2E_QueryDataSourceSortsNotArray(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "query-data-source", testDSID, "--sorts", `{"foo":"bar"}`).mustFail(t)
	if !strings.Contains(r.stderr, "array") {
		t.Errorf("expected array error, got: %s", r.stderr)
	}
}

func TestE2E_QueryDataSourceFilterNotObject(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "query-data-source", testDSID, "--filter", `[1,2,3]`).mustFail(t)
	if !strings.Contains(r.stderr, "object") {
		t.Errorf("expected object error, got: %s", r.stderr)
	}
}

// --- read operations (API) --------------------------------------------------

func TestE2E_Search(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "search", "", "--page-size", "5").mustSucceed(t)
	if r.stdout == "" {
		t.Errorf("expected non-empty search output")
	}
}

func TestE2E_SearchWithQuery(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "search", "e2e", "--page-size", "3").mustSucceed(t)
	if r.stdout == "" {
		t.Errorf("expected non-empty search output")
	}
}

func TestE2E_SearchWithSort(t *testing.T) {
	setupE2E(t)
	// Notion only accepts `last_edited_time` for the search sort timestamp;
	// using `created_time` is rejected by the server.
	r := runNotion(t, "-p", e2eProfile, "search", "",
		"--sort-timestamp", "last_edited_time",
		"--sort-direction", "ascending",
		"--page-size", "3",
	).mustSucceed(t)
	if r.stdout == "" {
		t.Errorf("expected non-empty search output")
	}
}

func TestE2E_FetchPage(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "fetch-page", scratchPageID).mustSucceed(t)
	if !strings.Contains(r.stdout, "page_id:") {
		t.Errorf("expected metadata block, got: %s", r.stdout)
	}
}

func TestE2E_FetchPageWithSlice(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "fetch-page", scratchPageID, "--slice", "0-2").mustSucceed(t)
	if !strings.Contains(r.stdout, "slice: 0-2") {
		t.Errorf("expected slice metadata, got: %s", r.stdout)
	}
}

func TestE2E_FetchPageNotFound(t *testing.T) {
	setupE2E(t)
	fakeID := "00000000-0000-0000-0000-000000000000"
	r := runNotion(t, "-p", e2eProfile, "fetch-page", fakeID).mustFail(t)
	if !strings.Contains(r.stderr, "Error") {
		t.Errorf("expected API error, got: %s", r.stderr)
	}
}

func TestE2E_FetchPageHexID(t *testing.T) {
	setupE2E(t)
	hexID := strings.ReplaceAll(scratchPageID, "-", "")
	r := runNotion(t, "-p", e2eProfile, "fetch-page", hexID).mustSucceed(t)
	if !strings.Contains(r.stdout, "page_id:") {
		t.Errorf("expected metadata block, got: %s", r.stdout)
	}
}

func TestE2E_FetchDatabase(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "fetch-database", testDBID).mustSucceed(t)
	var data map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &data); err != nil {
		t.Fatalf("expected JSON, got: %s\nerr: %v", r.stdout, err)
	}
	if data["object"] != "database" {
		t.Errorf("expected object=database, got: %v", data["object"])
	}
}

func TestE2E_FetchDataSource(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "fetch-data-source", testDSID).mustSucceed(t)
	var data map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &data); err != nil {
		t.Fatalf("expected JSON, got: %s\nerr: %v", r.stdout, err)
	}
	if data["object"] != "data_source" {
		t.Errorf("expected object=data_source, got: %v", data["object"])
	}
}

func TestE2E_QueryDataSource(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "query-data-source", testDSID, "--page-size", "5").mustSucceed(t)
	var data map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &data); err != nil {
		t.Fatalf("expected JSON, got: %s", r.stdout)
	}
	if _, ok := data["results"]; !ok {
		t.Errorf("expected results key, got: %v", data)
	}
}

func TestE2E_QueryDataSourceWithSorts(t *testing.T) {
	setupE2E(t)
	sorts := `[{"timestamp": "last_edited_time", "direction": "descending"}]`
	r := runNotion(t, "-p", e2eProfile, "query-data-source", testDSID, "--sorts", sorts, "--page-size", "3").mustSucceed(t)
	if r.stdout == "" {
		t.Errorf("expected non-empty output")
	}
}

func TestE2E_QueryDataSourceWithFilter(t *testing.T) {
	setupE2E(t)
	// Match nothing; a syntactically-valid filter is enough to exercise the path.
	filter := `{"property": "Name", "title": {"equals": "zzz_no_match_e2e_zzz"}}`
	r := runNotion(t, "-p", e2eProfile, "query-data-source", testDSID, "--filter", filter).mustSucceed(t)
	var data map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &data); err != nil {
		t.Fatalf("expected JSON, got: %s", r.stdout)
	}
}

// --- write operations (API, destructive) ------------------------------------

// These create or modify resources under the root page. Cleanup at teardown
// archives every child, so any page created here is wiped out automatically.

func TestE2E_CreatePage(t *testing.T) {
	setupE2E(t)
	title := fmt.Sprintf("e2e-test-%d", time.Now().UnixNano())
	r := runNotion(t, "-p", e2eProfile, "create-page", title,
		"--parent-page-id", rootPageID,
	).mustSucceed(t)
	if !strings.Contains(r.stdout, "# ✅ Created Page") {
		t.Errorf("expected created-page banner, got: %s", r.stdout)
	}
	if !strings.Contains(r.stdout, title) {
		t.Errorf("expected title in output, got: %s", r.stdout)
	}
}

func TestE2E_CreatePageWithContent(t *testing.T) {
	setupE2E(t)
	title := fmt.Sprintf("e2e-content-%d", time.Now().UnixNano())
	content := "# Hello\nThis is e2e test content."
	r := runNotion(t, "-p", e2eProfile, "create-page", title,
		"--parent-page-id", rootPageID,
		"--content", content,
	).mustSucceed(t)
	if !strings.Contains(r.stdout, title) {
		t.Errorf("expected title in output, got: %s", r.stdout)
	}
}

func TestE2E_UpdatePageReplace(t *testing.T) {
	setupE2E(t)
	content := fmt.Sprintf("e2e replace %d", time.Now().UnixNano())
	r := runNotion(t, "-p", e2eProfile, "update-page", scratchPageID,
		"--replace", "--content", content,
	).mustSucceed(t)
	if !strings.Contains(r.stdout, "mode: replace_content") {
		t.Errorf("expected replace_content mode, got: %s", r.stdout)
	}
}

func TestE2E_UpdatePageOldNew(t *testing.T) {
	setupE2E(t)
	seed := fmt.Sprintf("UNIQUE_%d", time.Now().UnixNano())
	runNotion(t, "-p", e2eProfile, "update-page", scratchPageID,
		"--replace", "--content", seed,
	).mustSucceed(t)
	r := runNotion(t, "-p", e2eProfile, "update-page", scratchPageID,
		"--old", seed, "--new", "REPLACED",
	).mustSucceed(t)
	if !strings.Contains(r.stdout, "mode: update_content") {
		t.Errorf("expected update_content mode, got: %s", r.stdout)
	}
}

func TestE2E_UpdatePageOldNewReplaceAll(t *testing.T) {
	setupE2E(t)
	seed := fmt.Sprintf("TOKEN_%d", time.Now().UnixNano())
	runNotion(t, "-p", e2eProfile, "update-page", scratchPageID,
		"--replace", "--content", seed+" "+seed,
	).mustSucceed(t)
	r := runNotion(t, "-p", e2eProfile, "update-page", scratchPageID,
		"--old", seed, "--new", "X", "--replace-all-matches",
	).mustSucceed(t)
	if !strings.Contains(r.stdout, "mode: update_content") {
		t.Errorf("expected update_content mode, got: %s", r.stdout)
	}
}

// --- create-database / list-block-children / trash-* (happy paths) ---------

func TestE2E_CreateDatabase(t *testing.T) {
	setupE2E(t)
	title := fmt.Sprintf("e2e-create-db-%d", time.Now().UnixNano())
	r := runNotion(t, "-p", e2eProfile, "create-database", title,
		"--parent-page-id", rootPageID,
	).mustSucceed(t)
	var data map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &data); err != nil {
		t.Fatalf("expected JSON, got: %s\nerr: %v", r.stdout, err)
	}
	if data["object"] != "database" {
		t.Errorf("expected object=database, got: %v", data["object"])
	}
	if data["id"] == "" {
		t.Errorf("expected id, got: %v", data["id"])
	}
	dss, ok := data["data_sources"].([]any)
	if !ok || len(dss) == 0 {
		t.Fatalf("expected data_sources array, got: %v", data["data_sources"])
	}
	first, _ := dss[0].(map[string]any)
	if asString(first["id"]) == "" {
		t.Errorf("expected data_sources[0].id, got: %v", first)
	}
}

func TestE2E_ListBlockChildren(t *testing.T) {
	setupE2E(t)
	r := runNotion(t, "-p", e2eProfile, "list-block-children", rootPageID, "--page-size", "100").mustSucceed(t)
	var data map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &data); err != nil {
		t.Fatalf("expected JSON, got: %s\nerr: %v", r.stdout, err)
	}
	if data["object"] != "list" {
		t.Errorf("expected object=list, got: %v", data["object"])
	}
	results, ok := data["results"].([]any)
	if !ok {
		t.Fatalf("expected results array, got: %T", data["results"])
	}
	// root is the e2e page; it always has at least the test database and scratch page.
	if len(results) == 0 {
		t.Errorf("expected non-empty results under root, got: %s", r.stdout)
	}
}

func TestE2E_TrashPage(t *testing.T) {
	setupE2E(t)
	title := fmt.Sprintf("e2e-trash-page-%d", time.Now().UnixNano())
	createR := runNotion(t, "-p", e2eProfile, "create-page", title,
		"--parent-page-id", rootPageID,
	).mustSucceed(t)
	pageID := mustExtractIDFromMetadata(t, createR.stdout, "page_id")

	r := runNotion(t, "-p", e2eProfile, "trash-page", pageID).mustSucceed(t)
	if !strings.Contains(r.stdout, "Moved Page to Trash") {
		t.Errorf("expected trash banner, got: %s", r.stdout)
	}
	if !strings.Contains(r.stdout, "in_trash: true") {
		t.Errorf("expected in_trash: true in metadata, got: %s", r.stdout)
	}
	// A trashed page is still fetchable on Notion's API, but the in_trash flag
	// is preserved on the resource; verify by re-fetching and checking it.
	refetch := runNotion(t, "-p", e2eProfile, "fetch-page", pageID).mustSucceed(t)
	if !strings.Contains(refetch.stdout, "page_id: "+pageID) {
		t.Errorf("expected refetch to still return the page, got: %s", refetch.stdout)
	}
}

func TestE2E_TrashDatabase(t *testing.T) {
	setupE2E(t)
	title := fmt.Sprintf("e2e-trash-db-%d", time.Now().UnixNano())
	createR := runNotion(t, "-p", e2eProfile, "create-database", title,
		"--parent-page-id", rootPageID,
	).mustSucceed(t)
	var data map[string]any
	if err := json.Unmarshal([]byte(createR.stdout), &data); err != nil {
		t.Fatalf("expected JSON, got: %s\nerr: %v", createR.stdout, err)
	}
	dbID := asString(data["id"])
	if dbID == "" {
		t.Fatalf("create-database response missing id: %s", createR.stdout)
	}

	r := runNotion(t, "-p", e2eProfile, "trash-database", dbID).mustSucceed(t)
	if !strings.Contains(r.stdout, "Moved Database to Trash") {
		t.Errorf("expected trash banner, got: %s", r.stdout)
	}
	if !strings.Contains(r.stdout, "in_trash: true") {
		t.Errorf("expected in_trash: true in metadata, got: %s", r.stdout)
	}
	// A trashed database is still fetchable; verify the refetch succeeds.
	refetch := runNotion(t, "-p", e2eProfile, "fetch-database", dbID).mustSucceed(t)
	var refetched map[string]any
	if err := json.Unmarshal([]byte(refetch.stdout), &refetched); err != nil {
		t.Fatalf("expected JSON on refetch, got: %s", refetch.stdout)
	}
	if refetched["id"] != dbID {
		t.Errorf("refetched id mismatch: got %v, want %s", refetched["id"], dbID)
	}
}

func TestE2E_TrashDataSource(t *testing.T) {
	setupE2E(t)
	// Reuse the fixture test database; trash only its data source.
	r := runNotion(t, "-p", e2eProfile, "trash-data-source", testDSID).mustSucceed(t)
	if !strings.Contains(r.stdout, "Moved Data Source to Trash") {
		t.Errorf("expected trash banner, got: %s", r.stdout)
	}
	if !strings.Contains(r.stdout, "in_trash: true") {
		t.Errorf("expected in_trash: true in metadata, got: %s", r.stdout)
	}
	// A trashed data source is still fetchable; verify the refetch succeeds.
	refetch := runNotion(t, "-p", e2eProfile, "fetch-data-source", testDSID).mustSucceed(t)
	var refetched map[string]any
	if err := json.Unmarshal([]byte(refetch.stdout), &refetched); err != nil {
		t.Fatalf("expected JSON on refetch, got: %s", refetch.stdout)
	}
	if refetched["id"] != testDSID {
		t.Errorf("refetched id mismatch: got %v, want %s", refetched["id"], testDSID)
	}
}

// mustExtractIDFromMetadata pulls a `key: <uuid>` line out of a CLI metadata
// block. The pattern is: `key: <id>` inside the `<!-- metadata ... -->` block.
func mustExtractIDFromMetadata(t *testing.T, output, key string) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `:\s*([0-9a-fA-F-]{36})`)
	m := re.FindStringSubmatch(output)
	if len(m) < 2 {
		t.Fatalf("no %s in output:\n%s", key, output)
	}
	return m[1]
}

