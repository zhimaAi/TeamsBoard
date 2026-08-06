package workflow

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"goteams-client/internal/skills"
)

const skillContextMarker = "[GoTeams Skill 运行上下文]"
const workDirContextMarker = "[GoTeams 工作目录上下文]"

// SkillPromptContext is the local Skill context injected into the prompt when creating a task.
type SkillPromptContext struct {
	APIBaseURL        string
	SkillsRoot        string
	APICollectionID   int64
	APICollectionName string
	APIFolderID       int64
	APIFolderName     string
	// DBProfiles are the task's selected database configs (prompt snapshot injected only when goteams-db is referenced)
	DBProfiles []DBProfileRef
}

// DBProfileRef is a summary of the task's selected database config, used to limit the accessible database_profile_id range in the prompt.
type DBProfileRef struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	DBType       string `json:"db_type"`
	DatabaseName string `json:"database_name"`
}

// BuiltinPlaceholderKey is the fixed unique key for a built-in placeholder (a stable contract shared by client and cloud).
// The placeholder text (token) is sent by the cloud in the snapshot; the client locates the local value logic by key alone,
// so changing the token text only requires a cloud change, no client release.
const (
	BuiltinKeyRequirement = "requirement_content" // Requirement document (default token: requirement content)
	BuiltinKeyAPI         = "api_interface"       // API Skill (default token: API interface)
	BuiltinKeyDB          = "db_operation"        // Database Skill (default token: Db operation)
)

// builtinResolvers maps built-in placeholder keys to client-side local value functions.
// Only this table is bound to keys; the token text is entirely cloud-controlled.
var builtinResolvers = map[string]func(ctx *PlaceholderContext) string{
	BuiltinKeyRequirement: func(c *PlaceholderContext) string { return c.RequirementDocPath },
	BuiltinKeyAPI:         func(c *PlaceholderContext) string { return c.SkillsAPIPath },
	BuiltinKeyDB:          func(c *PlaceholderContext) string { return c.SkillsDBPath },
}

// PlaceholderContext is the static placeholder context (resolved when creating a task)
type PlaceholderContext struct {
	// Document-related
	RequirementDocPath string            // Requirement document absolute path ({requirement content})
	DocumentPaths      map[string]string // Custom document name to knowledge-base absolute path
	SkillsAPIPath      string            // goteams-api Skill absolute path ({API interface})
	SkillsDBPath       string            // goteams-db Skill absolute path ({Db operation})
	// Built-in placeholder registry (sent by cloud: key -> token internal name)
	BuiltinPlaceholders map[string]string
}

// RuntimeContext is the runtime placeholder context (resolved when starting a Session)
type RuntimeContext struct {
	APIBaseURL      string            // Local loopback API address
	CapabilityToken string            // Session-level capability token (replaced with a marker after desensitization)
	SkillPaths      map[string]string // Skill name to local absolute path
}

// runtimePlaceholders is the runtime placeholder map
var runtimePlaceholders = []struct {
	placeholder string
	getValue    func(ctx *RuntimeContext) string
}{
	{"{接口开发API的token}", func(c *RuntimeContext) string {
		if c.CapabilityToken != "" {
			return "<GOTEAMS_LOCAL_CAPABILITY_TOKEN>"
		}
		return ""
	}},
	{"{接口开发API地址}", func(c *RuntimeContext) string { return c.APIBaseURL }},
}

// ResolveStatic resolves static placeholders (called when creating a task)
func ResolveStatic(template string, ctx PlaceholderContext) string {
	result := template
	// Built-in placeholders: resolved via the cloud-sent key->token registry; token text is cloud-controlled
	for key, token := range ctx.BuiltinPlaceholders {
		resolver, ok := builtinResolvers[key]
		if !ok || token == "" {
			continue
		}
		value := resolver(&ctx)
		if value == "" {
			continue
		}
		placeholder := "{" + strings.Trim(token, "{}") + "}"
		result = strings.ReplaceAll(result, placeholder, value)
	}
	for name, path := range ctx.DocumentPaths {
		if path != "" {
			result = strings.ReplaceAll(result, "{"+name+"}", path)
		}
	}
	return result
}

// ResolveRuntime resolves runtime placeholders (called when starting a Session)
func ResolveRuntime(template string, ctx RuntimeContext) string {
	result := template
	for _, ph := range runtimePlaceholders {
		value := ph.getValue(&ctx)
		if value != "" {
			result = strings.ReplaceAll(result, ph.placeholder, value)
		}
	}
	for name, path := range ctx.SkillPaths {
		if path != "" {
			result = strings.ReplaceAll(result, "{"+name+"地址}", path)
		}
	}
	return result
}

// BuiltinSkillPaths returns the deterministic local paths of all built-in Skills.
func BuiltinSkillPaths(skillsRoot string) map[string]string {
	paths := make(map[string]string, len(skills.BuiltinSkills))
	for _, skill := range skills.BuiltinSkills {
		paths[skill.Name] = filepath.Join(skillsRoot, skill.Name)
	}
	return paths
}

// ReferencedOperationalSkills returns the callable built-in Skills actually referenced by the prompt.
func ReferencedOperationalSkills(prompt string) []string {
	lower := strings.ToLower(prompt)
	result := make([]string, 0, 2)
	if containsAny(lower, "goteams-api", "{api接口}", "{goteams-api地址}") {
		result = append(result, skills.SkillAPI)
	}
	if containsAny(lower, "goteams-db", "{db操作}", "{goteams-db地址}") {
		result = append(result, skills.SkillDB)
	}
	return result
}

// InjectSkillPromptContext writes the request address, Skill absolute paths, and task scope into the prompt snapshot when creating a task.
// Authorized database_profile_id values are written compactly into the goteams-db section of the run context.
func InjectSkillPromptContext(resolvedPrompt, sourcePrompt string, ctx SkillPromptContext) string {
	if strings.Contains(resolvedPrompt, skillContextMarker) {
		return resolvedPrompt
	}
	referenced := ReferencedOperationalSkills(sourcePrompt)
	if len(referenced) == 0 {
		return resolvedPrompt
	}

	baseURL := strings.TrimRight(strings.TrimSpace(ctx.APIBaseURL), "/")
	if baseURL == "" {
		baseURL = "{接口开发API地址}"
	}
	lines := []string{
		skillContextMarker,
		"本地请求地址: " + baseURL,
	}
	for _, name := range referenced {
		skillPath := filepath.Join(ctx.SkillsRoot, name)
		if absolute, err := filepath.Abs(skillPath); err == nil {
			skillPath = absolute
		}
		lines = append(lines, "skills位置: "+skillPath)
		if name == skills.SkillAPI {
			lines = append(lines,
				"接口集合: "+ctx.APICollectionName+" (ID="+strconv.FormatInt(ctx.APICollectionID, 10)+")",
				"接口文件夹: "+ctx.APIFolderName+" (ID="+strconv.FormatInt(ctx.APIFolderID, 10)+")",
				"只能列出、查看、创建、修改或删除该文件夹内的接口，禁止操作集合或其他文件夹中的接口。",
			)
		}
		if name == skills.SkillDB {
			if len(ctx.DBProfiles) > 0 {
				for _, profile := range ctx.DBProfiles {
					dbType := profile.DBType
					if dbType == "" {
						dbType = "unknown"
					}
					dbName := profile.DatabaseName
					if dbName == "" {
						dbName = "-"
					}
					lines = append(lines, fmt.Sprintf(
						"database=%s，database_profile_id: %d, db_type=%s",
						dbName, profile.ID, dbType,
					))
				}
			}
		}
	}
	return strings.Join(lines, "\n") + "\n\n" + resolvedPrompt
}

// InjectWorkDirPromptContext writes the task's primary and related directories into each step's prompt snapshot.
// The CLI process uses the first directory as its current directory; the rest are used via absolute paths for cross-repo tasks.
func InjectWorkDirPromptContext(prompt string, workDirs []string) string {
	if strings.Contains(prompt, workDirContextMarker) {
		return prompt
	}

	normalized := make([]string, 0, len(workDirs))
	seen := make(map[string]struct{}, len(workDirs))
	for _, workDir := range workDirs {
		workDir = strings.TrimSpace(workDir)
		if workDir == "" {
			continue
		}
		if _, exists := seen[workDir]; exists {
			continue
		}
		seen[workDir] = struct{}{}
		normalized = append(normalized, workDir)
	}
	if len(normalized) == 0 {
		return prompt
	}

	lines := []string{
		workDirContextMarker,
		"以下目录均属于本任务；CLI 当前目录为主目录，涉及其他代码库时请使用对应绝对路径。",
		"主目录（CLI 当前目录）: " + normalized[0],
	}
	for index, workDir := range normalized[1:] {
		lines = append(lines, fmt.Sprintf("关联目录 %d: %s", index+1, workDir))
	}
	return strings.Join(lines, "\n") + "\n\n" + prompt
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
