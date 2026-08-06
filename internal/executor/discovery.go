package executor

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// DiscoveredCLI CLI tool information discovered by this machine
type DiscoveredCLI struct {
	Type      string   `json:"type"`      // codex/claude/codebuddy/opencode
	Name      string   `json:"name"`      // display name
	Installed bool     `json:"installed"` // Is it installed?
	ExecPath  string   `json:"exec_path"` //Executable file path
	Version   string   `json:"version"`   // version number
	Models    []string `json:"models"`    // List of available/configured models
}

// cliDefinition CLI definition
type cliDefinition struct {
	Type          string
	Name          string
	Executables   []string // List of executable file names (by priority)
	ModelResolver func(ctx context.Context, execPath string) []string
}

// DiscoverCLIs detects installed CLI tools and their model configurations on this machine
func DiscoverCLIs(ctx context.Context) []DiscoveredCLI {
	definitions := []cliDefinition{
		{
			Type:          CLITypeCodex,
			Name:          "Codex CLI",
			Executables:   []string{"codex"},
			ModelResolver: resolveCodexModels,
		},
		{
			Type:          CLITypeClaude,
			Name:          "Claude Code",
			Executables:   []string{"claude"},
			ModelResolver: resolveClaudeModels,
		},
		{
			Type:          CLITypeCodeBuddy,
			Name:          "CodeBuddy Code",
			Executables:   []string{"codebuddy", "cbc"},
			ModelResolver: resolveCodeBuddyModels,
		},
		{
			Type:          CLITypeOpenCode,
			Name:          "OpenCode",
			Executables:   []string{"opencode"},
			ModelResolver: resolveOpenCodeModels,
		},
	}

	results := make([]DiscoveredCLI, 0, len(definitions))
	for _, def := range definitions {
		item := DiscoveredCLI{
			Type:   def.Type,
			Name:   def.Name,
			Models: []string{},
		}

		execPath := lookupExecutable(def.Executables)
		if execPath == "" {
			results = append(results, item)
			continue
		}

		item.Installed = true
		item.ExecPath = execPath
		item.Version = detectVersion(ctx, execPath)

		if def.ModelResolver != nil {
			models := def.ModelResolver(ctx, execPath)
			if models != nil {
				item.Models = models
			}
		}

		results = append(results, item)
	}
	return results
}

// lookupExecutable Find executable files in PATH
func lookupExecutable(names []string) string {
	for _, name := range names {
		if path, err := osexec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

// detectVersion gets the CLI version number
func detectVersion(ctx context.Context, execPath string) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := osexec.CommandContext(ctx, execPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	version := strings.TrimSpace(string(output))
	// only take the first row
	if idx := strings.IndexByte(version, '\n'); idx > 0 {
		version = version[:idx]
	}
	return version
}

// resolveOpenCodeModels obtains the list of available models through the opencode models command
func resolveOpenCodeModels(ctx context.Context, execPath string) []string {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := osexec.CommandContext(ctx, execPath, "models")
	output, err := cmd.Output()
	if err != nil {
		return []string{}
	}

	var models []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// The output format of opencode models is provider/model
		if strings.Contains(line, "/") {
			models = append(models, line)
		}
	}
	return models
}

// resolveCodexModels reads the configured model from ~/.codex/config.toml
func resolveCodexModels(_ context.Context, _ string) []string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return []string{}
	}

	configPath := filepath.Join(homeDir, ".codex", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return []string{}
	}

	var models []string
	// Match model = "xxx" or model = 'xxx'
	re := regexp.MustCompile(`(?m)^\s*model\s*=\s*["']([^"']+)["']`)
	matches := re.FindAllStringSubmatch(string(data), -1)
	for _, match := range matches {
		if len(match) > 1 && match[1] != "" {
			models = append(models, match[1])
		}
	}

	// Extract from [profiles.xxx] section
	profileRe := regexp.MustCompile(`(?m)^\s*\[profiles\.([^\]]+)\]`)
	profileMatches := profileRe.FindAllStringSubmatch(string(data), -1)
	for _, match := range profileMatches {
		if len(match) > 1 && match[1] != "" {
			models = append(models, match[1])
		}
	}

	return dedup(models)
}

// resolveClaudeModels reads the configured model from the claude configuration file
func resolveClaudeModels(_ context.Context, _ string) []string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return []string{}
	}

	candidates := []string{
		filepath.Join(homeDir, ".claude", "settings.json"),
		filepath.Join(homeDir, ".claude.json"),
	}

	for _, configPath := range candidates {
		data, err := os.ReadFile(configPath)
		if err != nil {
			continue
		}

		var config map[string]interface{}
		if json.Unmarshal(data, &config) != nil {
			continue
		}

		var models []string
		if model, ok := config["model"].(string); ok && model != "" {
			models = append(models, model)
		}
		if model, ok := config["primaryModel"].(string); ok && model != "" {
			models = append(models, model)
		}
		if model, ok := config["smallModelOverride"].(string); ok && model != "" {
			models = append(models, model)
		}
		if len(models) > 0 {
			return dedup(models)
		}
	}

	return []string{}
}

// resolveCodeBuddyModels obtains the supported model list by parsing codebuddy --help output, and falls back to the configuration file if it fails.
func resolveCodeBuddyModels(ctx context.Context, execPath string) []string {
	// Prefer parsing the "Currently supported: (...)" model list from the --help output
	if models := resolveModelsFromHelp(ctx, execPath); len(models) > 0 {
		return models
	}

	// Fallback: read from configuration file
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return []string{}
	}

	candidates := []string{
		filepath.Join(homeDir, ".codebuddy", "settings.json"),
		filepath.Join(homeDir, ".codebuddy", "config.json"),
		filepath.Join(homeDir, ".cbc", "settings.json"),
	}

	for _, configPath := range candidates {
		data, err := os.ReadFile(configPath)
		if err != nil {
			continue
		}

		var config map[string]interface{}
		if json.Unmarshal(data, &config) != nil {
			continue
		}

		var models []string
		if model, ok := config["model"].(string); ok && model != "" {
			models = append(models, model)
		}
		if model, ok := config["primaryModel"].(string); ok && model != "" {
			models = append(models, model)
		}
		if len(models) > 0 {
			return dedup(models)
		}
	}

	return []string{}
}

// resolveModelsFromHelp extracts the list of models by parsing "Currently supported: (...)" in the CLI --help output.
// Suitable for CLIs such as CodeBuddy that embed the model list described by the --model option.
func resolveModelsFromHelp(ctx context.Context, execPath string) []string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := osexec.CommandContext(ctx, execPath, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}

	// Match "Currently supported: (model1, model2, ...)" pattern
	re := regexp.MustCompile(`Currently supported:\s*\(([^)]+)\)`)
	match := re.FindStringSubmatch(string(output))
	if len(match) < 2 {
		return nil
	}

	parts := strings.Split(match[1], ",")
	models := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			models = append(models, p)
		}
	}
	return models
}

// dedup removes duplicates
func dedup(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
