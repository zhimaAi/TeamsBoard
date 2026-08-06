package skills

import (
	"database/sql"
	"fmt"
)

// Built-in Skill name constants
const (
	SkillAPI = "goteams-api"
	SkillDB  = "goteams-db"
)

// BuiltinSkill is built-in Skill metadata
// Description / Capabilities / Routes align with the actual Go native routes,
// and are merged and returned by tools.listSkills for frontend display.
type BuiltinSkill struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
	Routes       []string `json:"routes"`
	AlwaysOn     bool     `json:"always_on"`
}

// BuiltinSkills is the built-in Skill list (order is the display order)
var BuiltinSkills = []BuiltinSkill{
	{
		Name:        SkillDB,
		Description: "GoTeams 数据库工具 Skill，支持 MySQL 和 PostgreSQL 的表结构查看、数据查询和受控写入。",
		Capabilities: []string{
			"查询数据库表、表结构和表数据",
			"SQL 白名单校验（SELECT/INSERT/UPDATE，禁止 DROP/DELETE 等）",
			"执行只读查询并返回列与行",
			"受控写入（需 confirmed=true）",
		},
		Routes: []string{
			"POST /api/local/tools/db/validate",
			"POST /api/local/tools/db/execute",
		},
	},
	{
		Name:        SkillAPI,
		Description: "GoTeams 接口管理 Skill，提供 API 请求编辑和执行能力。",
		Capabilities: []string{
			"管理接口集合和目录",
			"编辑和保存 API 请求定义",
			"管理环境变量",
		},
		Routes: []string{
			"GET/POST/PUT/DELETE /api/local/apis/collections",
			"GET/POST/PUT/DELETE /api/local/apis/folders",
			"GET/POST/PUT/DELETE /api/local/apis/requests",
			"GET/POST/PUT/DELETE /api/local/apis/environments",
		},
	},
}

// builtinIndex maps built-in Skill names to metadata
var builtinIndex = func() map[string]BuiltinSkill {
	m := make(map[string]BuiltinSkill, len(BuiltinSkills))
	for _, s := range BuiltinSkills {
		m[s.Name] = s
	}
	return m
}()

// Lookup queries built-in Skill metadata, returning nil if not found
func Lookup(name string) *BuiltinSkill {
	if s, ok := builtinIndex[name]; ok {
		return &s
	}
	return nil
}

// BuiltinNames returns all built-in Skill names (order matches BuiltinSkills)
func BuiltinNames() []string {
	names := make([]string, len(BuiltinSkills))
	for i, s := range BuiltinSkills {
		names[i] = s.Name
	}
	return names
}

// Require checks whether the specified Skill is enabled.
// Queries gt_skill_settings.enabled; a missing record is treated as enabled (for backward compatibility, to avoid locking up when the seed has not run).
// The caller passes db (usually obtained from storage.DBRef.Get()).
func Require(db *sql.DB, name string) error {
	if db == nil {
		return fmt.Errorf("数据库未初始化")
	}
	var enabled int
	err := db.QueryRow(
		`SELECT enabled FROM gt_skill_settings WHERE skill_name = ?`, name,
	).Scan(&enabled)
	if err == sql.ErrNoRows {
		// Record missing: allow by default (seed not run, or custom scenario)
		return nil
	}
	if err != nil {
		return fmt.Errorf("校验 Skill %s 开关失败: %w", name, err)
	}
	if enabled != 1 {
		return fmt.Errorf("Skill %s 已被禁用", name)
	}
	return nil
}

// IsEnabled queries whether a Skill is enabled (missing record treated as enabled)
func IsEnabled(db *sql.DB, name string) bool {
	if db == nil {
		return false
	}
	var enabled int
	err := db.QueryRow(
		`SELECT enabled FROM gt_skill_settings WHERE skill_name = ?`, name,
	).Scan(&enabled)
	if err != nil {
		return true // On missing record or error, allow by default
	}
	return enabled == 1
}
