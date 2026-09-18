package configcenter

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"goteams-client/internal/applog"
	"goteams-client/internal/config"
	"goteams-client/internal/i18n"
	"goteams-client/internal/secrets"
	"goteams-client/internal/storage"
)

// Handler configuration center CRUD processor
type Handler struct {
	dbRef       *storage.DBRef
	store       secrets.Store
	cloudCfg    *config.CloudConfig
	configPath  string // INI configuration file path
	accountIDFn func() (string, error)
}

// NewHandler creates a configuration central processor
// accountIDFn is used to isolate key references according to the current account to avoid key serial numbers when different accounts use the same ID in the DB.
func NewHandler(dbRef *storage.DBRef, store secrets.Store, cloudCfg *config.CloudConfig, accountIDFn func() (string, error)) *Handler {
	h := &Handler{dbRef: dbRef, store: store, cloudCfg: cloudCfg, accountIDFn: accountIDFn}
	if cloudCfg != nil {
		h.configPath = cloudCfg.INIPath()
	}
	return h
}

// RegisterRoutes registers all configuration center routes
func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	// Git project
	r.GET("/git-projects", h.listConfig("gt_git_projects"))
	r.POST("/git-projects", h.createGitProject)
	r.PUT("/git-projects/:id", h.updateGitProject)
	r.DELETE("/git-projects/:id", h.deleteConfig("gt_git_projects"))

	// SSH configuration
	r.GET("/ssh-profiles", h.listConfig("gt_ssh_profiles"))
	r.POST("/ssh-profiles", h.createSSHProfile)
	r.PUT("/ssh-profiles/:id", h.updateSSHProfile)
	r.DELETE("/ssh-profiles/:id", h.deleteConfig("gt_ssh_profiles"))
	r.POST("/ssh-profiles/:id/test", h.testSSHProfile)

	// Docker project
	r.GET("/docker-projects", h.listConfig("gt_docker_projects"))
	r.POST("/docker-projects", h.createDockerProject)
	r.PUT("/docker-projects/:id", h.updateDockerProject)
	r.DELETE("/docker-projects/:id", h.deleteConfig("gt_docker_projects"))

	//Database configuration
	r.GET("/database-profiles", h.listConfig("gt_database_profiles"))
	r.POST("/database-profiles", h.createDatabaseProfile)
	r.PUT("/database-profiles/:id", h.updateDatabaseProfile)
	r.DELETE("/database-profiles/:id", h.deleteConfig("gt_database_profiles"))
	r.POST("/database-profiles/:id/test", h.testDatabaseProfile)

	//Model service provider
	r.GET("/model-providers", h.listConfig("gt_model_providers"))
	r.POST("/model-providers", h.createModelProvider)
	r.PUT("/model-providers/:id", h.updateModelProvider)
	r.DELETE("/model-providers/:id", h.deleteConfig("gt_model_providers"))

	// Model Profile
	r.GET("/model-profiles", h.listConfig("gt_model_profiles"))
	r.POST("/model-profiles", h.createModelProfile)
	r.PUT("/model-profiles/:id", h.updateModelProfile)
	r.DELETE("/model-profiles/:id", h.deleteConfig("gt_model_profiles"))

}

// RegisterCloudRoutes register cloud connection configuration routing (no need to log in)
// You need to read/set the cloud address when starting up for the first time and before logging in, so it is placed in the public routing group.
func (h *Handler) RegisterCloudRoutes(r *gin.RouterGroup) {
	r.GET("/cloud", h.getCloudConfig)
	r.PUT("/cloud", h.setCloudConfig)
}

// now returns the current Unix millisecond timestamp
func now() int64 {
	return time.Now().UnixMilli()
}

// parseID parses the ID from the path parameter
func parseID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("id"), 10, 64)
}

// listConfig general list query
func (h *Handler) listConfig(table string) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
		if page < 1 {
			page = 1
		}
		if pageSize < 1 || pageSize > 200 {
			pageSize = 50
		}
		offset := (page - 1) * pageSize

		rows, err := h.dbRef.Get().Query(fmt.Sprintf(
			"SELECT * FROM %s ORDER BY id DESC LIMIT ? OFFSET ?", table), pageSize, offset)
		if err != nil {
			i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
			return
		}
		defer rows.Close()

		results, err := rowsToJSON(rows)
		if err != nil {
			i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
			return
		}

		var total int64
		if err := h.dbRef.Get().QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&total); err != nil {
			applog.Warn("查询配置总数失败", "table", table, "error", err)
		}

		c.JSON(http.StatusOK, gin.H{
			"items":     results,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		})
	}
}

// deleteConfig universal delete
func (h *Handler) deleteConfig(table string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := parseID(c)
		if err != nil {
			i18n.Error(c, http.StatusBadRequest, "common_invalid_id", "")
			return
		}
		result, err := h.dbRef.Get().Exec(fmt.Sprintf("DELETE FROM %s WHERE id = ?", table), id)
		if err != nil {
			i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
			return
		}
		rows, err := result.RowsAffected()
		if err != nil {
			i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
			return
		}
		if rows == 0 {
			i18n.Error(c, http.StatusNotFound, "common_record_not_found", "")
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

// rowsToJSON converts sql.Rows to []map[string]interface{}
func rowsToJSON(rows *sql.Rows) ([]map[string]interface{}, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			// Process []byte -> string
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		results = append(results, row)
	}
	return results, nil
}

// handleSecret handles the key field: if there is a password, store it in SecretStore and return secret_ref
// The key references the embedded account ID to ensure that there will be no aliasing even if different accounts have the same ID in the DB.
func (h *Handler) handleSecret(prefix, id, password string) (string, error) {
	if password == "" {
		return "", nil
	}
	accountID := ""
	if h.accountIDFn != nil {
		if aid, err := h.accountIDFn(); err == nil {
			accountID = aid
		}
	}
	key := fmt.Sprintf("acct:%s:%s:%s", accountID, prefix, id)
	if err := h.store.Set(key, password); err != nil {
		return "", err
	}
	return key, nil
}

// bindAndInsert universal insertion
func (h *Handler) bindAndInsert(table string, fields []string, values []interface{}) (int64, error) {
	placeholders := make([]string, len(fields))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table, strings.Join(fields, ", "), strings.Join(placeholders, ", "))
	result, err := h.dbRef.Get().Exec(query, values...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// bindAndUpdate general update
func (h *Handler) bindAndUpdate(table string, fields []string, values []interface{}, id int64) error {
	setParts := make([]string, len(fields))
	for i, f := range fields {
		setParts[i] = fmt.Sprintf("%s = ?", f)
	}
	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = ?",
		table, strings.Join(setParts, ", "))
	values = append(values, id)
	_, err := h.dbRef.Get().Exec(query, values...)
	return err
}

// jsonBind binds JSON and returns map
func jsonBind(c *gin.Context) (map[string]interface{}, error) {
	var m map[string]interface{}
	if err := c.ShouldBindJSON(&m); err != nil {
		return nil, err
	}
	return m, nil
}

// marshalJSON serializes the value into a JSON string
func marshalJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
