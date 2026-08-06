package protocol

// Error code definitions

const (
	// Request format error
	ErrCodeMessageInvalid = "MESSAGE_INVALID"

	// Message too large
	ErrCodePayloadTooLarge = "PAYLOAD_TOO_LARGE"

	// Task status not in the fixed enum
	ErrCodeTaskStatusInvalid = "TASK_STATUS_INVALID"

	// Normalized projection hash mismatch
	ErrCodeProjectionHashInvalid = "PROJECTION_HASH_INVALID"

	// Work item does not exist
	ErrCodeWorkItemNotFound = "WORK_ITEM_NOT_FOUND"

	// User not authorized to create this Agent task
	ErrCodeAgentForbidden = "AGENT_FORBIDDEN"

	// Snapshot violates an immutability constraint
	ErrCodeTaskSnapshotConflict = "TASK_SNAPSHOT_CONFLICT"

	// Rate limit exceeded
	ErrCodeRateLimited = "RATE_LIMITED"

	// JWT expired
	ErrCodeJWTExpired = "JWT_EXPIRED"

	// Device revoked
	ErrCodeDeviceRevoked = "DEVICE_REVOKED"

	// Client version incompatible
	ErrCodeClientUpgradeRequired = "CLIENT_UPGRADE_REQUIRED"

	// Server error
	ErrCodeInternalError = "INTERNAL_ERROR"

	// JWT invalid
	ErrCodeJWTInvalid = "JWT_INVALID"

	// Device credential invalid
	ErrCodeDeviceCredentialInvalid = "DEVICE_CREDENTIAL_INVALID"

	// JWT, device, or tenant mismatch
	ErrCodeTenantMismatch = "TENANT_MISMATCH"

	// Release version does not exist
	ErrCodeClientReleaseNotFound = "CLIENT_RELEASE_NOT_FOUND"

	// Protocol version not supported
	ErrCodeClientProtocolUnsupported = "CLIENT_PROTOCOL_UNSUPPORTED"

	// Request rate too high
	ErrCodeClientRateLimited = "CLIENT_RATE_LIMITED"
)

// ErrCodeRetryable reports whether an error code is retryable
var errCodeRetryable = map[string]bool{
	ErrCodeMessageInvalid:        false,
	ErrCodePayloadTooLarge:       false,
	ErrCodeTaskStatusInvalid:     false,
	ErrCodeProjectionHashInvalid: false,
	ErrCodeWorkItemNotFound:      false,
	ErrCodeAgentForbidden:        false,
	ErrCodeTaskSnapshotConflict:  false,
	ErrCodeRateLimited:           true, // Rate limiting is a transient fault; retry with backoff
	ErrCodeJWTExpired:            false,
	ErrCodeDeviceRevoked:         false,
	ErrCodeClientUpgradeRequired: false,
	ErrCodeInternalError:         true, // Server internal errors are usually retryable (transient fault)

	// The following error codes were newly added (previously missing, causing IsRetryable to return the zero value false)
	ErrCodeJWTInvalid:                false, // Auth failure, requires re-login, not retryable
	ErrCodeDeviceCredentialInvalid:   false, // Device credential invalid, not retryable
	ErrCodeTenantMismatch:            false, // Tenant mismatch, not retryable
	ErrCodeClientReleaseNotFound:     false, // Client version does not exist, requires upgrade, not retryable
	ErrCodeClientProtocolUnsupported: false, // Protocol not supported, not retryable
	ErrCodeClientRateLimited:         true,  // Client rate limited, retry with backoff
}

// IsRetryable reports whether an error code is retryable
func IsRetryable(code string) bool {
	return errCodeRetryable[code]
}

// ErrCodeKeepConnection reports whether an error code keeps the connection open
var errCodeKeepConnection = map[string]bool{
	ErrCodeMessageInvalid:        true,
	ErrCodePayloadTooLarge:       false,
	ErrCodeTaskStatusInvalid:     true,
	ErrCodeProjectionHashInvalid: true,
	ErrCodeWorkItemNotFound:      true,
	ErrCodeAgentForbidden:        true,
	ErrCodeTaskSnapshotConflict:  true,
	ErrCodeRateLimited:           true, // Rate limiting is transient; keep the connection and back off
	ErrCodeJWTExpired:            false,
	ErrCodeDeviceRevoked:         false,
	ErrCodeClientUpgradeRequired: false,
	ErrCodeInternalError:         true, // Internal errors keep the connection open to allow retry

	// The following error codes were newly added (previously missing, causing KeepConnection to fall through to the default branch)
	ErrCodeJWTInvalid:                false, // Auth failure, close connection and require re-login
	ErrCodeDeviceCredentialInvalid:   false, // Device credential invalid, close connection
	ErrCodeTenantMismatch:            false, // Tenant mismatch, close connection
	ErrCodeClientReleaseNotFound:     false, // Version missing, close connection and require upgrade
	ErrCodeClientProtocolUnsupported: false, // Protocol not supported, close connection
	ErrCodeClientRateLimited:         true,  // Client rate limited, keep connection and back off
}

// KeepConnection reports whether an error code should keep the connection open
func KeepConnection(code string) bool {
	keep, ok := errCodeKeepConnection[code]
	return ok && keep
}
