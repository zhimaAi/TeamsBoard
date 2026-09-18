package localserver

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"goteams-client/internal/expertgroup"
	"goteams-client/internal/i18n"
	"goteams-client/internal/pipeline"
)

type ExpertGroupsHandler struct {
	db        func() *sql.DB
	iconStore *IconStore
}

func NewExpertGroupsHandler(db func() *sql.DB, iconStore *IconStore) *ExpertGroupsHandler {
	return &ExpertGroupsHandler{db: db, iconStore: iconStore}
}

func (h *ExpertGroupsHandler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("", h.list)
	r.POST("", h.create)
	r.GET("/reusable-agents", h.listReusableAgents)
	r.GET("/:uuid", h.get)
	r.POST("/:uuid/copy", h.copy)
	r.PUT("/:uuid", h.update)
	r.DELETE("/:uuid", h.delete)
	r.POST("/:uuid/members", h.addMember)
	r.PUT("/:uuid/members/:member_uuid", h.updateMember)
	r.DELETE("/:uuid/members/:member_uuid", h.deleteMember)
}

func (h *ExpertGroupsHandler) service(c *gin.Context) (*expertgroup.Service, bool) {
	if h.db == nil || h.db() == nil {
		i18n.LocalServerError(c, http.StatusServiceUnavailable, errors.New("本地任务数据库尚未就绪"))
		return nil, false
	}
	return expertgroup.NewService(h.db()), true
}

func (h *ExpertGroupsHandler) list(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	items, err := svc.List(c.Request.Context())
	if err != nil {
		expertGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *ExpertGroupsHandler) get(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	item, err := svc.Get(c.Request.Context(), c.Param("uuid"))
	if err != nil {
		expertGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *ExpertGroupsHandler) create(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	input, uploaded, err := h.bindGroupInput(c)
	if err != nil {
		expertGroupError(c, err)
		return
	}
	item, err := svc.Create(c.Request.Context(), input)
	if err != nil {
		if uploaded != "" {
			_ = h.iconStore.Remove(uploaded)
		}
		expertGroupError(c, err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *ExpertGroupsHandler) update(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	input, uploaded, err := h.bindGroupInput(c)
	if err != nil {
		expertGroupError(c, err)
		return
	}
	item, err := svc.Update(c.Request.Context(), c.Param("uuid"), input)
	if err != nil {
		if uploaded != "" {
			_ = h.iconStore.Remove(uploaded)
		}
		expertGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *ExpertGroupsHandler) copy(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	item, err := svc.Copy(c.Request.Context(), c.Param("uuid"))
	if err != nil {
		expertGroupError(c, err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *ExpertGroupsHandler) delete(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	if err := svc.Delete(c.Request.Context(), c.Param("uuid")); err != nil {
		expertGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (h *ExpertGroupsHandler) addMember(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	input, uploaded, err := h.bindMemberInput(c)
	if err != nil {
		expertGroupError(c, err)
		return
	}
	item, err := svc.AddMember(c.Request.Context(), c.Param("uuid"), input)
	if err != nil {
		if uploaded != "" {
			_ = h.iconStore.Remove(uploaded)
		}
		expertGroupError(c, err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *ExpertGroupsHandler) updateMember(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	input, uploaded, err := h.bindMemberInput(c)
	if err != nil {
		expertGroupError(c, err)
		return
	}
	item, err := svc.UpdateMember(c.Request.Context(), c.Param("uuid"), c.Param("member_uuid"), input)
	if err != nil {
		if uploaded != "" {
			_ = h.iconStore.Remove(uploaded)
		}
		expertGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *ExpertGroupsHandler) deleteMember(c *gin.Context) {
	svc, ok := h.service(c)
	if !ok {
		return
	}
	if err := svc.DeleteMember(c.Request.Context(), c.Param("uuid"), c.Param("member_uuid")); err != nil {
		expertGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (h *ExpertGroupsHandler) listReusableAgents(c *gin.Context) {
	if h.db == nil || h.db() == nil {
		i18n.LocalServerError(c, http.StatusServiceUnavailable, errors.New("本地任务数据库尚未就绪"))
		return
	}
	type reusableAgent struct {
		UUID        string `json:"uuid"`
		Source      string `json:"source"`
		SourceName  string `json:"source_name"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Avatar      string `json:"avatar"`
		Prompt      string `json:"prompt"`
		CLIType     string `json:"cli_type"`
		ModelName   string `json:"model_name"`
	}
	items := make([]reusableAgent, 0)
	pipelines, err := pipeline.NewService(h.db()).List(c.Request.Context())
	if err != nil {
		expertGroupError(c, err)
		return
	}
	for _, item := range pipelines {
		for _, step := range item.Steps {
			items = append(items, reusableAgent{UUID: step.UUID, Source: "pipeline", SourceName: item.Name,
				Name: step.Name, Description: step.Description, Avatar: step.Avatar, Prompt: step.Prompt,
				CLIType: step.CLIType, ModelName: step.ModelName})
		}
	}
	groups, err := expertgroup.NewService(h.db()).List(c.Request.Context())
	if err != nil {
		expertGroupError(c, err)
		return
	}
	for _, group := range groups {
		members := append([]expertgroup.Member{}, group.Members...)
		if group.Leader != nil {
			members = append([]expertgroup.Member{*group.Leader}, members...)
		}
		for _, member := range members {
			items = append(items, reusableAgent{UUID: member.UUID, Source: "expert_group", SourceName: group.Name,
				Name: member.Name, Description: member.Description, Avatar: member.Avatar, Prompt: member.Prompt,
				CLIType: member.CLIType, ModelName: member.ModelName})
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *ExpertGroupsHandler) bindGroupInput(c *gin.Context) (expertgroup.GroupInput, string, error) {
	var input expertgroup.GroupInput
	if !isMultipartRequest(c) {
		return input, "", c.ShouldBindJSON(&input)
	}
	input = expertgroup.GroupInput{Name: c.PostForm("name"), Description: c.PostForm("description"), Avatar: c.PostForm("avatar")}
	avatar, err := h.saveOptionalIcon(c)
	if err != nil {
		return input, "", err
	}
	if avatar != "" {
		input.Avatar = avatar
	}
	return input, avatar, nil
}

func (h *ExpertGroupsHandler) bindMemberInput(c *gin.Context) (expertgroup.MemberInput, string, error) {
	var input expertgroup.MemberInput
	if !isMultipartRequest(c) {
		return input, "", c.ShouldBindJSON(&input)
	}
	input = expertgroup.MemberInput{MemberRole: c.PostForm("member_role"), Name: c.PostForm("name"),
		Description: c.PostForm("description"), Avatar: c.PostForm("avatar"), Prompt: c.PostForm("prompt"),
		CLIType: c.PostForm("cli_type"), ModelName: c.PostForm("model_name")}
	avatar, err := h.saveOptionalIcon(c)
	if err != nil {
		return input, "", err
	}
	if avatar != "" {
		input.Avatar = avatar
	}
	return input, avatar, nil
}

func (h *ExpertGroupsHandler) saveOptionalIcon(c *gin.Context) (string, error) {
	if h.iconStore == nil {
		return "", fmt.Errorf("本地头像存储尚未初始化")
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxIconUploadRequestSize)
	header, err := c.FormFile("avatar_file")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return "", nil
		}
		return "", err
	}
	return h.iconStore.Save(header)
}

func expertGroupError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, expertgroup.ErrNotFound) || errors.Is(err, expertgroup.ErrMemberNotFound) {
		status = http.StatusNotFound
	} else if strings.Contains(strings.ToLower(err.Error()), "unique") {
		status = http.StatusConflict
	}
	i18n.LocalServerError(c, status, err)
}
