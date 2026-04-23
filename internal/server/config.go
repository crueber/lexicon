package server

// Config holds all server configuration, parsed from environment variables.
type Config struct {
	Port              int    `env:"PORT" envDefault:"6060"`
	DataDir           string `env:"DATA_DIR" envDefault:"/app/data"`
	JWTSecret         string `env:"JWT_SECRET"`
	LogLevel          string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat         string `env:"LOG_FORMAT" envDefault:"json"`
	DevMode           bool   `env:"DEV_MODE" envDefault:"false"`
	GoogleBooksAPIKey string `env:"GOOGLE_BOOKS_API_KEY" envDefault:""`
	HardcoverAPIKey   string `env:"HARDCOVER_API_KEY" envDefault:""`
	ComicVineAPIKey   string `env:"COMICVINE_API_KEY" envDefault:""`
	BookdropPath      string `env:"BOOKDROP_PATH" envDefault:"/bookdrop"`
	BookdropEnabled   bool   `env:"BOOKDROP_ENABLED" envDefault:"false"`

	// OIDC configuration.
	OIDCEnabled      bool   `env:"OIDC_ENABLED" envDefault:"false"`
	OIDCIssuerURI    string `env:"OIDC_ISSUER_URI" envDefault:""`
	OIDCClientID     string `env:"OIDC_CLIENT_ID" envDefault:""`
	OIDCClientSecret string `env:"OIDC_CLIENT_SECRET" envDefault:""`
	OIDCProviderName string `env:"OIDC_PROVIDER_NAME" envDefault:""`
	OIDCScope        string `env:"OIDC_SCOPE" envDefault:""`

	// Remote auth configuration.
	RemoteAuthEnabled      bool   `env:"REMOTE_AUTH_ENABLED" envDefault:"false"`
	RemoteAuthUserHeader   string `env:"REMOTE_AUTH_USER_HEADER" envDefault:""`
	RemoteAuthEmailHeader  string `env:"REMOTE_AUTH_EMAIL_HEADER" envDefault:""`
	RemoteAuthGroupsHeader string `env:"REMOTE_AUTH_GROUPS_HEADER" envDefault:""`
	RemoteAuthAutoCreate   bool   `env:"REMOTE_AUTH_AUTO_CREATE" envDefault:"false"`
}
