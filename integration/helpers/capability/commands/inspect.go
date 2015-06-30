package commands

import (
	"flag"
	"fmt"
	"log"
	"os/user"
	"strings"

	"github.com/cloudfoundry-incubator/garden-linux/integration/helpers/capability/inspector"
	"github.com/golang/go/src/pkg/strconv"
	"github.com/syndtr/gocapability/capability"
)

const INSPECT_SUBCOMMAND = "inspect"

type InspectCommand struct {
	flagSet *flag.FlagSet
}

func NewInspectCommand() *InspectCommand {
	command := &InspectCommand{}
	command.flagSet = flag.NewFlagSet(INSPECT_SUBCOMMAND, flag.ContinueOnError)
	return command
}

func (cmd *InspectCommand) PrintDefaults() {
	fmt.Println("Inspect Command")
	fmt.Println("  Usage: capability inspect [CAP_FLAGS]")
	cmd.flagSet.PrintDefaults()
}

func (cmd *InspectCommand) Execute(args []string) {
	if !cmd.flagSet.Parsed() {
		if err := cmd.flagSet.Parse(args); err != nil {
			log.Fatal(fmt.Printf("Wrong command: %v", err))
		}
	}

	nobody, err := user.Lookup("nobody")
	if err != nil {
		log.Fatal("The nobody user is missing from the root file system.")
	}

	uid, err := strconv.Atoi(nobody.Uid)
	if err != nil {
		log.Fatal("The nobody user id is not integer.")
	}

	gid, err := strconv.Atoi(nobody.Gid)
	if err != nil {
		log.Fatal("The nobody group id is not integer.")
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

	convert := func(flags []string) []capability.Cap {
		list := []capability.Cap{}
		for _, capabilityFlag := range flags {
			probe := parseCapability(capabilityFlag)
			if probe == nil {
				fmt.Printf("Flag %q is not valid capability flag.\n", capabilityFlag)
				continue
			}
			list = append(list, *probe)
		}
		return list
	}

	capabilityList := convert(cmd.flagSet.Args())

	if len(capabilityList) == 0 {
		capabilityList = capabilities
	}

	for _, probe := range capabilityList {
		fmt.Printf("Inspecting CAP_%v\n", probe.String())
		switch probe {
		case capability.CAP_SETUID:
			inspector.ProbeSETUID(uid, gid)
		case capability.CAP_SETGID:
			inspector.ProbeSETGID(uid, gid)
		case capability.CAP_CHOWN:
			inspector.ProbeCHOWN(uid, gid)
		default:
			fmt.Printf("WARNING: Inspecting %q is not started. No implementation.\n", strings.ToUpper(probe.String()))
		}
	}
}
