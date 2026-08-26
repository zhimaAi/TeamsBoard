package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// uploadAssetRe matches src=/href= attributes whose value points to a cloud
// relative upload path (e.g. /uploads/1/requirement/xxx.png). Such URLs cannot
// be rendered by the local client because they are relative to the cloud site
// root; they must be prefixed with the cloud origin (scheme://host).
var uploadAssetRe = regexp.MustCompile(`(?i)(src|href)\s*=\s*("|')(/uploads[^"']*)("|')`)

// WorkItem cloud work item
type WorkItem struct {
	Type          string `json:"type"` // requirement | defect
	ID            int64  `json:"id"`
	WorkspaceID   int64  `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"` // The name of the project (workspace) it belongs to, the cloud has returned it in the work-items interface
	Title         string `json:"title"`
	Description   string `json:"description"`
}

// MyWorkResult Cloud "My Work" list. Fields are determined by cloud work item configuration and are retained using map
// Complete response such as status, priority, handler, etc. to prevent the client agent from losing extended fields.
type MyWorkResult struct {
	List []map[string]interface{} `json:"list"`
}

// PipelineSummary Pipeline summary information
type PipelineSummary struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Color   string `json:"color"`
	CLIType string `json:"cli_type"`
	Icon    string `json:"icon"` // Cloud avatar, a relative path like /uploads/...
}

// PipelineStep Pipeline workflow step
type PipelineStep struct {
	StepKey   string `json:"step_key"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
	CLIType   string `json:"cli_type"`
	Prompt    string `json:"prompt"`
}

// PipelineSnapshot Pipeline complete snapshot (including workflow)
type PipelineSnapshot struct {
	ID      int64          `json:"id"`
	Name    string         `json:"name"`
	Color   string         `json:"color"`
	CLIType string         `json:"cli_type"`
	Icon    string         `json:"icon"` // Cloud avatar, a relative path like /uploads/...
	Steps   []PipelineStep `json:"steps"`
}

// ExecutionAuthorization execution authorization information
type ExecutionAuthorization struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

// GetWorkItems Gets the list of importable work items from the cloud
func (c *Client) GetWorkItems(ctx context.Context) ([]WorkItem, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/client/work-items", nil, &raw); err != nil {
		return nil, err
	}
	var direct []WorkItem
	if err := json.Unmarshal(raw, &direct); err == nil {
		c.normalizeWorkItems(direct)
		return direct, nil
	}
	var result struct {
		Items []WorkItem `json:"items"`
		List  []WorkItem `json:"list"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("解析工作项列表失败: %w", err)
	}
	if result.Items != nil {
		c.normalizeWorkItems(result.Items)
		return result.Items, nil
	}
	c.normalizeWorkItems(result.List)
	return result.List, nil
}

// contentOrigin returns the cloud site origin (scheme://host) derived from the
// configured API base URL. Upload assets are served from the cloud site root,
// so a relative /uploads/... URL must be prefixed with this origin to be
// reachable from the local client.
func (c *Client) contentOrigin() string {
	u, err := url.Parse(c.baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// normalizeWorkItems rewrites cloud-relative upload asset URLs inside each work
// item description into absolute http/https URLs so they can be rendered by the
// local client. Already-absolute URLs are left untouched (idempotent).
func (c *Client) normalizeWorkItems(items []WorkItem) {
	for i := range items {
		items[i].Description = c.normalizeWorkItemContent(items[i].Description)
	}
}

// normalizeWorkItemContent rewrites relative /uploads/... asset URLs (in src or
// href attributes) in the given HTML fragment into absolute http/https URLs
// using the cloud origin.
func (c *Client) normalizeWorkItemContent(description string) string {
	origin := c.contentOrigin()
	if origin == "" || description == "" {
		return description
	}
	// Escape "$" in the origin so it is not treated as a group reference in the
	// replacement template.
	escaped := strings.ReplaceAll(origin, "$", "$$")
	return uploadAssetRe.ReplaceAllString(description, `${1}=${2}`+escaped+`${3}${4}`)
}

// normalizeAssetURL rewrites a cloud-relative asset URL (e.g. the pipeline/step
// avatar returned as "/uploads/1/pipeline/xxx.png") into an absolute http/https
// URL using the cloud site origin, so it can be rendered by the local client.
// It mirrors normalizeWorkItemContent but operates on a plain URL string instead
// of an HTML fragment. Empty values and already-absolute URLs are returned
// unchanged, so the call is idempotent and safe to apply repeatedly.
func (c *Client) normalizeAssetURL(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}
	origin := c.contentOrigin()
	if origin == "" {
		return rawURL
	}
	if strings.HasPrefix(rawURL, "/uploads") {
		return origin + rawURL
	}
	return rawURL
}

// GetMyWork Gets all the work items that the currently logged in user is responsible for.
func (c *Client) GetMyWork(ctx context.Context) (*MyWorkResult, error) {
	var result MyWorkResult
	if err := c.do(ctx, http.MethodGet, "/api/my-work", nil, &result); err != nil {
		return nil, err
	}
	if result.List == nil {
		result.List = []map[string]interface{}{}
	}
	c.normalizeMyWork(result.List)
	return &result, nil
}

// normalizeMyWork rewrites cloud-relative upload asset URLs inside each work
// item's description (stored as a generic map to preserve extended fields) into
// absolute http/https URLs, mirroring the normalization applied to GetWorkItems.
func (c *Client) normalizeMyWork(list []map[string]interface{}) {
	for _, item := range list {
		if item == nil {
			continue
		}
		if desc, ok := item["description"].(string); ok && desc != "" {
			item["description"] = c.normalizeWorkItemContent(desc)
		}
	}
}

// GetPipelines gets the list of pipelines available to the current user from the cloud
func (c *Client) GetPipelines(ctx context.Context) ([]PipelineSummary, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/client/pipelines", nil, &raw); err != nil {
		return nil, err
	}
	var direct []PipelineSummary
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct, nil
	}
	var result struct {
		Items []PipelineSummary `json:"items"`
		List  []PipelineSummary `json:"list"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("解析流水线列表失败: %w", err)
	}
	if result.Items != nil {
		return result.Items, nil
	}
	return result.List, nil
}

// GetPipelineSnapshot Gets the complete pipeline snapshot from the cloud
func (c *Client) GetPipelineSnapshot(ctx context.Context, pipelineID int64) (*PipelineSnapshot, error) {
	path := fmt.Sprintf("/api/client/pipelines/%d/snapshot", pipelineID)
	var result struct {
		ID       int64           `json:"id"`
		Name     string          `json:"name"`
		Color    string          `json:"color"`
		CLIType  string          `json:"cli_type"`
		Icon     string          `json:"icon"`
		Pipeline PipelineSummary `json:"pipeline"`
		Steps    []struct {
			StepKey       string `json:"step_key"`
			Name          string `json:"name"`
			SortOrder     int    `json:"sort_order"`
			CLIType       string `json:"cli_type"`
			Prompt        string `json:"prompt"`
			PromptContent string `json:"prompt_content"`
		} `json:"steps"`
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	snapshot := &PipelineSnapshot{
		ID:      result.ID,
		Name:    result.Name,
		Color:   result.Color,
		CLIType: result.CLIType,
		Icon:    c.normalizeAssetURL(result.Icon),
	}
	if result.Pipeline.ID != 0 {
		snapshot.ID = result.Pipeline.ID
		snapshot.Name = result.Pipeline.Name
		snapshot.Color = result.Pipeline.Color
		snapshot.CLIType = result.Pipeline.CLIType
		snapshot.Icon = c.normalizeAssetURL(result.Pipeline.Icon)
	}
	for _, step := range result.Steps {
		prompt := step.Prompt
		if prompt == "" {
			prompt = step.PromptContent
		}
		cliType := snapshot.CLIType
		if cliType == "" {
			cliType = step.CLIType
		}
		snapshot.Steps = append(snapshot.Steps, PipelineStep{
			StepKey:   step.StepKey,
			Name:      step.Name,
			SortOrder: step.SortOrder,
			CLIType:   cliType,
			Prompt:    prompt,
		})
	}
	return snapshot, nil
}

// GetPipelineExecutionAuthorization checks pipeline execution authorization
func (c *Client) GetPipelineExecutionAuthorization(ctx context.Context, pipelineID int64) (*ExecutionAuthorization, error) {
	path := fmt.Sprintf("/api/client/pipelines/%d/execution-authorization", pipelineID)
	var result ExecutionAuthorization
	if err := c.do(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
