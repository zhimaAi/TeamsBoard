package command

import (
	"context"
	"database/sql"
	"encoding/json"
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

	"goteams-client/internal/i18n"
	"goteams-client/internal/secrets"
	"goteams-client/internal/storage"
)

// Command execution timeout
const commandTimeout = 30 * time.Second

// Output maximum length (100KB)
const maxOutputSize = 100 * 1024

// GitProject Git project
type GitProject struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	SSHProfileID  int64  `json:"ssh_profile_id"`
	SSHName       string `json:"ssh_name"`
	RemoteWorkDir string `json:"remote_work_dir"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// DockerProject Docker project
type DockerProject struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	SSHProfileID    int64  `json:"ssh_profile_id"`
	ComposeFilePath string `json:"compose_file_path"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

// CommandRun command execution record
type CommandRun struct {
	ID          int64    `json:"id"`
	CommandType string   `json:"command_type"`
	ProjectID   int64    `json:"project_id"`
	Command     string   `json:"command"`
	Args        []string `json:"args"`
	WorkDir     string   `json:"work_dir"`
	Status      string   `json:"status"`
	ExitCode    int      `json:"exit_code"`
	Output      string   `json:"output"`
	DurationMs  int64    `json:"duration_ms"`
	StartedAt   int64    `json:"started_at"`
	FinishedAt  int64    `json:"finished_at"`
}

// RunResult command execution result
type RunResult struct {
	RunID      int64  `json:"run_id"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output"`
	DurationMs int64  `json:"duration_ms"`
}

// Handler command execution processor
type Handler struct {
	dbRef       *storage.DBRef
	secretStore secrets.Store
}

// NewHandler creates a command execution processor
func NewHandler(dbRef *storage.DBRef, secretStore secrets.Store) *Handler {
	return &Handler{dbRef: dbRef, secretStore: secretStore}
}

// nowMillis returns the current Unix millisecond timestamp
func nowMillis() int64 {
	return time.Now().UnixMilli()
}

// gitWhitelist allowed Git subcommand whitelist
var gitWhitelist = map[string]bool{
	"status": true, "log": true, "branch": true, "diff": true,
	"fetch": true, "pull": true, "push": true, "add": true,
	"commit": true, "checkout": true, "stash": true,
	"merge": true, "rebase": true,
}

// dockerWhitelist Allowed Docker Compose subcommand whitelist
var dockerWhitelist = map[string]bool{
	"ps": true, "up": true, "down": true, "logs": true,
	"build": true, "restart": true, "stop": true,
	"start": true, "config": true, "images": true,
}

// ==================== Git commands ====================

// ListGitProjects gets the configured Git project list
// GET /api/local/commands/git/projects
func (h *Handler) ListGitProjects(c *gin.Context) {
	rows, err := h.dbRef.Get().QueryContext(c.Request.Context(),
		`SELECT p.id, p.name, p.ssh_profile_id, COALESCE(s.name, ''), p.remote_work_dir, p.created_at, p.updated_at
		 FROM gt_git_projects p
		 LEFT JOIN gt_ssh_profiles s ON s.id = p.ssh_profile_id
		 ORDER BY p.id ASC`)
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	defer rows.Close()

	var list []GitProject
	for rows.Next() {
		var p GitProject
		if err := rows.Scan(&p.ID, &p.Name, &p.SSHProfileID, &p.SSHName, &p.RemoteWorkDir, &p.CreatedAt, &p.UpdatedAt); err != nil {
			i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
			return
		}
		list = append(list, p)
	}

	if list == nil {
		list = []GitProject{}
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

// RunGitCommand executes Git command
// POST /api/local/commands/git/run
func (h *Handler) RunGitCommand(c *gin.Context) {
	var input struct {
		ProjectID int64    `json:"project_id"`
		Command   string   `json:"command"`
		Args      []string `json:"args"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		i18n.Error(c, http.StatusBadRequest, "common_request_invalid", "")
		return
	}
	if input.Command == "" {
		i18n.Error(c, http.StatusBadRequest, "common_request_invalid", "")
		return
	}
	if !gitWhitelist[input.Command] {
		i18n.Error(c, http.StatusBadRequest, "common_command_not_allowed", "")
		return
	}

	// Get the remote working directory and SSH configuration
	var remoteWorkDir string
	var sshConfig SSHConnectionConfig
	var secretRef string
	err := h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT p.remote_work_dir, s.host, s.port, s.username, s.secret_ref, s.key_path
		 FROM gt_git_projects p
		 JOIN gt_ssh_profiles s ON s.id = p.ssh_profile_id
		 WHERE p.id = ?`, input.ProjectID).
		Scan(&remoteWorkDir, &sshConfig.Host, &sshConfig.Port, &sshConfig.Username, &secretRef, &sshConfig.KeyPath)
	if err == sql.ErrNoRows {
		i18n.Error(c, http.StatusNotFound, "common_record_not_found", "")
		return
	}
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	if secretRef != "" {
		if h.secretStore == nil {
			i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
			return
		}
		sshConfig.Password, err = h.secretStore.Get(secretRef)
		if err != nil {
			i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
			return
		}
	}

	remoteCommand := buildRemoteGitCommand(remoteWorkDir, input.Command, input.Args)
	result := h.executeAndRecordSSH(
		c.Request.Context(), "git", input.ProjectID, input.Command, input.Args,
		remoteWorkDir, sshConfig, remoteCommand,
	)
	c.JSON(http.StatusOK, result)
}

// ==================== Docker commands ====================

// ListDockerProjects Gets the list of configured Docker projects
// GET /api/local/commands/docker/projects
func (h *Handler) ListDockerProjects(c *gin.Context) {
	rows, err := h.dbRef.Get().QueryContext(c.Request.Context(),
		`SELECT id, name, ssh_profile_id, compose_file_path, created_at, updated_at FROM gt_docker_projects ORDER BY id ASC`)
	if err != nil {
		i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
		return
	}
	defer rows.Close()

	var list []DockerProject
	for rows.Next() {
		var p DockerProject
		if err := rows.Scan(&p.ID, &p.Name, &p.SSHProfileID, &p.ComposeFilePath, &p.CreatedAt, &p.UpdatedAt); err != nil {
			i18n.Error(c, http.StatusInternalServerError, "common_server_error", "")
			return
		}
		list = append(list, p)
	}

	if list == nil {
		list = []DockerProject{}
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

// RunDockerCommand executes the Docker Compose command
// POST /api/local/commands/docker/run
func (h *Handler) RunDockerCommand(c *gin.Context) {
	var input struct {
		ProjectID int64    `json:"project_id"`
		Command   string   `json:"command"`
		Args      []string `json:"args"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		i18n.Error(c, http.StatusBadRequest, "common_request_invalid", "")
		return
	}
	if input.Command == "" {
		i18n.Error(c, http.StatusBadRequest, "common_request_invalid", "")
		return
	}
	if !dockerWhitelist[input.Command] {
		i18n.Error(c, http.StatusBadRequest, "common_command_not_allowed", "")
		return
	}

	// Get the compose file path and associated SSH configuration
	var composeFilePath string
	var sshConfig SSHConnectionConfig
	var secretRef string
	err := h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT p.compose_file_path, s.host, s.port, s.username, s.secret_ref, s.key_path
		 FROM gt_docker_projects p
		 JOIN gt_ssh_profiles s ON s.id = p.ssh_profile_id
		 WHERE p.id = ?`, input.ProjectID).
		Scan(&composeFilePath, &sshConfig.Host, &sshConfig.Port, &sshConfig.Username, &secretRef, &sshConfig.KeyPath)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.T(c, "command_git_project_or_ssh_not_found")})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": i18n.T(c, "command_docker_projects_query_failed")})
		return
	}
	if secretRef != "" {
		if h.secretStore == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": i18n.T(c, "command_ssh_store_uninitialized")})
			return
		}
		sshConfig.Password, err = h.secretStore.Get(secretRef)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": i18n.T(c, "command_ssh_credential_read_failed")})
			return
		}
	}

	// Build remote command: sudo docker compose -f <file> <command> <args...>
	// Remote execution of docker compose requires administrator rights, and sudo is automatically added.
	parts := []string{"sudo", "docker", "compose", "-f", shellQuote(composeFilePath), shellQuote(input.Command)}
	for _, arg := range input.Args {
		parts = append(parts, shellQuote(arg))
	}
	remoteCommand := strings.Join(parts, " ")
	result := h.executeAndRecordSSH(
		c.Request.Context(), "docker", input.ProjectID, input.Command, input.Args,
		"", sshConfig, remoteCommand,
	)
	c.JSON(http.StatusOK, result)
}

// ==================== Command history ====================

// ListHistory command history list
// GET /api/local/commands/history?type=git&page=1&page_size=20
func (h *Handler) ListHistory(c *gin.Context) {
	cmdType := c.Query("type")
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}
	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil || pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	// Build query
	query := `SELECT id, command_type, project_id, command, args_json, work_dir, status, exit_code, output, duration_ms, started_at, finished_at FROM gt_command_runs`
	var rows *sql.Rows
	if cmdType != "" {
		query += ` WHERE command_type=? ORDER BY started_at DESC LIMIT ? OFFSET ?`
		rows, err = h.dbRef.Get().QueryContext(c.Request.Context(), query, cmdType, pageSize, offset)
	} else {
		query += ` ORDER BY started_at DESC LIMIT ? OFFSET ?`
		rows, err = h.dbRef.Get().QueryContext(c.Request.Context(), query, pageSize, offset)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": i18n.T(c, "command_history_query_failed")})
		return
	}
	defer rows.Close()

	var list []CommandRun
	for rows.Next() {
		var r CommandRun
		var argsJSON string
		if err := rows.Scan(&r.ID, &r.CommandType, &r.ProjectID, &r.Command, &argsJSON, &r.WorkDir, &r.Status, &r.ExitCode, &r.Output, &r.DurationMs, &r.StartedAt, &r.FinishedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": i18n.T(c, "command_history_scan_failed")})
			return
		}
		r.Args = parseStringSlice(argsJSON)
		list = append(list, r)
	}

	//Total number of queries
	var total int64
	countQuery := `SELECT COUNT(*) FROM gt_command_runs`
	if cmdType != "" {
		countQuery += ` WHERE command_type=?`
		h.dbRef.Get().QueryRowContext(c.Request.Context(), countQuery, cmdType).Scan(&total)
	} else {
		h.dbRef.Get().QueryRowContext(c.Request.Context(), countQuery).Scan(&total)
	}

	if list == nil {
		list = []CommandRun{}
	}
	c.JSON(http.StatusOK, gin.H{
		"data":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetHistoryDetail command execution details
// GET /api/local/commands/history/:id
func (h *Handler) GetHistoryDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.T(c, "command_history_id_invalid")})
		return
	}

	var r CommandRun
	var argsJSON string
	err = h.dbRef.Get().QueryRowContext(c.Request.Context(),
		`SELECT id, command_type, project_id, command, args_json, work_dir, status, exit_code, output, duration_ms, started_at, finished_at FROM gt_command_runs WHERE id=?`, id).
		Scan(&r.ID, &r.CommandType, &r.ProjectID, &r.Command, &argsJSON, &r.WorkDir, &r.Status, &r.ExitCode, &r.Output, &r.DurationMs, &r.StartedAt, &r.FinishedAt)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.T(c, "command_history_record_not_found")})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": i18n.T(c, "command_history_detail_query_failed")})
		return
	}

	r.Args = parseStringSlice(argsJSON)
	c.JSON(http.StatusOK, r)
}

// ==================== Internal execution logic ====================

// SSHConnectionConfig SSH connection configuration required for a single remote command.
type SSHConnectionConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	KeyPath  string
}

// buildRemoteGitCommand constructs a Git command executed in the remote working directory.
func buildRemoteGitCommand(remoteWorkDir, command string, args []string) string {
	parts := []string{"git", shellQuote(command)}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return "cd -- " + shellQuote(remoteWorkDir) + " && " + strings.Join(parts, " ")
}

// shellQuote encodes the value as a POSIX shell parameter.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// executeAndRecordSSH executes remote commands via SSH and records the results.
func (h *Handler) executeAndRecordSSH(
	ctx context.Context,
	commandType string,
	projectID int64,
	command string,
	args []string,
	remoteWorkDir string,
	config SSHConnectionConfig,
	remoteCommand string,
) RunResult {
	startedAt := nowMillis()
	argsJSON, _ := jsonStringSlice(args)
	res, err := h.dbRef.Get().ExecContext(ctx,
		`INSERT INTO gt_command_runs (command_type, project_id, command, args_json, work_dir, status, exit_code, output, duration_ms, started_at, finished_at)
		 VALUES (?, ?, ?, ?, ?, 'running', -1, '', 0, ?, 0)`,
		commandType, projectID, command, argsJSON, remoteWorkDir, startedAt)
	if err != nil {
		return RunResult{Status: "failed", ExitCode: -1, Output: i18n.WithLocale(i18n.LocaleFromContext(ctx), "command_run_record_failed")}
	}
	runID, err := res.LastInsertId()
	if err != nil {
		return RunResult{Status: "failed", ExitCode: -1, Output: i18n.WithLocale(i18n.LocaleFromContext(ctx), "command_run_id_failed")}
	}

	execCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	output, exitCode, execErr := runSSHCommand(execCtx, config, remoteCommand)
	finishedAt := nowMillis()
	durationMs := finishedAt - startedAt

	if len(output) > maxOutputSize {
		output = output[:maxOutputSize] + "\n... (输出已截断)"
	}
	status := "success"
	if execErr != nil {
		status = "failed"
		if output != "" {
			output += "\n"
		}
		if execCtx.Err() == context.DeadlineExceeded {
			exitCode = -1
			output += "命令执行超时（30秒）"
		} else if exitCode < 0 {
			output += "SSH 命令执行失败: " + execErr.Error()
		}
	}

	_, _ = h.dbRef.Get().ExecContext(ctx,
		`UPDATE gt_command_runs SET status=?, exit_code=?, output=?, duration_ms=?, finished_at=? WHERE id=?`,
		status, exitCode, output, durationMs, finishedAt, runID)

	return RunResult{
		RunID:      runID,
		Status:     status,
		ExitCode:   exitCode,
		Output:     output,
		DurationMs: durationMs,
	}
}

func runSSHCommand(ctx context.Context, config SSHConnectionConfig, remoteCommand string) (string, int, error) {
	if config.Host == "" || config.Username == "" {
		return "", -1, fmt.Errorf("SSH 主机和用户名不能为空")
	}
	if config.Port == 0 {
		config.Port = 22
	}

	authMethods, err := sshAuthMethods(config)
	if err != nil {
		return "", -1, err
	}
	clientConfig := &ssh.ClientConfig{
		User:            config.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Consistent with dtool's current connection behavior.
		Timeout:         10 * time.Second,
	}
	address := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	netConn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return "", -1, fmt.Errorf("连接 SSH 失败: %w", err)
	}

	conn, channels, requests, err := ssh.NewClientConn(netConn, address, clientConfig)
	if err != nil {
		netConn.Close()
		return "", -1, fmt.Errorf("SSH 握手失败: %w", err)
	}
	client := ssh.NewClient(conn, channels, requests)
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", -1, fmt.Errorf("创建 SSH Session 失败: %w", err)
	}
	defer session.Close()

	type commandResult struct {
		output []byte
		err    error
	}
	resultCh := make(chan commandResult, 1)
	go func() {
		output, runErr := session.CombinedOutput(remoteCommand)
		resultCh <- commandResult{output: output, err: runErr}
	}()

	select {
	case <-ctx.Done():
		_ = client.Close()
		return "", -1, ctx.Err()
	case result := <-resultCh:
		if result.err == nil {
			return string(result.output), 0, nil
		}
		if exitErr, ok := result.err.(*ssh.ExitError); ok {
			return string(result.output), exitErr.ExitStatus(), result.err
		}
		return string(result.output), -1, result.err
	}
}

func sshAuthMethods(config SSHConnectionConfig) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if config.KeyPath != "" {
		keyPath := config.KeyPath
		if strings.HasPrefix(keyPath, "~/") || strings.HasPrefix(keyPath, `~\`) {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("解析 SSH 私钥路径失败: %w", err)
			}
			keyPath = filepath.Join(homeDir, keyPath[2:])
		}
		privateKey, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("读取 SSH 私钥失败: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(privateKey)
		if err != nil && config.Password != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(privateKey, []byte(config.Password))
		}
		if err != nil {
			return nil, fmt.Errorf("解析 SSH 私钥失败: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if config.Password != "" {
		methods = append(methods,
			ssh.Password(config.Password),
			ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = config.Password
				}
				return answers, nil
			}),
		)
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("SSH 配置未提供密码或私钥")
	}
	return methods, nil
}

// ==================== Auxiliary functions ====================

// parseStringSlice parses the JSON string into []string
func parseStringSlice(s string) []string {
	var list []string
	if s == "" || s == "[]" {
		return []string{}
	}
	if err := json.Unmarshal([]byte(s), &list); err != nil {
		return []string{}
	}
	if list == nil {
		list = []string{}
	}
	return list
}

// jsonStringSlice serializes []string to JSON string
func jsonStringSlice(s []string) (string, error) {
	if s == nil {
		s = []string{}
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "[]", err
	}
	return string(b), nil
}
