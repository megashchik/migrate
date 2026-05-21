package config

const (
	CommandUp      = "up"
	CommandNew     = "new"
	CommandList    = "list"
	CommandLast    = "last"
	CommandHelp    = "help"
	CommandVersion = "version"
)

type Config struct {
	Conn  string
	Dir   string
	Extra bool

	// extra options
	Short         bool
	Desc          bool
	Ts            bool
	Table         string
	Schema        string
	FullTableName string
	EnvURL        string
	Format        string
	Command       string // Для хранения команды: list, last, new
	CommandArg    string // Для имени миграции в 'new <name>'
}
