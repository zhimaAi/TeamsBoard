package configcenter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/ssh"

	"goteams-client/internal/config"
	"goteams-client/internal/dbconn"
)

// ========== Git project ==========

func (h *Handler) createGitProject(c *gin.Context) {
	var body struct {
		Name          string `json:"name"`
		SSHProfileID  int64  `json:"ssh_profile_id"`
		RemoteWorkDir string `json:"remote_work_dir"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Name == "" || body.SSHProfileID <= 0 || body.RemoteWorkDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name、ssh_profile_id 和 remote_work_dir 不能为空"})
		return
	}
	var sshCount int
	if err := h.dbRef.Get().QueryRow(
		`SELECT COUNT(*) FROM gt_ssh_profiles WHERE id = ?`, body.SSHProfileID,
	).Scan(&sshCount); err != nil || sshCount == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SSH 配置不存在"})
		return
	}

	id, err := h.bindAndInsert("gt_git_projects",
		[]string{"name", "ssh_profile_id", "remote_work_dir", "created_at", "updated_at"},
		[]interface{}{body.Name, body.SSHProfileID, body.RemoteWorkDir, now(), now()})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *Handler) updateGitProject(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	var body struct {
		Name          *string `json:"name"`
		SSHProfileID  *int64  `json:"ssh_profile_id"`
		RemoteWorkDir *string `json:"remote_work_dir"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var fields []string
	var values []interface{}
	if body.Name != nil {
		fields = append(fields, "name")
		values = append(values, *body.Name)
	}
	if body.SSHProfileID != nil {
		var sshCount int
		if *body.SSHProfileID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ssh_profile_id 不能为空"})
			return
		}
		if err := h.dbRef.Get().QueryRow(
			`SELECT COUNT(*) FROM gt_ssh_profiles WHERE id = ?`, *body.SSHProfileID,
		).Scan(&sshCount); err != nil || sshCount == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "SSH 配置不存在"})
			return
		}
		fields = append(fields, "ssh_profile_id")
		values = append(values, *body.SSHProfileID)
	}
	if body.RemoteWorkDir != nil {
		if *body.RemoteWorkDir == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "remote_work_dir 不能为空"})
			return
		}
		fields = append(fields, "remote_work_dir")
		values = append(values, *body.RemoteWorkDir)
	}
	if len(fields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有要更新的字段"})
		return
	}
	fields = append(fields, "updated_at")
	values = append(values, now())
	if err := h.bindAndUpdate("gt_git_projects", fields, values, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ========== SSH configuration ==========

func (h *Handler) createSSHProfile(c *gin.Context) {
	var body struct {
		Name     string `json:"name"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
		KeyPath  string `json:"key_path"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Name == "" || body.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name 和 host 不能为空"})
		return
	}
	if body.Port == 0 {
		body.Port = 22
	}

	id, err := h.bindAndInsert("gt_ssh_profiles",
		[]string{"name", "host", "port", "username", "secret_ref", "key_path", "created_at", "updated_at"},
		[]interface{}{body.Name, body.Host, body.Port, body.Username, "", body.KeyPath, now(), now()})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if body.Password != "" {
		if err := h.persistSecretRef(c, "gt_ssh_profiles", "ssh", id, body.Password); err != nil {
			return
		}
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *Handler) updateSSHProfile(c *gin.Context) {
	id, err := parseID(c)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var fields []string
	var values []interface{}
	for _, f := range []string{"name", "host", "port", "username", "key_path"} {
		if v, ok := body[f]; ok {
			fields = append(fields, f)
			values = append(values, v)
		}
	}
	if v, ok := body["password"].(string); ok && v != "" {
		// Failure to write the keystore must be interrupted: continuing execution will overwrite the empty ref into secret_ref.
		// The result is that the password is silently lost and the original reference is erased.
		ref, err := h.handleSecret("ssh", fmt.Sprintf("%d", id), v)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存 SSH 密码失败: " + err.Error()})
			return
		}
		fields = append(fields, "secret_ref")
		values = append(values, ref)
	}
	if len(fields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有要更新的字段"})
		return
	}
	fields = append(fields, "updated_at")
	values = append(values, now())
	if err := h.bindAndUpdate("gt_ssh_profiles", fields, values, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新 SSH 配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ========== Docker project ==========

func (h *Handler) createDockerProject(c *gin.Context) {
	var body struct {
		Name         string `json:"name"`
		SSHProfileID int64  `json:"ssh_profile_id"`
		ComposeFile  string `json:"compose_file_path"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Name == "" || body.ComposeFile == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name 和 compose_file_path 不能为空"})
		return
	}
	if body.SSHProfileID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ssh_profile_id 不能为空"})
		return
	}
	var sshCount int
	if err := h.dbRef.Get().QueryRow(
		`SELECT COUNT(*) FROM gt_ssh_profiles WHERE id = ?`, body.SSHProfileID,
	).Scan(&sshCount); err != nil || sshCount == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SSH 配置不存在"})
		return
	}
	id, err := h.bindAndInsert("gt_docker_projects",
		[]string{"name", "ssh_profile_id", "compose_file_path", "created_at", "updated_at"},
		[]interface{}{body.Name, body.SSHProfileID, body.ComposeFile, now(), now()})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *Handler) updateDockerProject(c *gin.Context) {
	id, err := parseID(c)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var fields []string
	var values []interface{}
	for _, f := range []string{"name", "ssh_profile_id", "compose_file_path"} {
		if v, ok := body[f]; ok {
			if f == "ssh_profile_id" {
				if n, ok := toInt64(v); !ok || n <= 0 {
					c.JSON(http.StatusBadRequest, gin.H{"error": "ssh_profile_id 不能为空"})
					return
				}
				var sshCount int
				if err := h.dbRef.Get().QueryRow(
					`SELECT COUNT(*) FROM gt_ssh_profiles WHERE id = ?`, v,
				).Scan(&sshCount); err != nil || sshCount == 0 {
					c.JSON(http.StatusBadRequest, gin.H{"error": "SSH 配置不存在"})
					return
				}
			}
			fields = append(fields, f)
			values = append(values, v)
		}
	}
	if len(fields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有要更新的字段"})
		return
	}
	fields = append(fields, "updated_at")
	values = append(values, now())
	if err := h.bindAndUpdate("gt_docker_projects", fields, values, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新 Docker 项目失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ========== Database configuration ==========

func (h *Handler) createDatabaseProfile(c *gin.Context) {
	var body struct {
		Name         string `json:"name"`
		DBType       string `json:"db_type"`
		Host         string `json:"host"`
		Port         int    `json:"port"`
		DatabaseName string `json:"database_name"`
		Username     string `json:"username"`
		Password     string `json:"password"`
		SSHProfileID int64  `json:"ssh_profile_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name 不能为空"})
		return
	}
	if body.DBType == "" {
		body.DBType = "mysql"
	}
	if body.Port == 0 {
		body.Port = 3306
	}

	id, err := h.bindAndInsert("gt_database_profiles",
		[]string{"name", "db_type", "host", "port", "database_name", "username", "secret_ref", "ssh_profile_id", "created_at", "updated_at"},
		[]interface{}{body.Name, body.DBType, body.Host, body.Port, body.DatabaseName, body.Username, "", body.SSHProfileID, now(), now()})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if body.Password != "" {
		if err := h.persistSecretRef(c, "gt_database_profiles", "db", id, body.Password); err != nil {
			return
		}
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *Handler) updateDatabaseProfile(c *gin.Context) {
	id, err := parseID(c)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var fields []string
	var values []interface{}
	for _, f := range []string{"name", "db_type", "host", "port", "database_name", "username", "ssh_profile_id"} {
		if v, ok := body[f]; ok {
			fields = append(fields, f)
			values = append(values, v)
		}
	}
	if v, ok := body["password"].(string); ok && v != "" {
		ref, err := h.handleSecret("db", fmt.Sprintf("%d", id), v)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存数据库密码失败: " + err.Error()})
			return
		}
		fields = append(fields, "secret_ref")
		values = append(values, ref)
	}
	if len(fields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有要更新的字段"})
		return
	}
	fields = append(fields, "updated_at")
	values = append(values, now())
	if err := h.bindAndUpdate("gt_database_profiles", fields, values, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新数据库配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ========== Model service provider ==========

func (h *Handler) createModelProvider(c *gin.Context) {
	var body struct {
		Name         string `json:"name"`
		ProviderType string `json:"provider_type"`
		APIBaseURL   string `json:"api_base_url"`
		APIKey       string `json:"api_key"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name 不能为空"})
		return
	}
	id, err := h.bindAndInsert("gt_model_providers",
		[]string{"name", "provider_type", "api_base_url", "secret_ref", "created_at", "updated_at"},
		[]interface{}{body.Name, body.ProviderType, body.APIBaseURL, "", now(), now()})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if body.APIKey != "" {
		if err := h.persistSecretRef(c, "gt_model_providers", "model_provider", id, body.APIKey); err != nil {
			return
		}
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *Handler) updateModelProvider(c *gin.Context) {
	id, err := parseID(c)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var fields []string
	var values []interface{}
	for _, f := range []string{"name", "provider_type", "api_base_url"} {
		if v, ok := body[f]; ok {
			fields = append(fields, f)
			values = append(values, v)
		}
	}
	if v, ok := body["api_key"].(string); ok && v != "" {
		ref, err := h.handleSecret("model_provider", fmt.Sprintf("%d", id), v)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存 API Key 失败: " + err.Error()})
			return
		}
		fields = append(fields, "secret_ref")
		values = append(values, ref)
	}
	if len(fields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有要更新的字段"})
		return
	}
	fields = append(fields, "updated_at")
	values = append(values, now())
	if err := h.bindAndUpdate("gt_model_providers", fields, values, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新模型服务商失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ========== Model Profile ==========

func (h *Handler) createModelProfile(c *gin.Context) {
	var body struct {
		Name        string  `json:"name"`
		ProviderID  int64   `json:"provider_id"`
		ModelName   string  `json:"model_name"`
		Temperature float64 `json:"temperature"`
		MaxTokens   int     `json:"max_tokens"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Name == "" || body.ProviderID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name 和 provider_id 不能为空"})
		return
	}
	if body.Temperature == 0 {
		body.Temperature = 0.7
	}
	if body.MaxTokens == 0 {
		body.MaxTokens = 4096
	}

	id, err := h.bindAndInsert("gt_model_profiles",
		[]string{"name", "provider_id", "model_name", "temperature", "max_tokens", "created_at", "updated_at"},
		[]interface{}{body.Name, body.ProviderID, body.ModelName, body.Temperature, body.MaxTokens, now(), now()})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *Handler) updateModelProfile(c *gin.Context) {
	id, err := parseID(c)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return
	}
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var fields []string
	var values []interface{}
	for _, f := range []string{"name", "provider_id", "model_name", "temperature", "max_tokens"} {
		if v, ok := body[f]; ok {
			fields = append(fields, f)
			values = append(values, v)
		}
	}
	if len(fields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有要更新的字段"})
		return
	}
	fields = append(fields, "updated_at")
	values = append(values, now())
	if err := h.bindAndUpdate("gt_model_profiles", fields, values, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新模型配置失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ========== Cloud connection configuration ==========

// getCloudConfig gets the cloud connection configuration
// GET /api/local/config/cloud
func (h *Handler) getCloudConfig(c *gin.Context) {
	apiBaseURL := ""
	if h.cloudCfg != nil {
		apiBaseURL = h.cloudCfg.APIBaseURL
	}
	if h.configPath != "" {
		// Read disk config.ini override only if APP_ENV is not set;
		// When there is APP_ENV, strictly refer to the embedded config_<env>.ini to avoid interference from disk residual overwriting.
		if env := strings.TrimSpace(os.Getenv("APP_ENV")); env == "" {
			if ini, err := config.LoadINI(h.configPath); err == nil {
				if override := ini.Get("cloud", "api_base_url"); override != "" {
					apiBaseURL = override
				}
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"api_base_url": apiBaseURL,
	})
}

// setCloudConfig sets cloud connection configuration
// PUT /api/local/config/cloud
func (h *Handler) setCloudConfig(c *gin.Context) {
	var body struct {
		APIBaseURL string `json:"api_base_url"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.APIBaseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "api_base_url 不能为空"})
		return
	}
	if h.configPath == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "配置文件路径未初始化"})
		return
	}

	ini, err := config.LoadINI(h.configPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取配置文件失败: " + err.Error()})
			return
		}
		if err := os.MkdirAll(filepath.Dir(h.configPath), 0700); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建配置目录失败: " + err.Error()})
			return
		}
		ini, err = config.LoadINIBytes(nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "初始化配置文件失败: " + err.Error()})
			return
		}
		ini.SetPath(h.configPath)
	}
	ini.Set("cloud", "api_base_url", body.APIBaseURL)
	if err := ini.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存配置文件失败: " + err.Error()})
		return
	}
	if h.cloudCfg != nil {
		h.cloudCfg.APIBaseURL = body.APIBaseURL
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// persistSecretRef writes the secret and backfills secret_ref after the record is created successfully.
//
// Cannot be silent when key writing fails: the record has been dropped into the database, but the loss of the password will cause subsequent connections to always fail without any prompt.
// Here, the HTTP error is directly written back and error is returned. The caller interrupts accordingly and returns the id at the same time.
// It is convenient for the front end to prompt "The configuration has been created, please refill the password" instead of creating it again.
func (h *Handler) persistSecretRef(c *gin.Context, table, prefix string, id int64, secret string) error {
	ref, err := h.handleSecret(prefix, fmt.Sprintf("%d", id), secret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"id":    id,
			"error": "配置已创建，但密钥保存失败，请编辑后重新填写: " + err.Error(),
		})
		return err
	}
	if err := h.bindAndUpdate(table, []string{"secret_ref"}, []interface{}{ref}, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"id":    id,
			"error": "配置已创建，但密钥引用写入失败，请编辑后重新填写: " + err.Error(),
		})
		return err
	}
	return nil
}

// ---- Test connection ----

const connectionTestTimeout = 10 * time.Second

// sshTestConfig describes the parameters required to test an SSH connection.
type sshTestConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	KeyPath  string
}

// buildSSHAuthMethods generates SSH authentication methods based on password/private key.
func buildSSHAuthMethods(cfg sshTestConfig) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if cfg.Password != "" {
		methods = append(methods, ssh.Password(cfg.Password))
	}
	if cfg.KeyPath != "" {
		keyData, err := os.ReadFile(cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("读取私钥失败: %w", err)
		}
		var signer ssh.Signer
		if cfg.Password != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(cfg.Password))
		} else {
			signer, err = ssh.ParsePrivateKey(keyData)
		}
		if err != nil {
			return nil, fmt.Errorf("解析私钥失败: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("未配置密码或私钥，无法认证")
	}
	return methods, nil
}

// testSSHConnection establishes an SSH connection and closes it immediately to verify whether the configuration is available.
func testSSHConnection(ctx context.Context, cfg sshTestConfig) error {
	if cfg.Host == "" || cfg.Username == "" {
		return fmt.Errorf("SSH 主机和用户名不能为空")
	}
	port := cfg.Port
	if port == 0 {
		port = 22
	}
	authMethods, err := buildSSHAuthMethods(cfg)
	if err != nil {
		return err
	}
	clientConfig := &ssh.ClientConfig{
		User:            cfg.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         connectionTestTimeout,
	}
	address := net.JoinHostPort(cfg.Host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: connectionTestTimeout}
	netConn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("无法连接 SSH 主机: %w", err)
	}
	conn, channels, requests, err := ssh.NewClientConn(netConn, address, clientConfig)
	if err != nil {
		netConn.Close()
		return fmt.Errorf("SSH 认证或握手失败: %w", err)
	}
	client := ssh.NewClient(conn, channels, requests)
	return client.Close()
}

func (h *Handler) lookupSecret(ref string) string {
	if ref == "" || h.store == nil {
		return ""
	}
	if pwd, err := h.store.Get(ref); err == nil {
		return pwd
	}
	return ""
}

// testSSHProfile tests whether the specified SSH configuration can be connected.
func (h *Handler) testSSHProfile(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID", "ok": false})
		return
	}
	var host string
	var port int
	var username, secretRef, keyPath string
	err = h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT host, port, username, secret_ref, key_path FROM gt_ssh_profiles WHERE id = ?`, id).
		Scan(&host, &port, &username, &secretRef, &keyPath)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "SSH 配置不存在", "ok": false})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "ok": false})
		return
	}
	cfg := sshTestConfig{Host: host, Port: port, Username: username, Password: h.lookupSecret(secretRef), KeyPath: keyPath}
	if err := testSSHConnection(c.Request.Context(), cfg); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// testDatabaseProfile tests whether the specified database configuration can be connected.
func (h *Handler) testDatabaseProfile(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID", "ok": false})
		return
	}
	var dbType, host, databaseName, username, secretRef string
	var port int
	var sshProfileID int64
	err = h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT db_type, host, port, database_name, username, secret_ref, ssh_profile_id
		 FROM gt_database_profiles WHERE id = ?`, id).
		Scan(&dbType, &host, &port, &databaseName, &username, &secretRef, &sshProfileID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "数据库配置不存在", "ok": false})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "ok": false})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "ok": false})
		return
	}

	db, cleanup, err := dbconn.Open(c.Request.Context(), dbCfg, sshCfg)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	defer cleanup()

	pingCtx, cancel := context.WithTimeout(c.Request.Context(), connectionTestTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": fmt.Sprintf("连接数据库失败: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// toInt64 converts numbers (float64) or strings in JSON to int64
func toInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	case string:
		if i, err := strconv.ParseInt(n, 10, 64); err == nil {
			return i, true
		}
	}
	return 0, false
}
