package tools

import (
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"

	"goteams-client/internal/dbconn"
	"goteams-client/internal/secrets"
	"goteams-client/internal/skills"
	"goteams-client/internal/storage"
)

// Handler is the tool-center handler
type Handler struct {
	dbRef *storage.DBRef
	store secrets.Store
}

// NewHandler creates the tool-center handler
func NewHandler(dbRef *storage.DBRef, store secrets.Store) *Handler {
	return &Handler{dbRef: dbRef, store: store}
}

// RegisterRoutes registers the tool-center routes
func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	// Skill switch management
	r.GET("/skills", h.listSkills)
	r.POST("/skills", h.createSkill)
	r.PUT("/skills/:id", h.updateSkill)
	r.DELETE("/skills/:id", h.deleteSkill)

	// Database tools
	r.POST("/db/execute", h.executeSQL)
	r.POST("/db/validate", h.validateSQL)

	// Left navigation menu switch configuration
	h.RegisterMenuRoutes(r)
}

// ========== Skill management ==========

// SkillItem is a Skill list entry (merged with built-in metadata)
type SkillItem struct {
	ID           int64    `json:"id"`
	SkillName    string   `json:"skill_name"`
	Enabled      bool     `json:"enabled"`
	ConfigJSON   string   `json:"config_json"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
	Routes       []string `json:"routes"`
	AlwaysOn     bool     `json:"always_on"`
	Builtin      bool     `json:"builtin"`
	CreatedAt    int64    `json:"created_at"`
	UpdatedAt    int64    `json:"updated_at"`
}

func (h *Handler) listSkills(c *gin.Context) {
	rows, err := h.dbRef.Get().Query(`SELECT id, skill_name, enabled, config_json, created_at, updated_at FROM gt_skill_settings ORDER BY id`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	dbItems := make(map[string]SkillItem, 8)
	for rows.Next() {
		var item SkillItem
		var enabled int
		if err := rows.Scan(&item.ID, &item.SkillName, &enabled, &item.ConfigJSON, &item.CreatedAt, &item.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		item.Enabled = enabled == 1
		if meta := skills.Lookup(item.SkillName); meta != nil {
			item.Description = meta.Description
			item.Capabilities = meta.Capabilities
			item.Routes = meta.Routes
			item.AlwaysOn = meta.AlwaysOn
			item.Builtin = true
		}
		dbItems[item.SkillName] = item
	}

	// Merge: output in built-in order, backfilling entries not registered in the DB
	var items []SkillItem
	for _, meta := range skills.BuiltinSkills {
		if item, ok := dbItems[meta.Name]; ok {
			items = append(items, item)
		} else {
			items = append(items, SkillItem{
				SkillName:    meta.Name,
				Enabled:      true,
				ConfigJSON:   "{}",
				Description:  meta.Description,
				Capabilities: meta.Capabilities,
				Routes:       meta.Routes,
				AlwaysOn:     meta.AlwaysOn,
				Builtin:      true,
			})
		}
	}
	// Append non-built-in custom records
	for name, item := range dbItems {
		if skills.Lookup(name) == nil {
			items = append(items, item)
		}
	}

	if items == nil {
		items = []SkillItem{}
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handler) createSkill(c *gin.Context) {
	var body struct {
		SkillName string                 `json:"skill_name"`
		Enabled   bool                   `json:"enabled"`
		Config    map[string]interface{} `json:"config"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.SkillName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "skill_name 不能为空"})
		return
	}

	enabledVal := 0
	if body.Enabled {
		enabledVal = 1
	}
	configJSON := "{}"
	if body.Config != nil {
		configJSON = marshalJSON(body.Config)
	}

	result, err := h.dbRef.Get().Exec(
		`INSERT INTO gt_skill_settings (skill_name, enabled, config_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		body.SkillName, enabledVal, configJSON, now(), now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取设置 ID 失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *Handler) updateSkill(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		Enabled *bool                  `json:"enabled"`
		Config  map[string]interface{} `json:"config"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var fields []string
	var values []interface{}
	if body.Enabled != nil {
		enabledVal := 0
		if *body.Enabled {
			enabledVal = 1
		}
		fields = append(fields, "enabled")
		values = append(values, enabledVal)
	}
	if body.Config != nil {
		fields = append(fields, "config_json")
		values = append(values, marshalJSON(body.Config))
	}
	if len(fields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有要更新的字段"})
		return
	}
	fields = append(fields, "updated_at")
	values = append(values, now())
	values = append(values, id)

	query := fmt.Sprintf("UPDATE gt_skill_settings SET %s WHERE id = ?", strings.Join(formatSetClause(fields), ", "))
	_, err := h.dbRef.Get().Exec(query, values...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) deleteSkill(c *gin.Context) {
	id := c.Param("id")
	_, err := h.dbRef.Get().Exec("DELETE FROM gt_skill_settings WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ========== SQL whitelist validation ==========

// SQL operation types
const (
	SQLOperationSelect  = "SELECT"
	SQLOperationInsert  = "INSERT"
	SQLOperationUpdate  = "UPDATE"
	SQLOperationDelete  = "DELETE"
	SQLOperationExplain = "EXPLAIN"
	SQLOperationShow    = "SHOW"
	SQLOperationDesc    = "DESC"
)

// Read-only operation types (only these are allowed for goteams-db)
var readOnlyOperations = []string{
	SQLOperationExplain,
	SQLOperationShow,
	SQLOperationDesc,
	SQLOperationSelect,
}

// ValidateResult is the SQL validation result
type ValidateResult struct {
	Valid        bool   `json:"valid"`
	Operation    string `json:"operation"`
	NeedsConfirm bool   `json:"needs_confirm"`
	Message      string `json:"message"`
}

// validateSQL validates a SQL statement
// goteams-db is a read-only skill, allowing only SELECT/EXPLAIN/SHOW/DESC and forbidding any write or DDL operation.
func validateSQL(sqlText string) ValidateResult {
	sqlText = strings.TrimSpace(sqlText)
	if sqlText == "" {
		return ValidateResult{Valid: false, Message: "SQL 不能为空"}
	}

	upper := strings.ToUpper(sqlText)

	// Forbid dangerous operations (write / DDL / privilege / transaction control, etc.)
	if dangerous := dangerousSQLPattern.FindString(upper); dangerous != "" {
		return ValidateResult{Valid: false, Operation: "", Message: fmt.Sprintf("禁止 %s 操作", dangerous)}
	}

	// Extract the first operation type
	var operation string
	for _, op := range readOnlyOperations {
		if strings.HasPrefix(upper, op) {
			operation = op
			break
		}
	}

	if operation == "" {
		return ValidateResult{Valid: false, Message: "不支持的 SQL 操作：goteams-db 仅允许只读查询（SELECT/EXPLAIN/SHOW/DESC）"}
	}

	// Read-only operation, allow directly
	return ValidateResult{Valid: true, Operation: operation, NeedsConfirm: false, Message: "只读操作"}
}

func (h *Handler) validateSQL(c *gin.Context) {
	var body struct {
		SQL string `json:"sql"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Validate the goteams-db Skill switch
	if err := skills.Require(h.dbRef.Get(), skills.SkillDB); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	result := validateSQL(body.SQL)
	c.JSON(http.StatusOK, result)
}

// ========== Database query execution ==========

// lookupSecret retrieves the password from the SecretStore.
func (h *Handler) lookupSecret(ref string) string {
	if ref == "" || h.store == nil {
		return ""
	}
	if pwd, err := h.store.Get(ref); err == nil {
		return pwd
	}
	return ""
}

func (h *Handler) executeSQL(c *gin.Context) {
	var body struct {
		DatabaseProfileID int64  `json:"database_profile_id"`
		SQL               string `json:"sql"`
		Confirmed         bool   `json:"confirmed"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.DatabaseProfileID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "database_profile_id 不能为空"})
		return
	}

	// Validate the goteams-db Skill switch
	if err := skills.Require(h.dbRef.Get(), skills.SkillDB); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	// Validate SQL (goteams-db is read-only, only SELECT/EXPLAIN/SHOW/DESC allowed)
	validation := validateSQL(body.SQL)
	if !validation.Valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": validation.Message})
		return
	}
	if validation.NeedsConfirm {
		// Read-only validation never returns NeedsConfirm; this serves as a second guard, forbidding any write.
		c.JSON(http.StatusForbidden, gin.H{"error": "goteams-db 为只读技能，禁止写入或变更操作"})
		return
	}

	// Get the database configuration
	var dbType, host, databaseName, username, secretRef string
	var port int
	var sshProfileID int64
	err := h.dbRef.Get().QueryRow(
		`SELECT db_type, host, port, database_name, username, secret_ref, ssh_profile_id FROM gt_database_profiles WHERE id = ?`,
		body.DatabaseProfileID).Scan(&dbType, &host, &port, &databaseName, &username, &secretRef, &sshProfileID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "数据库配置不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	dbCfg := dbconn.DBConfig{
		Type:         dbType,
		Host:         host,
		Port:         port,
		DatabaseName: databaseName,
		Username:     username,
		Password:     h.lookupSecret(secretRef),
	}
	sshCfg, err := dbconn.ResolveSSHConfig(c.Request.Context(), h.dbRef.Get(), h.lookupSecret, sshProfileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	targetDB, cleanup, err := dbconn.Open(c.Request.Context(), dbCfg, sshCfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer cleanup()

	targetDB.SetConnMaxLifetime(30 * time.Second)

	// Execute SQL
	upperSQL := strings.ToUpper(strings.TrimSpace(body.SQL))
	if strings.HasPrefix(upperSQL, "SELECT") || strings.HasPrefix(upperSQL, "EXPLAIN") ||
		strings.HasPrefix(upperSQL, "SHOW") || strings.HasPrefix(upperSQL, "DESC") {
		// Query
		rows, err := targetDB.Query(body.SQL)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取结果列失败: " + err.Error()})
			return
		}
		results := make([]map[string]interface{}, 0)
		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			// Scan failure must error: silently skipping would let the user receive a result that looks successful
			// but actually has missing rows, which is highly misleading in data-reconciliation scenarios.
			if err := rows.Scan(valuePtrs...); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "读取查询结果失败: " + err.Error()})
				return
			}
			row := make(map[string]interface{})
			for i, col := range columns {
				if b, ok := values[i].([]byte); ok {
					row[col] = string(b)
				} else {
					row[col] = values[i]
				}
			}
			results = append(results, row)
		}
		if err := rows.Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "遍历查询结果失败: " + err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"columns": columns,
			"rows":    results,
			"count":   len(results),
		})
	}
}

// ========== Helper functions ==========

func now() int64 {
	return time.Now().UnixMilli()
}

func marshalJSON(v interface{}) string {
	b, _ := jsonMarshal(v)
	return string(b)
}

func formatSetClause(fields []string) []string {
	result := make([]string, len(fields))
	for i, f := range fields {
		result[i] = f + " = ?"
	}
	return result
}

// Avoid importing encoding/json directly (may conflict with other files)
var sqlPattern = regexp.MustCompile(`^\s*(\w+)`)

// goteams-db is a read-only skill; forbid any write / DDL / privilege / transaction-control operation
var dangerousSQLPattern = regexp.MustCompile(`\b(DROP|DELETE|TRUNCATE|ALTER|CREATE|GRANT|REVOKE|SHUTDOWN|INSERT|UPDATE|MERGE|REPLACE|SET|BEGIN|COMMIT|ROLLBACK)\b`)

// jsonMarshal wraps json.Marshal
func jsonMarshal(v interface{}) ([]byte, error) {
	return jsonMarshalImpl(v)
}
