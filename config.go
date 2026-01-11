package main

import (
	"flag"
	"log"
	"os"

	"github.com/lib/pq"
	"github.com/megashchik/migrate/cmd"
	"github.com/megashchik/migrate/config"
)

const (
	CommandUp      = "up"
	CommandNew     = "new"
	CommandList    = "list"
	CommandLast    = "last"
	CommandHelp    = "help"
	CommandVersion = "version"
)

// initConfig returns the application configuration.
func initConfig() *config.Config {
	c := &config.Config{}

	flag.StringVar(&c.Conn, "conn", "", "")
	flag.StringVar(&c.Table, "t", "schema_migrations", "")
	flag.StringVar(&c.Dir, "dir", "./migrations", "")
	flag.BoolVar(&c.Extra, "extra", false, "")

	flag.BoolVar(&c.Short, "short", false, "")
	flag.BoolVar(&c.Desc, "desc", false, "")
	flag.StringVar(&c.Schema, "schema", "public", "")
	flag.StringVar(&c.EnvURL, "env-url", "DATABASE_URL", "")
	flag.StringVar(&c.Format, "f", "T", "")

	flag.Usage = func() {
		cmd.Help(c)
	}

	flag.Parse()

	c.Table = pq.QuoteIdentifier(c.Table)
	c.Schema = pq.QuoteIdentifier(c.Schema)
	c.FullTableName = c.Schema + "." + c.Table

	if c.Conn == "" {
		c.Conn = os.Getenv(c.EnvURL)
	}

	switch args := flag.Args(); len(args) {
	case 0:
	case 1:
		c.Command = args[0]
	case 2:
		c.Command = args[0]
		c.CommandArg = args[1]
	default:
		log.Println("many args, see help")

		c.Command = args[0]
		c.CommandArg = args[1]
	}

	return c
}
