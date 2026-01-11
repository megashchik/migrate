package main

import (
	"log"

	_ "github.com/lib/pq"
	"github.com/megashchik/migrate/cmd"
)

func main() {
	c := initConfig()

	var err error

	switch c.Command {
	case CommandNew:
		err = cmd.New(c)
	case CommandList:
		err = cmd.List(c)
	case CommandLast:
		err = cmd.Last(c)
	case CommandUp:
		err = cmd.Up(c)
	case CommandHelp:
		cmd.Help(c)
	case CommandVersion:
		cmd.Version()
	default:
		err = cmd.Up(c)
	}

	if err != nil {
		log.Fatal(err)
	}
}
