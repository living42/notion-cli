package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config mirrors the on-disk JSON config file.
type Config struct {
	Profiles map[string]Profile `json:"profiles"`
}

// Profile is a single named Notion integration.
type Profile struct {
	NotionSecret string `json:"notion_secret"`
}

func configPath() string {
	if p := os.Getenv("NOTION_CLI_CONFIG_PATH"); p != "" {
		return p
	}
	h, err := os.UserHomeDir()
	if err != nil {
		failf("Unable to resolve home directory: %v", err)
	}
	return filepath.Join(h, ".config", "notion-cli", "config.json")
}

func loadConfig(required bool) (*Config, error) {
	path := configPath()
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if required {
				return nil, cliError{"No config found. Run `notion configure` first."}
			}
			return &Config{Profiles: map[string]Profile{}}, nil
		}
		return nil, err
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, cliError{fmt.Sprintf("Config file is malformed JSON: %s. Please re-run `notion configure`.", path)}
	}

	if raw == nil {
		return nil, cliError{fmt.Sprintf("Invalid config format in %s. Expected a JSON object.", path)}
	}

	cfg := &Config{Profiles: map[string]Profile{}}

	if profilesAny, ok := raw["profiles"].(map[string]any); ok {
		for name, pAny := range profilesAny {
			pMap, _ := pAny.(map[string]any)
			secret, _ := pMap["notion_secret"].(string)
			cfg.Profiles[name] = Profile{NotionSecret: secret}
		}
		return cfg, nil
	}

	if legacySecret, ok := raw["notion_secret"].(string); ok && strings.TrimSpace(legacySecret) != "" {
		cfg.Profiles["default"] = Profile{NotionSecret: strings.TrimSpace(legacySecret)}
		return cfg, nil
	}

	return nil, cliError{fmt.Sprintf("Invalid config format in %s. Expected top-level 'profiles' object.", path)}
}

func (c *Config) save() error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o644)
}

func (c *Config) secret(profile string) (string, error) {
	p, ok := c.Profiles[profile]
	if !ok {
		return "", cliError{fmt.Sprintf("Profile '%s' not configured. Run: notion configure -p %s", profile, profile)}
	}
	if strings.TrimSpace(p.NotionSecret) == "" {
		return "", cliError{fmt.Sprintf("Profile '%s' has no valid notion_secret. Re-run: notion configure -p %s", profile, profile)}
	}
	return strings.TrimSpace(p.NotionSecret), nil
}

func getSelectedProfile(rawProfile string) (string, error) {
	profile := strings.TrimSpace(rawProfile)
	if profile == "" {
		return "", cliError{"Profile cannot be empty."}
	}
	return profile, nil
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
	return cfg.secret(selectedProfile)
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

	existingSecret := strings.TrimSpace(cfg.Profiles[selectedProfile].NotionSecret)
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

	cfg.Profiles[selectedProfile] = Profile{NotionSecret: secret}
	if err := cfg.save(); err != nil {
		return err
	}
	fmt.Printf("Config saved to %s (profile: %s)\n", configPath(), selectedProfile)
	return nil
}
