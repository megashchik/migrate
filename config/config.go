package config

type Config struct {
	Conn  string
	Dir   string
	Extra bool

	// extra options
	Short         bool
	Table         string
	Desc          bool
	Schema        string
	FullTableName string
	EnvURL        string
	Format        string
	Command       string // Для хранения команды: list, last, new
	CommandArg    string // Для имени миграции в 'new <name>'
}
