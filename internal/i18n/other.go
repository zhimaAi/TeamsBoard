package i18n

import "github.com/gin-gonic/gin"

// Errorf writes a localized error response with named template parameters.
func Errorf(c *gin.Context, status int, key string, params Params) {
	c.Header(headerContentLanguage, FromContext(c))
	c.AbortWithStatusJSON(status, gin.H{"error": Format(c, key, params)})
}
