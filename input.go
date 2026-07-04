package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

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

func ifEmpty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
