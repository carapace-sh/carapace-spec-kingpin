package spec

import (
	"fmt"

	"github.com/alecthomas/kingpin/v2"
	"github.com/carapace-sh/carapace-spec/pkg/command"
	"gopkg.in/yaml.v3"
)

func Register(app *kingpin.Application) {
	cmd := app.GetCommand("_carapace")
	if cmd == nil {
		cmd = app.Command("_carapace", "")
		cmd.Hidden()
	}

	specCmd := cmd.Command("spec", "")
	specCmd.Action(func(pc *kingpin.ParseContext) error {
		m, err := yaml.Marshal(Command(app))
		if err != nil {
			panic(err.Error())
		}
		fmt.Println(string(m))
		return nil
	})
}

func Command(app *kingpin.Application) command.Command {
	return scrape(&kingpin.CmdModel{
		Name:           app.Name,
		Help:           app.Help,
		FlagGroupModel: app.Model().FlagGroupModel,
		CmdGroupModel:  app.Model().CmdGroupModel,
	}, true)
}

func scrape(c *kingpin.CmdModel, root bool) command.Command {
	cmd := command.Command{
		Name:        c.Name,
		Aliases:     c.Aliases,
		Description: c.Help,
		Hidden:      c.Hidden,
		Commands:    make([]command.Command, 0),
	}
	cmd.Completion.Flag = make(map[string][]string)

	// TODO groups

	for _, flag := range c.Flags {
		f := command.Flag{
			Longhand:   "--" + flag.Name,
			Value:      !flag.IsBoolFlag(),
			Usage:      flag.Help,
			Hidden:     flag.Hidden,
			Required:   flag.Required,
			Persistent: root,
		}

		if flag.Short != 0 {
			f.Shorthand = "-" + string(flag.Short)
		}

		cmd.AddFlag(f)

		if flag.IsBoolFlag() {
			f.Longhand = "--no-" + flag.Name
			f.Shorthand = ""
			f.Hidden = true
			cmd.AddFlag(f)
		}
	}

	for _, subcmd := range c.Commands {
		if subcmd.Name != "_carapace" {
			cmd.Commands = append(cmd.Commands, scrape(subcmd, false))
		}
	}
	return cmd
}
