package apimanager

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"goteams-client/internal/capability"
)

func capabilityGrant(c *gin.Context) (capability.Grant, bool) {
	value, ok := c.Get(capability.GinContextGrantKey)
	if !ok {
		return capability.Grant{}, false
	}
	grant, ok := value.(capability.Grant)
	return grant, ok
}

func requireRequestInCapabilityScope(c *gin.Context, collectionID, folderID int64) bool {
	grant, scoped := capabilityGrant(c)
	if !scoped {
		return true
	}
	if grant.APICollectionID == collectionID && grant.APIFolderID == folderID {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "接口不在当前任务授权的集合和文件夹内"})
	return false
}

func (h *Handler) rejectCapabilityMutation(c *gin.Context) {
	if _, scoped := capabilityGrant(c); scoped {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "任务 Skill 无权修改集合、文件夹或环境"})
		return
	}
	c.Next()
}
