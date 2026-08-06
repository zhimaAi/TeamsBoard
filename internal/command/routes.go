package command

import (
	"github.com/gin-gonic/gin"

	"goteams-client/internal/secrets"
	"goteams-client/internal/storage"
)

// RegisterRoutes register command center routes
func RegisterRoutes(r *gin.RouterGroup, dbRef *storage.DBRef, secretStore secrets.Store) {
	h := NewHandler(dbRef, secretStore)

	// Git commands
	git := r.Group("/git")
	{
		git.GET("/projects", h.ListGitProjects)
		git.POST("/run", h.RunGitCommand)
	}

	// Docker command
	docker := r.Group("/docker")
	{
		docker.GET("/projects", h.ListDockerProjects)
		docker.POST("/run", h.RunDockerCommand)
	}

	//Command history
	history := r.Group("/history")
	{
		history.GET("", h.ListHistory)
		history.GET("/:id", h.GetHistoryDetail)
	}
}
