package commands

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/cloudfoundry-incubator/garden-linux/integration/helpers/capability/inspector"
	"github.com/syndtr/gocapability/capability"
)

const INSPECT_SUBCOMMAND = "inspect"

type InspectCommand struct {
	flagSet *flag.FlagSet
	filter  *string
}

func NewInspectCommand() *InspectCommand {
	command := &InspectCommand{}

	flagSet := flag.NewFlagSet(INSPECT_SUBCOMMAND, flag.ContinueOnError)
	filter := flagSet.String("filter", "all", "List (all/whitelist/blacklist) capabilities in linux")

	command.flagSet = flagSet
	command.filter = filter
	return command
}

func (cmd *InspectCommand) PrintDefaults() {
	fmt.Println("Inspect Command")
	fmt.Println("  Usage: capability inspect [ARGUMENTS]")
	cmd.flagSet.PrintDefaults()
}

func (cmd *InspectCommand) Execute(args []string) {
	if !cmd.flagSet.Parsed() {
		if err := cmd.flagSet.Parse(args); err != nil {
			log.Fatal(fmt.Printf("Wrong command: %v", err))
		}
	}

	capabilities := capability.List()

	parseCapability := func(name string) *capability.Cap {
		for _, availableCap := range capabilities {
			prefixed := fmt.Sprintf("CAP_%s", strings.ToUpper(availableCap.String()))
			if strings.EqualFold(prefixed, name) {
				return &availableCap
			}
		}

		return nil
	}

	for _, capabilityFlag := range cmd.flagSet.Args() {
		probe := parseCapability(capabilityFlag)
		if probe == nil {
			fmt.Printf("Flag %q is not valid capability flag.\n", capabilityFlag)
			continue
		}

		fmt.Printf("Inspecting %v\n", capabilityFlag)
		switch *probe {
		case capability.CAP_SETUID:
			inspector.ProbeSETUID()
			break
		default:
			fmt.Printf("WARNING: Inspecting %q is not started. No implementation.\n", strings.ToUpper(probe.String()))
		}
	}
}
