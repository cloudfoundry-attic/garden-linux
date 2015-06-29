package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cloudfoundry-incubator/garden-linux/integration/helpers/capability/commands"
)

const HELP_SUBCOMMAND = "help"

func main() {
	flag.NewFlagSet(HELP_SUBCOMMAND, flag.ContinueOnError)
	listCommand := commands.NewListCommand()
	inspectCommand := commands.NewInspectCommand()

	args := os.Args
	if len(args) == 1 {
		args = append(args, HELP_SUBCOMMAND)
	}

	switch args[1] {
	case HELP_SUBCOMMAND:
		fmt.Println("Capability Help")
		fmt.Println("  Usage: capability [SUBCOMMAND] [ARGUMENTS]\n")

		listCommand.PrintDefaults()
		fmt.Println()
		inspectCommand.PrintDefaults()
	case commands.LIST_SUBCOMMAND:
		listCommand.Execute(args[2:])
	case commands.INSPECT_SUBCOMMAND:
		inspectCommand.Execute(args[2:])
	}
}
