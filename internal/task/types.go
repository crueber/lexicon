package task

import "context"

// Task type constants.
const (
	TypeLibraryScan           = "LIBRARY_SCAN"
	TypeMetadataRefresh       = "METADATA_REFRESH"
	TypeCoverRefresh          = "COVER_REFRESH"
	TypeBookdropScan          = "BOOKDROP_SCAN"
	TypeDuplicateDetection    = "DUPLICATE_DETECTION"
	TypeRecommendationRebuild = "RECOMMENDATION_REBUILD"
	TypeFileOrganization      = "FILE_ORGANIZATION"
	TypeAuditLogCleanup       = "AUDIT_LOG_CLEANUP"
)

// Task status constants.
const (
	StatusQueued    = "QUEUED"
	StatusRunning   = "RUNNING"
	StatusCompleted = "COMPLETED"
	StatusFailed    = "FAILED"
	StatusCancelled = "CANCELLED"
)

// TaskFunc is the function signature for task implementations.
type TaskFunc func(ctx context.Context, payload string, reporter Reporter) error

// Reporter allows a task to report progress.
type Reporter interface {
	Progress(current, total int, message string)
}
