package apimanager

import (
	"github.com/gin-gonic/gin"

	"goteams-client/internal/secrets"
	"goteams-client/internal/storage"
)

// RegisterRoutes register interface management route
func RegisterRoutes(r *gin.RouterGroup, dbRef *storage.DBRef, accountIDFn func() (string, error), secretStore secrets.Store) {
	h := NewHandler(dbRef, accountIDFn, secretStore)

	// Collection management
	collections := r.Group("/collections")
	{
		collections.GET("", h.ListCollections)
		collections.POST("", h.CreateCollection)
		collections.PUT("/:id", h.UpdateCollection)
		collections.DELETE("/:id", h.DeleteCollection)
	}

	// Directory within the collection
	folders := r.Group("/folders")
	{
		folders.GET("", h.ListFolders)
		folders.POST("", h.CreateFolder)
		folders.PUT("/reorder", h.ReorderFolders)
		folders.PUT("/:id", h.UpdateFolder)
		folders.DELETE("/:id", h.DeleteFolder)
	}

	// Request management
	requests := r.Group("/requests")
	{
		requests.GET("", h.ListRequests)
		requests.POST("/parse-curl", h.ParseCurl)
		requests.GET("/:id", h.GetRequest)
		requests.POST("", h.CreateRequest)
		requests.PUT("/:id", h.UpdateRequest)
		requests.DELETE("/:id", h.DeleteRequest)
		requests.POST("/:id/execute", h.ExecuteRequest)
		requests.GET("/:id/history", h.ListHistory)
		requests.DELETE("/:id/history", h.ClearHistory)
	}

	// Environment management
	environments := r.Group("/environments")
	{
		environments.GET("", h.ListEnvironments)
		environments.POST("", h.CreateEnvironment)
		environments.PUT("/:id", h.UpdateEnvironment)
		environments.DELETE("/:id", h.DeleteEnvironment)
	}
}
