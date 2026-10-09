package knowledge

import (
	"github.com/gin-gonic/gin"

	"goteams-client/internal/storage"
)

// RegisterRoutes registers knowledge base routes.
// contentDirProvider resolves the KR (knowledge root directory) for the current Profile.
// defaultRootDir returns the default KR used by GetRoot for the breadcrumb default.
func RegisterRoutes(r *gin.RouterGroup, dbRef *storage.DBRef, contentDirProvider func() (string, error), defaultRootDir func() string) {
	h := NewHandler(dbRef, contentDirProvider, defaultRootDir)

	// KR config (BE-01 / DA-01 / IN-01)
	r.GET("/root", h.GetRoot)
	r.PUT("/root", h.PutRoot)
	r.POST("/scan", h.Scan)

	// Folder management
	folders := r.Group("/folders")
	{
		folders.GET("", h.ListFolders)
		folders.POST("", h.CreateFolder)
		folders.PUT("/:id", h.UpdateFolder)
		folders.DELETE("/:id", h.DeleteFolder)
	}

	// Document management
	documents := r.Group("/documents")
	{
		documents.GET("", h.ListDocuments)
		documents.GET("/:uuid", h.GetDocument)
		documents.POST("", h.CreateDocument)
		documents.PUT("/:uuid", h.UpdateDocument)
		documents.DELETE("/:uuid", h.DeleteDocument)
		documents.POST("/:uuid/restore", h.RestoreDocument)
		documents.DELETE("/:uuid/permanent", h.HardDeleteDocument)
		// Reference resolution (BE-06 / IN-01)
		documents.GET("/:uuid/path", h.DocumentPath)
	}

	// recycle bin
	r.GET("/trash", h.ListTrash)

	// Document history
	r.GET("/documents/:uuid/history", h.ListHistory)
	r.GET("/documents/:uuid/history/:historyId", h.GetHistory)

	// search
	r.GET("/search", h.Search)
}
