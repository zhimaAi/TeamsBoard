package executor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
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

// cliDefinition describes a probe target: how to locate its executable and how
// to resolve its available models on demand.
type cliDefinition struct {
	Type          string
	Name          string
	Executables   []string // List of executable file names (by priority)
	ModelResolver func(ctx context.Context, execPath string) []string
}

// cliDefinitions returns the CLI definitions to probe on this machine.
func cliDefinitions() []cliDefinition {
	return []cliDefinition{
		{
			Type:          CLITypeCodex,
			Name:          "Codex CLI",
			Executables:   []string{"codex"},
			ModelResolver: codexModelResolver,
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
		{
			Type:          CLITypeCursor,
			Name:          "Cursor Agent",
			Executables:   []string{"agent"},
			ModelResolver: resolveCursorModels,
		},
		{
			Type:          CLITypeCopilot,
			Name:          "GitHub Copilot CLI",
			Executables:   []string{"copilot"},
			ModelResolver: resolveCopilotModels,
		},
		{
			Type:          CLITypeGrok,
			Name:          "Grok CLI",
			Executables:   []string{"grok"},
			ModelResolver: resolveGrokModels,
		},
		// {
		// 	Type:          CLITypeHermes,
		// 	Name:          "Hermes",
		// 	Executables:   []string{"hermes"},
		// 	ModelResolver: resolveHermesModels,
		// },
		{
			Type:          CLITypeKimi,
			Name:          "Kimi Code",
			Executables:   []string{"kimi"},
			ModelResolver: resolveKimiModels,
		},
		{
			Type:          CLITypeQoder,
			Name:          "Qoder CLI",
			Executables:   []string{"qodercli"},
			ModelResolver: resolveQoderModels,
		},
		{
			Type:          CLITypeQoderCN,
			Name:          "Qoder CLI (CN)",
			Executables:   []string{"qoderclicn"},
			ModelResolver: resolveQoderModels,
		},
		{
			Type:          CLITypeQwen,
			Name:          "Qwen Code",
			Executables:   []string{"qwen"},
			ModelResolver: resolveQwenModels,
		},
		// {
		// 	Type:          CLITypeOpenClaw,
		// 	Name:          "OpenClaw",
		// 	Executables:   []string{"openclaw"},
		// 	ModelResolver: resolveOpenClawModels,
		// },
		{
			Type:          CLITypePi,
			Name:          "Pi Agent",
			Executables:   []string{"pi"},
			ModelResolver: resolvePiModels,
		},
	}
}

// DiscoverCLIs detects installed CLI tools on this machine. It reports install
// status and version only; model lists are resolved separately via
// DiscoverCLIModels (the background refresh in discovery_cache.go probes the
// models of every installed CLI and persists them to clis.json/models.json).
func DiscoverCLIs(ctx context.Context) []DiscoveredCLI {
	defs := cliDefinitions()
	results := make([]DiscoveredCLI, 0, len(defs))
	for _, def := range defs {
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

		results = append(results, item)
	}
	return results
}

// DiscoverCLIModels resolves the model list for a single CLI type, spawning that
// CLI only when needed. It returns a non-nil slice; callers should treat an
// empty slice as "no models available" (CLI not installed or unsupported).
func DiscoverCLIModels(ctx context.Context, cliType string) []string {
	for _, def := range cliDefinitions() {
		if def.Type != cliType {
			continue
		}
		execPath := lookupExecutable(def.Executables)
		if execPath == "" {
			return []string{}
		}
		if def.ModelResolver != nil {
			models := def.ModelResolver(ctx, execPath)
			if models != nil {
				return models
			}
		}
		return []string{}
	}
	return []string{}
}

// knownExecutableSearchDirs are extra directories to probe when PATH lookup fails.
// Some installers (e.g. Cursor Agent) add their bin dir only to the interactive
// shell's PATH, so a long-running backend process may not resolve the binary via
// LookPath even though it works in a user's terminal. We probe the well-known
// install location with platform executable extensions as a fallback.
var knownExecutableSearchDirs = func() []string {
	dirs := make([]string, 0, 8)
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		dirs = append(dirs, filepath.Join(la, "cursor-agent"))
	}
	if hd, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(hd, "AppData", "Local", "cursor-agent"))
	}
	for _, pf := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramW6432")} {
		if pf == "" {
			continue
		}
		dirs = append(dirs, filepath.Join(pf, "cursor-agent"))
	}
	return dirs
}()

// ResolveCLIExecutable locates the executable for the given CLI type.
// Unlike a bare PATH lookup it also probes well-known install directories and the
// platform's script extensions (e.g. %LOCALAPPDATA%\cursor-agent\agent.cmd), so
// CLIs installed without being added to PATH (such as the Cursor agent wrapper)
// still work. Returns a user-friendly error when the CLI cannot be found.
func ResolveCLIExecutable(cliType string) (string, error) {
	names := cliExecutableNames(cliType)
	if len(names) == 0 {
		return "", fmt.Errorf("CLI 类型无效: %q", cliType)
	}
	execPath := lookupExecutable(names)
	if execPath == "" {
		return "", fmt.Errorf("未找到 %s CLI，请先安装并加入 PATH", cliType)
	}
	return execPath, nil
}

// cliExecutableNames returns the candidate executable names for a CLI type
// (by priority).
func cliExecutableNames(cliType string) []string {
	switch cliType {
	case CLITypeCodex:
		return []string{"codex"}
	case CLITypeClaude:
		return []string{"claude"}
	case CLITypeCodeBuddy:
		return []string{"codebuddy", "cbc"}
	case CLITypeOpenCode:
		return []string{"opencode"}
	case CLITypeCursor:
		// The Cursor CLI binary is `agent`, not `cursor`.
		return []string{"agent"}
	case CLITypeCopilot:
		return []string{"copilot"}
	case CLITypeGrok:
		return []string{"grok"}
	case CLITypeKimi:
		return []string{"kimi"}
	case CLITypeQoder:
		return []string{"qodercli"}
	case CLITypeQoderCN:
		return []string{"qoderclicn"}
	case CLITypeQwen:
		return []string{"qwen"}
	case CLITypePi:
		return []string{"pi"}
	default:
		return nil
	}
}

// lookupExecutable finds the first resolvable executable among the candidate names.
// It first checks the current process PATH (which handles PATHEXT extensions on
// Windows), then probes the persisted PATH directories (registry on Windows) and
// known install directories (with .exe/.cmd/.bat variants) as a fallback. The
// persisted PATH matters for tools like kimi/hermes whose installers only append
// to the registry PATH: a long-running backend started before the install would
// otherwise miss them even though they resolve in an interactive shell.
func lookupExecutable(names []string) string {
	for _, name := range names {
		if path, err := osexec.LookPath(name); err == nil && path != "" {
			return path
		}
	}
	dirs := append(persistedPathDirs(), knownExecutableSearchDirs...)
	for _, dir := range dirs {
		for _, name := range names {
			for _, ext := range []string{"", ".exe", ".cmd", ".bat"} {
				candidate := filepath.Join(dir, name+ext)
				if isExecutableFile(candidate) {
					return candidate
				}
			}
		}
	}
	return ""
}

// isExecutableFile reports whether path exists and is a regular file.
func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
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

// resolveCursorModels obtains the list of available models through the `agent models`
// command. The output format is one entry per line: "<id> - <description>", e.g.
//
//	auto - Auto (current, default)
//	gpt-5.3-codex - Codex 5.3
//
// We extract the model id (the token before " - ") and ignore the header ("Available
// models") and the trailing "Tip: ..." hint line. When the command fails or returns
// nothing usable, we fall back to the built-in catalog so the picker never shows an
// empty list just because the CLI could not be queried.
func resolveCursorModels(ctx context.Context, execPath string) []string {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := osexec.CommandContext(ctx, execPath, "models")
	output, err := cmd.Output()
	if err != nil {
		// `agent models` can fail when the Cursor CLI is not logged in, is still
		// starting up, or the executable is a wrapper whose sub-process output is
		// delayed; keep a usable model list in those cases.
		return cursorStaticModels()
	}

	models := parseCursorModelOutput(string(output))
	if len(models) == 0 {
		return cursorStaticModels()
	}
	return models
}

// parseCursorModelOutput extracts model ids from `agent models` output. It accepts
// both the "<id> - <description>" form and bare token lines, and strips ANSI escape
// sequences (some Cursor CLI versions colorize the listing).
func parseCursorModelOutput(output string) []string {
	var models []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := stripANSI(strings.TrimSpace(scanner.Text()))
		// Skip blank lines, the header and the trailing tip line.
		if line == "" || strings.HasPrefix(line, "Available models") || strings.HasPrefix(line, "Tip:") {
			continue
		}
		// Preferred form: "<id> - <description>".
		if idx := strings.Index(line, " - "); idx > 0 {
			id := strings.TrimSpace(line[:idx])
			if id != "" && !strings.ContainsAny(id, " \t") {
				models = append(models, id)
				continue
			}
		}
		// Tolerant form: a bare token that contains no spaces (older Cursor
		// CLI versions may print just the ids).
		if !strings.ContainsAny(line, " \t") {
			models = append(models, line)
		}
	}
	return dedup(models)
}

// cursorStaticModels returns a small built-in catalog of common Cursor Agent
// models, used as a fallback when `agent models` cannot be queried. Users can
// always override with a manually entered model.
func cursorStaticModels() []string {
	return []string{
		"auto",
		"gpt-5.3-codex",
		"gpt-5.3-codex-high",
		"gpt-5.3-codex-xhigh",
		"gpt-5.2",
		"claude-sonnet-5-thinking-high",
		"claude-opus-5-thinking-high",
		"composer-2.5",
		"cursor-grok-4.5-high",
	}
}

// stripANSI removes ANSI escape sequences (e.g. color codes) from a string.
var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func stripANSI(s string) string {
	return ansiEscapeRe.ReplaceAllString(s, "")
}

// codexModelResolver is registered by the internal/executor/codex package via
// RegisterCodexModelResolver. It is resolved lazily to avoid an import cycle
// (the codex package imports executor, so executor must not import codex).
var codexModelResolver func(ctx context.Context, execPath string) []string

// RegisterCodexModelResolver lets the codex subpackage plug in its model
// resolution implementation (which queries the `codex app-server` model/list
// JSON-RPC endpoint and falls back to parsing ~/.codex/config.toml).
func RegisterCodexModelResolver(f func(ctx context.Context, execPath string) []string) {
	codexModelResolver = f
}

// claudeStaticModels returns the hardcoded default model catalog for the Claude CLI.
// These are always available as defaults even when the local config does not list
// any model or the claude binary cannot be queried.
func claudeStaticModels() []string {
	return []string{
		"claude-sonnet-5",
		"claude-sonnet-4-6",
		"claude-fable-5",
		"claude-opus-5",
		"claude-opus-4-8",
		"claude-opus-4-7",
		"claude-haiku-4-5-20251001",
		"claude-opus-4-6",
		"claude-sonnet-4-5",
	}
}

// claudeEnvModelVars are the environment variables that configure which model
// the Claude CLI uses; their values are treated as available models, mirroring
// the entries the CLI's own /model menu derives from them.
var claudeEnvModelVars = []string{
	"ANTHROPIC_MODEL",
	"ANTHROPIC_DEFAULT_OPUS_MODEL",
	"ANTHROPIC_DEFAULT_SONNET_MODEL",
	"ANTHROPIC_DEFAULT_HAIKU_MODEL",
	"CLAUDE_CODE_SUBAGENT_MODEL",
}

// claudeSettingsEnvModels reads the model-related env entries from the Claude
// user settings file (~/.claude/settings.json). The Claude CLI applies this env
// block at startup, so it is the authoritative source for the models the spawned
// CLI will actually use even when this process was launched without the user's
// shell environment (e.g. from the desktop host).
func claudeSettingsEnvModels(homeDir string) (customEndpoint bool, models []string) {
	data, err := os.ReadFile(filepath.Join(homeDir, ".claude", "settings.json"))
	if err != nil {
		return false, nil
	}

	var config map[string]interface{}
	if json.Unmarshal(data, &config) != nil {
		return false, nil
	}

	envRaw, ok := config["env"].(map[string]interface{})
	if !ok {
		return false, nil
	}

	if baseURL, ok := envRaw["ANTHROPIC_BASE_URL"].(string); ok && strings.TrimSpace(baseURL) != "" {
		customEndpoint = true
	}
	for _, key := range claudeEnvModelVars {
		if value, ok := envRaw[key].(string); ok {
			if value = strings.TrimSpace(value); value != "" {
				models = append(models, value)
			}
		}
	}
	return customEndpoint, models
}

// resolveClaudeModels reads models configured via environment variables and the
// claude configuration file. When talking to the official Anthropic endpoint
// (no ANTHROPIC_BASE_URL) the hardcoded default catalog is merged in so the CLI
// picker always has a usable list; a custom endpoint (e.g. a gateway or another
// provider's Anthropic-compatible API) only serves the explicitly configured
// models, because the hardcoded Anthropic catalog would be unusable there.
func resolveClaudeModels(_ context.Context, _ string) []string {
	var envModels []string
	for _, key := range claudeEnvModelVars {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			envModels = append(envModels, value)
		}
	}

	customEndpoint := strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL")) != ""

	var configModels []string

	homeDir, err := os.UserHomeDir()
	if err == nil {
		// The CLI applies ~/.claude/settings.json's env block at startup, so it
		// must drive the endpoint decision and model list even when this process
		// never inherited the user's shell environment.
		settingsCustom, settingsModels := claudeSettingsEnvModels(homeDir)
		if settingsCustom {
			customEndpoint = true
		}
		envModels = append(envModels, settingsModels...)

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

			if model, ok := config["model"].(string); ok && model != "" {
				configModels = append(configModels, model)
			}
			if model, ok := config["primaryModel"].(string); ok && model != "" {
				configModels = append(configModels, model)
			}
			if model, ok := config["smallModelOverride"].(string); ok && model != "" {
				configModels = append(configModels, model)
			}
		}
	}

	// Env-var models first (they override config), then config-detected models.
	merged := dedup(append(envModels, configModels...))
	if !customEndpoint {
		merged = append(merged, claudeStaticModels()...)
	}
	return dedup(merged)
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

// copilotModelSectionRe matches the "`model`:" config section header that lists
// every model id as a quoted bullet. The Copilot CLI does not expose a `models`
// subcommand, so the picker derives the catalog from this documented setting.
var copilotModelSectionRe = regexp.MustCompile("^\\s*`model`:")

// copilotModelLineRe matches a single model bullet under the model section,
// e.g. `    - "gpt-5.4"`.
var copilotModelLineRe = regexp.MustCompile(`^\s+-\s+"([^"]+)"\s*$`)

// copilotConfigSectionRe matches the start of any other config key
// (e.g. “  `contextTier`: “) that ends the model bullet block.
var copilotConfigSectionRe = regexp.MustCompile("^\\s*`[^`]+`:")

// resolveCopilotModels returns the supported model list for the GitHub Copilot
// CLI. Copilot has no `copilot models` subcommand, so the list is read from
// `copilot help config` (which documents every model id), merged with the
// "auto" pick and the built-in catalog so no CLI query failure strands the
// user with an empty picker.
func resolveCopilotModels(ctx context.Context, execPath string) []string {
	var models []string
	if parsed := parseCopilotModelsFromHelpConfig(ctx, execPath); len(parsed) > 0 {
		models = append(models, parsed...)
	}
	// "auto" lets Copilot pick the model automatically; always present.
	merged := append([]string{"auto"}, models...)
	merged = append(merged, copilotStaticModels()...)
	return dedup(merged)
}

// parseCopilotModelsFromHelpConfig extracts model ids from `copilot help config`
// output. Only the `model` section is parsed; the block ends at the next config
// key so unrelated quoted bullets (e.g. banner frequency) are not collected.
func parseCopilotModelsFromHelpConfig(ctx context.Context, execPath string) []string {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := osexec.CommandContext(ctx, execPath, "help", "config")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}
	return parseCopilotModelsFromText(string(output))
}

// parseCopilotModelsFromText extracts model ids from `copilot help config` text.
func parseCopilotModelsFromText(output string) []string {
	var models []string
	inModelSection := false
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if copilotModelSectionRe.MatchString(line) {
			inModelSection = true
			continue
		}
		if inModelSection && copilotConfigSectionRe.MatchString(line) {
			break
		}
		if inModelSection {
			if match := copilotModelLineRe.FindStringSubmatch(line); match != nil {
				models = append(models, match[1])
			}
		}
	}
	return models
}

// copilotStaticModels is a small built-in catalog of GitHub Copilot CLI model
// ids. It acts as a safety net so the model picker never shows an empty list
// when `copilot help config` cannot be queried (e.g. CLI not authenticated).
func copilotStaticModels() []string {
	return []string{
		"auto",
		"claude-sonnet-5",
		"claude-fable-5",
		"claude-opus-5",
		"claude-opus-4.8",
		"claude-opus-4.8-fast",
		"claude-opus-4.7",
		"claude-sonnet-4.6",
		"claude-opus-4.6",
		"claude-sonnet-4.5",
		"claude-opus-4.5",
		"claude-haiku-4.5",
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-5.6-luna",
		"gpt-5.5",
		"gpt-5.4",
		"gpt-5.4-mini",
		"gpt-5.3-codex",
		"gpt-5-mini",
		"mai-code-1-flash-picker",
		"gemini-3.6-flash",
		"gemini-3.5-flash",
		"gemini-3.1-pro-preview",
		"grok-4.5",
		"kimi-k3",
		"kimi-k2.7-code",
	}
}

// grokModelColorRe matches the id of a model entry line in `grok models` output.
// The CLI colorizes the id (e.g. "\x1b[36mgrok-4.3\x1b[0m ..."); description and
// alias continuation lines carry no ANSI escape, so extracting the colored token
// isolates exactly one id per entry.
var grokModelColorRe = regexp.MustCompile(`\x1b\[[0-9;]*m([^\x1b\s]+)\x1b\[0m`)

// parseGrokModelOutput extracts model ids from `grok models` output. Each entry
// line is "  \x1b[36m<id>\x1b[0m ..."; header/description/alias lines carry no
// SGR sequence and are skipped automatically.
func parseGrokModelOutput(output string) []string {
	var models []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if match := grokModelColorRe.FindStringSubmatch(line); match != nil {
			models = append(models, match[1])
		}
	}
	return dedup(models)
}

// resolveGrokModels obtains the available model list through the `grok models`
// command, falling back to the local settings files and a small built-in catalog
// so the picker never shows an empty list when the CLI cannot be queried.
func resolveGrokModels(ctx context.Context, execPath string) []string {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := osexec.CommandContext(ctx, execPath, "models")
	output, err := cmd.Output()
	if err == nil {
		if models := parseGrokModelOutput(string(output)); len(models) > 0 {
			return models
		}
	}

	// Fallback: models in ~/.grok settings files.
	var configured []string
	homeDir, err := os.UserHomeDir()
	if err == nil {
		configured = append(configured, grokConfiguredModels(
			filepath.Join(homeDir, ".grok", "settings.json"),
			filepath.Join(homeDir, ".grok", "user-settings.json"),
		)...)
	}
	merged := append(configured, grokStaticModels()...)
	return dedup(merged)
}

// grokConfiguredModels reads the "model"/"defaultModel" keys from the grok
// settings files to seed the picker even before a successful `grok models`.
func grokConfiguredModels(paths ...string) []string {
	var models []string
	for _, configPath := range paths {
		data, err := os.ReadFile(configPath)
		if err != nil {
			continue
		}
		var config map[string]interface{}
		if json.Unmarshal(data, &config) != nil {
			continue
		}
		for _, key := range []string{"model", "defaultModel", "primaryModel"} {
			if model, ok := config[key].(string); ok && strings.TrimSpace(model) != "" {
				models = append(models, strings.TrimSpace(model))
			}
		}
	}
	return dedup(models)
}

// grokStaticModels is a small built-in catalog of Grok CLI model ids, used as a
// safety net when neither `grok models` nor the settings files are available.
func grokStaticModels() []string {
	return []string{
		"grok-4.3",
		"grok-4.5",
		"grok-4.20-multi-agent",
		"grok-4.20-non-reasoning",
		"grok-3-mini",
		"grok-composer-2.5-fast",
	}
}

// resolveHermesModels obtains the available model list for the Hermes CLI.
// `hermes model` is an interactive TTY-gated wizard — piping it from a
// subprocess just exits with "requires an interactive terminal" — so the
// resolver reads the model data Hermes itself maintains on disk instead:
//
//   - $HERMES_HOME/provider_models_cache.json — the `hermes model` picker's
//     per-provider model cache (exactly what the interactive picker shows)
//   - $HERMES_HOME/models_dev_cache.json      — the models.dev catalog snapshot
//   - $HERMES_HOME/config.yaml                — configured provider + default
//     model, plus every custom provider and its curated model list
//
// The union is deduplicated and falls back to a small built-in catalog so the
// picker never shows an empty list when the local data files are missing.
func resolveHermesModels(_ context.Context, _ string) []string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return hermesStaticModels()
	}

	hermesHome := os.Getenv("HERMES_HOME")
	if strings.TrimSpace(hermesHome) == "" {
		hermesHome = filepath.Join(homeDir, ".hermes")
	}

	models := append([]string{},
		hermesModelsFromPickerCache(hermesHome)...,
	)
	models = append(models, hermesModelsFromModelsDev(hermesHome)...)
	models = append(models, hermesConfiguredModels(hermesHome)...)

	models = dedup(models)
	if len(models) == 0 {
		return hermesStaticModels()
	}
	return models
}

// hermesModelsFromPickerCache reads $HERMES_HOME/provider_models_cache.json,
// written by the `hermes model` picker for every provider it has queried:
//
//	{"<provider>": {"fp": "...", "at": ..., "models": ["id", ...]}, ...}
//
// The union of all model ids is exactly the set the interactive picker can show.
func hermesModelsFromPickerCache(hermesHome string) []string {
	data, err := os.ReadFile(filepath.Join(hermesHome, "provider_models_cache.json"))
	if err != nil {
		return nil
	}

	var cache map[string]struct {
		Models []string `json:"models"`
	}
	if json.Unmarshal(data, &cache) != nil {
		return nil
	}

	var models []string
	for _, entry := range cache {
		models = append(models, entry.Models...)
	}
	return models
}

// hermesModelsFromModelsDev reads $HERMES_HOME/models_dev_cache.json, the
// models.dev catalog snapshot of shape
// {"<provider>": {"models": {"<id>": {...}}, ...}}.
func hermesModelsFromModelsDev(hermesHome string) []string {
	data, err := os.ReadFile(filepath.Join(hermesHome, "models_dev_cache.json"))
	if err != nil {
		return nil
	}

	var cache map[string]struct {
		Models map[string]json.RawMessage `json:"models"`
	}
	if json.Unmarshal(data, &cache) != nil {
		return nil
	}

	var models []string
	for _, provider := range cache {
		for id := range provider.Models {
			models = append(models, id)
		}
	}
	return models
}

// hermesConfiguredModels reads $HERMES_HOME/config.yaml and collects the
// configured default model plus the curated models of every custom provider.
func hermesConfiguredModels(hermesHome string) []string {
	var models []string
	raw, err := os.ReadFile(filepath.Join(hermesHome, "config.yaml"))
	if err != nil {
		return models
	}

	var cfg struct {
		Model struct {
			Provider string `yaml:"provider"`
			Default  string `yaml:"default"`
		} `yaml:"model"`
		CustomProviders []struct {
			Models []string `yaml:"models"`
		} `yaml:"custom_providers"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return models
	}

	if m := strings.TrimSpace(cfg.Model.Default); m != "" {
		models = append(models, m)
	}
	for _, provider := range cfg.CustomProviders {
		models = append(models, provider.Models...)
	}
	return models
}

// hermesStaticModels is a small built-in catalog of common Hermes models
// (mirroring the Nous Portal / OpenRouter curated lists the `hermes model`
// picker shows), used as a safety net when no local hermes data is available.
func hermesStaticModels() []string {
	return []string{
		"anthropic/claude-fable-5",
		"anthropic/claude-sonnet-5",
		"anthropic/claude-opus-5",
		"anthropic/claude-opus-4.8",
		"anthropic/claude-haiku-4.5",
		"openai/gpt-5.6-sol",
		"openai/gpt-5.6-terra",
		"openai/gpt-5.6-luna",
		"openai/gpt-5.5",
		"openai/gpt-5.4-mini",
		"google/gemini-3.6-flash",
		"google/gemini-3.1-pro-preview",
		"x-ai/grok-4.6",
		"deepseek/deepseek-v4-pro",
		"deepseek/deepseek-v4-flash",
		"qwen/qwen3.8-max",
		"moonshotai/kimi-k3",
		"minimax/minimax-m3",
		"z-ai/glm-5.2",
		"xiaomi/mimo-v2.5-pro",
		"tencent/hy3",
		"stepfun/step-3.7-flash",
		"nvidia/nemotron-3-super-120b-a12b",
	}
}

// kimiModelsOutput holds the JSON shape of `kimi provider list --json`. The
// `models` map is keyed by the model alias users pass to `kimi -m <alias>`.
type kimiModelsOutput struct {
	Models map[string]json.RawMessage `json:"models"`
}

// resolveKimiModels obtains the available model aliases through the
// `kimi provider list --json` command. Kimi's provider config maps every
// configured model alias to its provider metadata; the alias keys are exactly
// what `kimi -m <alias>` accepts, so they become the picker's model list. When
// the command fails (e.g. not logged in yet) a small built-in catalog is used
// so the picker never shows an empty list.
func resolveKimiModels(ctx context.Context, execPath string) []string {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := osexec.CommandContext(ctx, execPath, "provider", "list", "--json")
	output, err := cmd.Output()
	if err == nil {
		var parsed kimiModelsOutput
		if json.Unmarshal(output, &parsed) == nil && len(parsed.Models) > 0 {
			models := make([]string, 0, len(parsed.Models))
			for alias := range parsed.Models {
				models = append(models, alias)
			}
			return dedup(models)
		}
	}

	return kimiStaticModels()
}

// kimiStaticModels is a small built-in catalog of common Kimi Code model
// aliases, used as a safety net when `kimi provider list` cannot be queried
// (e.g. the CLI is not logged in or has no provider configured).
func kimiStaticModels() []string {
	return []string{
		"kimi-k3",
		"kimi-k2.7-code",
	}
}

// qoderModelsOutput holds the tolerant JSON shapes of `qodercli --list-models`.
// The CLI emits the model catalog either as an object keyed by model id, as a
// "models" array, or as a top-level array of entries; each entry may be a bare
// string or an object carrying "value"/"id"/"name" fields (the SDK shape).
type qoderModelsOutput struct {
	Models []json.RawMessage `json:"models"`
	Value  string            `json:"value"`
	ID     string            `json:"id"`
	Name   string            `json:"name"`
}

// resolveQoderModels obtains the available model list through the
// `qodercli --list-models` command. The command lists every model the account
// can select and exits (headless mode). Its output shape has varied across
// releases (JSON catalog vs. plain text), so the parser accepts a JSON object
// keyed by model id, a JSON "models" array, a top-level JSON array, or
// line-oriented text (including the SDK "value<TAB>displayName" form and the
// "id - description" form). When the command cannot be queried a small
// built-in catalog is used so the picker never shows an empty list.
func resolveQoderModels(ctx context.Context, execPath string) []string {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := osexec.CommandContext(ctx, execPath, "--list-models")
	output, err := cmd.Output()
	if err != nil {
		return qoderStaticModels()
	}

	if models := parseQoderModelOutput(string(output)); len(models) > 0 {
		return models
	}
	return qoderStaticModels()
}

// parseQoderModelOutput extracts model ids from `qodercli --list-models` output,
// accepting the JSON and plain-text shapes described on resolveQoderModels.
func parseQoderModelOutput(output string) []string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil
	}

	// Tolerant JSON parsing: top-level array, object with a "models" array, or
	// an object keyed by model id (each key is an accepted model id).
	if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
		var models []string

		var rawArray []json.RawMessage
		if json.Unmarshal([]byte(trimmed), &rawArray) == nil {
			for _, item := range rawArray {
				models = append(models, qoderModelIDFromRaw(item)...)
			}
			return dedup(models)
		}

		var rawObject map[string]json.RawMessage
		if json.Unmarshal([]byte(trimmed), &rawObject) == nil {
			if entries, ok := rawObject["models"]; ok {
				if json.Unmarshal(entries, &rawArray) == nil {
					for _, item := range rawArray {
						models = append(models, qoderModelIDFromRaw(item)...)
					}
					return dedup(models)
				}
			}
			for key := range rawObject {
				if strings.TrimSpace(key) != "" {
					models = append(models, key)
				}
			}
			return dedup(models)
		}
	}

	// Plain-text fallback: one entry per line in "value<TAB>displayName",
	// "id - description" or bare-token form. `qodercli --list-models` emits a
	// table whose only header column is "MODEL", which is not a model id.
	var models []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := stripANSI(strings.TrimSpace(scanner.Text()))
		if line == "" || strings.EqualFold(line, "MODEL") {
			continue
		}
		switch {
		case strings.Contains(line, "\t"):
			models = append(models, strings.TrimSpace(strings.SplitN(line, "\t", 2)[0]))
		case strings.Contains(line, " - "):
			models = append(models, strings.TrimSpace(strings.SplitN(line, " - ", 2)[0]))
		default:
			models = append(models, line)
		}
	}
	return dedup(models)
}

// qoderModelIDFromRaw extracts the model id from one JSON entry of the
// `qodercli --list-models` catalog: either a bare string or an object with
// "value"/"id"/"name" fields.
func qoderModelIDFromRaw(raw json.RawMessage) []string {
	var id string
	if json.Unmarshal(raw, &id) == nil && strings.TrimSpace(id) != "" {
		return []string{strings.TrimSpace(id)}
	}

	var obj qoderModelsOutput
	if json.Unmarshal(raw, &obj) != nil {
		return nil
	}
	for _, candidate := range []string{obj.Value, obj.ID, obj.Name} {
		if strings.TrimSpace(candidate) != "" {
			return []string{strings.TrimSpace(candidate)}
		}
	}
	return nil
}

// qoderStaticModels is a small built-in catalog of Qoder CLI model tiers, used
// as a safety net when `qodercli --list-models` cannot be queried (e.g. the CLI
// is not logged in or offline).
func qoderStaticModels() []string {
	return []string{
		"auto",
		"lite",
		"performance",
		"ultimate",
	}
}

// qwenSettingsModels holds the model-related keys of Qwen Code's
// ~/.qwen/settings.json. `model.name` is the currently selected model, and each
// `modelProviders.<provider>` entry declares a selectable model via its `name`
// (with `id` as the fallback).
type qwenSettingsModels struct {
	Model struct {
		Name string `json:"name"`
	} `json:"model"`
	ModelProviders map[string][]struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"modelProviders"`
}

// resolveQwenModels reads the selectable model list from Qwen Code's local
// settings file (~/.qwen/settings.json), which is exactly the model set the CLI
// offers. The configured default model (model.name) comes first, followed by
// every model name declared under modelProviders (using `name`, falling back to
// `id`). When the settings file is missing or unreadable a small built-in
// catalog is used so the picker never shows an empty list.
func resolveQwenModels(_ context.Context, _ string) []string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return qwenStaticModels()
	}
	data, err := os.ReadFile(filepath.Join(homeDir, ".qwen", "settings.json"))
	if err != nil {
		return qwenStaticModels()
	}
	return qwenModelsFromData(data)
}

// qwenModelsFromData parses the JSON payload of ~/.qwen/settings.json and
// returns the selectable model list: the configured model.name first, then
// every model name declared under modelProviders (using `name`, falling back to
// `id`), deduplicated. Falls back to the static catalog when the data cannot be
// parsed or contains no models.
func qwenModelsFromData(data []byte) []string {
	var cfg qwenSettingsModels
	if json.Unmarshal(data, &cfg) != nil {
		return qwenStaticModels()
	}

	var models []string
	if m := strings.TrimSpace(cfg.Model.Name); m != "" {
		models = append(models, m)
	}
	for _, providers := range cfg.ModelProviders {
		for _, provider := range providers {
			if m := strings.TrimSpace(provider.Name); m != "" {
				models = append(models, m)
				continue
			}
			if m := strings.TrimSpace(provider.ID); m != "" {
				models = append(models, m)
			}
		}
	}

	models = dedup(models)
	if len(models) == 0 {
		return qwenStaticModels()
	}
	return models
}

// qwenStaticModels is a small built-in catalog of common Qwen Code models, used
// as a safety net when ~/.qwen/settings.json cannot be read.
func qwenStaticModels() []string {
	return []string{
		"qwen3-coder-plus",
		"qwen3-coder",
		"qwen3-coder-flash",
	}
}

// resolveOpenClawModels obtains the list of selectable model refs through the
// `openclaw models list` command. The command is read-only, uses the configured
// default agent, and returns every available model as a `provider/model` ref
// (the exact form `openclaw agent exec --model` accepts). `--json` reserves
// stdout for a single JSON document, so the parser only needs to tolerate the
// envelope shapes listed on parseOpenClawModelsOutput. When the command cannot
// be queried a small built-in catalog keeps the picker non-empty.
func resolveOpenClawModels(ctx context.Context, execPath string) []string {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := osexec.CommandContext(ctx, execPath, "models", "list", "--json")
	output, err := cmd.Output()
	if err != nil {
		return openclawStaticModels()
	}

	if models := parseOpenClawModelsOutput(string(output)); len(models) > 0 {
		return models
	}
	return openclawStaticModels()
}

// parseOpenClawModelsOutput extracts model refs from `openclaw models list
// --json` output. The JSON envelope is a document whose rows carry
// `provider/model` refs; across releases it has appeared as a top-level array,
// as a wrapper key ("models"/"rows"/"entries"/"items"/"data"), or keyed by
// model ref directly. Each entry may be a bare ref string or an object carrying
// a ref-like field. A line-oriented fallback covers `--plain`-style output
// (one ref per line). Display names (multi-word) and header cells are dropped.
func parseOpenClawModelsOutput(output string) []string {
	trimmed := strings.TrimSpace(stripANSI(output))
	if trimmed == "" {
		return nil
	}

	if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
		var models []string

		var rawArray []json.RawMessage
		if json.Unmarshal([]byte(trimmed), &rawArray) == nil {
			for _, item := range rawArray {
				models = append(models, openClawModelRefFromRaw(item)...)
			}
			return dedup(models)
		}

		var rawObject map[string]json.RawMessage
		if json.Unmarshal([]byte(trimmed), &rawObject) == nil {
			for _, wrapper := range []string{"models", "rows", "entries", "items", "data", "list"} {
				entries, ok := rawObject[wrapper]
				if !ok {
					continue
				}
				if json.Unmarshal(entries, &rawArray) == nil {
					for _, item := range rawArray {
						models = append(models, openClawModelRefFromRaw(item)...)
					}
					return dedup(models)
				}
			}
			for key := range rawObject {
				if strings.TrimSpace(key) != "" {
					models = append(models, key)
				}
			}
			return dedup(models)
		}
	}

	// Plain-text fallback: one ref per line, possibly "ref\tdisplayName".
	var models []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := stripANSI(strings.TrimSpace(scanner.Text()))
		if line == "" {
			continue
		}
		candidate := line
		if i := strings.IndexAny(line, "\t "); i > 0 {
			candidate = line[:i]
		}
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || strings.ContainsAny(candidate, " \t") || openClawIsHeader(candidate) {
			continue
		}
		models = append(models, candidate)
	}
	return dedup(models)
}

// openClawModelRefFromRaw extracts a model ref from one JSON entry of the
// `openclaw models list --json` catalog: a bare string, or an object carrying
// the `key` field (the `provider/model` ref, e.g. "openai/gpt-5.6-sol").
// Other ref-like fields are tried as fallbacks for coverage across releases.
// The display `name` is only used when it is a single token (a display name
// such as "Claude Opus 4.6 (Claude CLI)" contains whitespace and is dropped).
func openClawModelRefFromRaw(raw json.RawMessage) []string {
	var ref string
	if json.Unmarshal(raw, &ref) == nil {
		ref = strings.TrimSpace(ref)
		if ref != "" {
			return []string{ref}
		}
	}

	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return nil
	}
	for _, field := range []string{"key", "ref", "model", "id", "value", "name"} {
		var value string
		if json.Unmarshal(obj[field], &value) != nil {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if field == "name" && strings.ContainsAny(value, " \t") {
			continue
		}
		return []string{value}
	}
	return nil
}

// openClawIsHeader reports whether a plain-text cell looks like a table header
// rather than a model ref.
func openClawIsHeader(candidate string) bool {
	switch strings.ToUpper(candidate) {
	case "MODEL", "NAME", "PROVIDER", "STATUS", "AUTH", "CTX", "INPUT",
		"OUTPUT", "COST", "ALIASES", "ACTIVE", "PRIMARY", "FALLBACK":
		return true
	}
	return false
}

// openclawStaticModels is a small built-in catalog of common OpenClaw model
// refs, used as a safety net when `openclaw models list` cannot be queried
// (e.g. the CLI is not logged in or offline).
func openclawStaticModels() []string {
	return []string{
		"anthropic/claude-sonnet-4-6",
		"openai/gpt-5.6-sol",
		"google/gemini-3.1-pro-preview",
	}
}

// resolvePiModels obtains the available model list through the
// `pi --list-models` command. The output is a table with columns:
//
//	provider  model  context  max-out  thinking  images
//
// We extract the "provider/model" pair from each data row. When the command
// cannot be queried a small built-in catalog is used as fallback.
func resolvePiModels(ctx context.Context, execPath string) []string {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := osexec.CommandContext(ctx, execPath, "--list-models")
	output, err := cmd.Output()
	if err == nil {
		if models := parsePiModelOutput(string(output)); len(models) > 0 {
			return models
		}
	}
	return piStaticModels()
}

// parsePiModelOutput extracts "provider/model" pairs from `pi --list-models` table output.
// The table uses 2+ spaces as column separators. The header row is skipped.
func parsePiModelOutput(output string) []string {
	var models []string
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Skip header row
		if strings.HasPrefix(strings.TrimSpace(line), "provider") {
			continue
		}
		// Split on 2+ spaces to get table columns
		cols := regexp.MustCompile(`\s{2,}`).Split(line, -1)
		if len(cols) < 2 {
			continue
		}
		provider := strings.TrimSpace(cols[0])
		model := strings.TrimSpace(cols[1])
		if provider == "" || model == "" {
			continue
		}
		models = append(models, provider+"/"+model)
	}
	return dedup(models)
}

// piStaticModels is a small built-in catalog of common Pi Agent model tiers,
// used as a safety net when `pi --list-models` cannot be queried.
func piStaticModels() []string {
	return []string{
		"auto",
		"deepseek/deepseek-v4-flash",
		"deepseek/deepseek-v4-pro",
		"opencode/gpt-5.6-terra",
		"opencode/gpt-5.6-sol",
		"opencode/claude-sonnet-5",
		"opencode/claude-opus-5",
		"pixel/gpt-5.6-terra",
		"pixel/gpt-5.6-sol",
	}
}
