package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

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

// AgentSummary Agent summary information
type AgentSummary struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Color           string `json:"color"`
	CLIType         string `json:"cli_type"`
	WorkflowVersion int    `json:"workflow_version"`
}

// WorkflowStep Agent workflow step
type WorkflowStep struct {
	StepKey   string          `json:"step_key"`
	Name      string          `json:"name"`
	SortOrder int             `json:"sort_order"`
	CLIType   string          `json:"cli_type"`
	Prompt    string          `json:"prompt"`
	Documents []AgentDocument `json:"step_documents"`
}

// AgentDocument Agent custom document definition
type AgentDocument struct {
	ClientDocumentID string `json:"client_document_id"`
	ID               string `json:"id"`
	Key              string `json:"key"`
	Name             string `json:"name"`
	Placeholder      string `json:"placeholder"`
	TitleTemplate    string `json:"title_template"`
	DefaultContent   string `json:"default_content"`
}

// BuiltinPlaceholderDef Fixed built-in placeholder definition issued by the cloud.
// Key is the stable contract shared between the client and the cloud; Token is the placeholder text that actually appears in the prompt word (excluding curly brackets).
type BuiltinPlaceholderDef struct {
	Key   string `json:"key"`
	Token string `json:"token"`
}

// AgentSnapshot Agent complete snapshot (including workflow)
type AgentSnapshot struct {
	ID                  int64                   `json:"id"`
	Name                string                  `json:"name"`
	Color               string                  `json:"color"`
	CLIType             string                  `json:"cli_type"`
	WorkflowVersion     int                     `json:"workflow_version"`
	Documents           []AgentDocument         `json:"documents"`
	Steps               []WorkflowStep          `json:"steps"`
	BuiltinPlaceholders []BuiltinPlaceholderDef `json:"builtin_placeholders"`
}

//ExecutionAuthorization execution authorization information
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
		return result.Items, nil
	}
	return result.List, nil
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
	return &result, nil
}

// GetAgents gets the list of Agents available to the current user from the cloud
func (c *Client) GetAgents(ctx context.Context) ([]AgentSummary, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/client/agents", nil, &raw); err != nil {
		return nil, err
	}
	var direct []AgentSummary
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct, nil
	}
	var result struct {
		Items []AgentSummary `json:"items"`
		List  []AgentSummary `json:"list"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("解析 Agent 列表失败: %w", err)
	}
	if result.Items != nil {
		return result.Items, nil
	}
	return result.List, nil
}

// GetAgentSnapshot Gets the complete Agent snapshot from the cloud
func (c *Client) GetAgentSnapshot(ctx context.Context, agentID int64) (*AgentSnapshot, error) {
	path := fmt.Sprintf("/api/client/agents/%d/snapshot", agentID)
	var result struct {
		ID                  int64                   `json:"id"`
		Name                string                  `json:"name"`
		Color               string                  `json:"color"`
		CLIType             string                  `json:"cli_type"`
		WorkflowVersion     int                     `json:"workflow_version"`
		Agent               AgentSummary            `json:"agent"`
		Documents           []AgentDocument         `json:"documents"`
		BuiltinPlaceholders []BuiltinPlaceholderDef `json:"builtin_placeholders"`
		Steps               []struct {
			StepKey       string          `json:"step_key"`
			Name          string          `json:"name"`
			SortOrder     int             `json:"sort_order"`
			CLIType       string          `json:"cli_type"`
			Prompt        string          `json:"prompt"`
			PromptContent string          `json:"prompt_content"`
			StepDocuments []AgentDocument `json:"step_documents"`
		} `json:"steps"`
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	snapshot := &AgentSnapshot{
		ID:              result.ID,
		Name:            result.Name,
		Color:           result.Color,
		CLIType:         result.CLIType,
		WorkflowVersion: result.WorkflowVersion,
	}
	if result.Agent.ID != 0 {
		snapshot.ID = result.Agent.ID
		snapshot.Name = result.Agent.Name
		snapshot.Color = result.Agent.Color
		snapshot.CLIType = result.Agent.CLIType
		snapshot.WorkflowVersion = result.Agent.WorkflowVersion
	}
	snapshot.Documents = result.Documents
	snapshot.BuiltinPlaceholders = result.BuiltinPlaceholders
	for _, step := range result.Steps {
		prompt := step.Prompt
		if prompt == "" {
			prompt = step.PromptContent
		}
		cliType := snapshot.CLIType
		if cliType == "" {
			cliType = step.CLIType
		}
		snapshot.Steps = append(snapshot.Steps, WorkflowStep{
			StepKey:   step.StepKey,
			Name:      step.Name,
			SortOrder: step.SortOrder,
			CLIType:   cliType,
			Prompt:    prompt,
			Documents: step.StepDocuments,
		})
	}
	return snapshot, nil
}

// GetExecutionAuthorization checks Agent execution authorization
func (c *Client) GetExecutionAuthorization(ctx context.Context, agentID int64) (*ExecutionAuthorization, error) {
	path := fmt.Sprintf("/api/client/agents/%d/execution-authorization", agentID)
	var result ExecutionAuthorization
	if err := c.do(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
