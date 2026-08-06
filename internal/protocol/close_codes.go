package protocol

// WebSocket close code definitions

// Standard close codes
const (
	CloseCodeNormal      = 1000 // Normal closure
	CloseCodePolicyError = 1008 // Generic permission or protocol policy error
	CloseCodeTooLarge    = 1009 // Message too large
	CloseCodeInternal    = 1011 // Cloud internal error
)

// Custom close codes
const (
	CloseCodeJWTExpired          = 4001 // JWT expired or invalid
	CloseCodeDeviceRevoked       = 4002 // Device revoked
	CloseCodeVersionUnsupported  = 4003 // Client version not supported
	CloseCodeProtocolUnsupported = 4004 // Protocol version not supported
	CloseCodeUserInactive        = 4005 // Current user invalidated or removed from tenant
	CloseCodeTenantMismatch      = 4006 // Tenant mismatch
)

// CloseCodeMessage maps close codes to user-facing messages.
// Close codes are sent by the cloud; this table is the sole source of client display text;
// unmatched codes are treated as "unknown close reason".
var CloseCodeMessage = map[int]string{
	CloseCodeNormal:              "正常关闭",
	CloseCodePolicyError:         "权限或协议策略错误",
	CloseCodeTooLarge:            "消息过大",
	CloseCodeInternal:            "服务端内部错误",
	CloseCodeJWTExpired:          "JWT 已过期或无效，请重新登录",
	CloseCodeDeviceRevoked:       "设备已撤销，请重新注册",
	CloseCodeVersionUnsupported:  "客户端版本不受支持，请重新下载",
	CloseCodeProtocolUnsupported: "协议版本不受支持，请重新下载",
	CloseCodeUserInactive:        "用户已失效或移出租户",
	CloseCodeTenantMismatch:      "租户不一致，安装包不匹配",
}
