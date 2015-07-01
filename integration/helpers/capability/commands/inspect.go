package commands

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/cloudfoundry-incubator/garden-linux/integration/helpers/capability/inspector"
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
			fail("Wrong command: %v", err)
		}
	}

	const (
		user  = "vcap"
		group = "vcap"
	)

	uid, err := fetchUserAttribute(user, "u")

	if err != nil {
		fail("Getting uid for %s failed with error: %s", user, err)
	}

	gid, err := fetchUserAttribute(group, "g")
	if err != nil {
		fail("Getting gid for %s failed with error: %s", group, err)
	}

	capabilities := convert(cmd.flagSet.Args())

	var (
		probeResult inspector.ProbeResult
		resultSet   []inspector.ProbeResult
		statusCode  int
	)

	for _, probe := range capabilities {
		fmt.Printf("Inspecting CAP_%v\n", strings.ToUpper(probe.String()))

		switch probe {
		case capability.CAP_SETUID:
			probeResult = inspector.ProbeSETUID(uid, gid)
		case capability.CAP_SETGID:
			probeResult = inspector.ProbeSETGID(uid, gid)
		case capability.CAP_CHOWN:
			probeResult = inspector.ProbeCHOWN(uid, gid)
		default:
			fmt.Printf("WARNING: Inspecting %q is not started. No implementation.\n", strings.ToUpper(probe.String()))
		}

		if probeResult.Error != nil {
			resultSet = append(resultSet, probeResult)
		}
	}

	if len(resultSet) == 1 {
		statusCode = resultSet[0].StatusCode
	} else {
		statusCode = len(resultSet)
	}

	os.Exit(statusCode)
}

func fetchUserAttribute(user, attr string) (int, error) {
	output, err := exec.Command("id", fmt.Sprintf("-%s", attr), user).Output()
	if err != nil {
		return -1, err
	}

	text := string(output)
	text = strings.Trim(text, "\n")
	return strconv.Atoi(text)
}

func fail(text string, args ...interface{}) {
	fmt.Printf(text, args)
	os.Exit(1)
}

func convert(flags []string) []capability.Cap {
	capabilities := capability.List()
	list := []capability.Cap{}

	for _, capabilityFlag := range flags {
		probe := parseCapability(capabilityFlag, capabilities)
		if probe == nil {
			fmt.Printf("Flag %q is not valid capability flag.\n", capabilityFlag)
			continue
		}
		list = append(list, *probe)
	}

	if len(list) == 0 {
		list = capabilities
	}
	return list
}

func parseCapability(name string, capabilities []capability.Cap) *capability.Cap {
	for _, availableCap := range capabilities {
		prefixed := fmt.Sprintf("CAP_%s", strings.ToUpper(availableCap.String()))
		if strings.EqualFold(prefixed, name) {
			return &availableCap
		}
	}

	return nil
}
