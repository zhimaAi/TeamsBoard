package apimanager

import (
	"github.com/gin-gonic/gin"

	"goteams-client/internal/secrets"
	"goteams-client/internal/storage"
)

// RegisterRoutes register interface management route
func RegisterRoutes(r *gin.RouterGroup, dbRef *storage.DBRef, accountIDFn func() (string, error), secretStore secrets.Store) {
	h := NewHandler(dbRef, accountIDFn, secretStore)

	// Collection management (write operation verification goteams-api switch)
	collections := r.Group("/collections")
	{
		collections.GET("", h.ListCollections)
		collections.POST("", h.rejectCapabilityMutation, h.requireSkill, h.CreateCollection)
		collections.PUT("/:id", h.rejectCapabilityMutation, h.requireSkill, h.UpdateCollection)
		collections.DELETE("/:id", h.rejectCapabilityMutation, h.requireSkill, h.DeleteCollection)
	}

	// Directory within the collection
	folders := r.Group("/folders")
	{
		folders.GET("", h.ListFolders)
		folders.POST("", h.rejectCapabilityMutation, h.requireSkill, h.CreateFolder)
		folders.PUT("/reorder", h.rejectCapabilityMutation, h.requireSkill, h.ReorderFolders)
		folders.PUT("/:id", h.rejectCapabilityMutation, h.requireSkill, h.UpdateFolder)
		folders.DELETE("/:id", h.rejectCapabilityMutation, h.requireSkill, h.DeleteFolder)
	}

	// Request management (write operation verification goteams-api switch)
	requests := r.Group("/requests")
	{
		requests.GET("", h.ListRequests)
		requests.POST("/parse-curl", h.requireSkill, h.ParseCurl)
		requests.GET("/:id", h.GetRequest)
		requests.POST("", h.requireSkill, h.CreateRequest)
		requests.PUT("/:id", h.requireSkill, h.UpdateRequest)
		requests.DELETE("/:id", h.requireSkill, h.DeleteRequest)
		requests.POST("/:id/execute", h.requireSkill, h.ExecuteRequest)
		requests.GET("/:id/history", h.ListHistory)
		requests.DELETE("/:id/history", h.requireSkill, h.ClearHistory)
	}

	// Environment management (write operation verification goteams-api switch)
	environments := r.Group("/environments")
	{
		environments.GET("", h.ListEnvironments)
		environments.POST("", h.rejectCapabilityMutation, h.requireSkill, h.CreateEnvironment)
		environments.PUT("/:id", h.rejectCapabilityMutation, h.requireSkill, h.UpdateEnvironment)
		environments.DELETE("/:id", h.rejectCapabilityMutation, h.requireSkill, h.DeleteEnvironment)
	}
}
