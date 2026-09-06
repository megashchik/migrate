package main

import (
	"log"

	_ "github.com/lib/pq"
	"github.com/megashchik/migrate/cmd"
	"github.com/megashchik/migrate/config"
)

func main() {
	c := initConfig()

	var err error

	switch c.Command {
	case config.CommandNew:
		err = cmd.New(c)
	case config.CommandList:
		err = cmd.List(c)
	case config.CommandLast:
		err = cmd.Last(c)
	case config.CommandCheck:
		err = cmd.Check(c)
	case config.CommandUp:
		err = cmd.Up(c)
	case config.CommandHelp:
		cmd.Help(c)
	case config.CommandVersion:
		cmd.Version()
	default:
		err = cmd.Up(c)
	}

	if err != nil {
		log.Fatal(err)
	}
}
