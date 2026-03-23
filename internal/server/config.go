package server

// Config holds all server configuration, parsed from environment variables.
type Config struct {
	Port      int    `env:"PORT" envDefault:"6060"`
	DataDir   string `env:"DATA_DIR" envDefault:"/app/data"`
	JWTSecret string `env:"JWT_SECRET"`
	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
	LogFormat string `env:"LOG_FORMAT" envDefault:"json"`
	DevMode   bool   `env:"DEV_MODE" envDefault:"false"`
}
