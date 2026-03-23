package auth

// Principal represents an authenticated user in request context.
type Principal struct {
	UserID      int64
	Username    string
	Role        string // "ADMIN" or "USER"
	Permissions Permissions
	LibraryIDs  []int64
}

// IsAdmin returns true if the principal has the ADMIN role.
func (p *Principal) IsAdmin() bool {
	return p.Role == "ADMIN"
}

// Permissions holds per-feature permission flags.
type Permissions struct {
	CanDownload     bool
	CanUpload       bool
	CanEmailSend    bool
	CanEditMetadata bool
	OPDSAccess      bool
}
