package knowledge

import (
	"github.com/gin-gonic/gin"

	"goteams-client/internal/storage"
)

// RegisterRoutes registers knowledge base routes
// contentDirProvider is used to parse the knowledge base content directory according to the current login account (account directory: <accountDataDir>/knowledge)
func RegisterRoutes(r *gin.RouterGroup, dbRef *storage.DBRef, contentDirProvider func() (string, error)) {
	h := NewHandler(dbRef, contentDirProvider)

	//Folder management
	folders := r.Group("/folders")
	{
		folders.GET("", h.ListFolders)
		folders.POST("", h.CreateFolder)
		folders.PUT("/:id", h.UpdateFolder)
		folders.DELETE("/:id", h.DeleteFolder)
	}

	//Document management
	documents := r.Group("/documents")
	{
		documents.GET("", h.ListDocuments)
		documents.GET("/:uuid", h.GetDocument)
		documents.POST("", h.CreateDocument)
		documents.PUT("/:uuid", h.UpdateDocument)
		documents.DELETE("/:uuid", h.DeleteDocument)
		documents.POST("/:uuid/restore", h.RestoreDocument)
		documents.DELETE("/:uuid/permanent", h.HardDeleteDocument)
	}

	//recycle bin
	r.GET("/trash", h.ListTrash)

	// Document history
	r.GET("/documents/:uuid/history", h.ListHistory)
	r.GET("/documents/:uuid/history/:historyId", h.GetHistory)

	// search
	r.GET("/search", h.Search)
}
