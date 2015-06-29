package main

import (
	"fmt"
	"os"

	"strings"

	"github.com/cloudfoundry-incubator/garden-linux/integration/helpers/capcheck/commands"
)

const HELP_SUBCOMMAND = "help"

func main() {
	args := os.Args
	if len(args) == 2 && args[1] == "help" {
		fmt.Println("Capability Help:")
		fmt.Printf("  Usage: %s <CAPABILITY<,...>>\n", args[0])
		fmt.Printf("  Example: %s CAP_CHOWN,CAP_NET_BIND_SERVICE\n\n", args[0])
		return
	}

	caps := make(map[string]bool, 0)
	if len(args) == 2 {
		capsList := args[1]
		for _, cap := range strings.Split(capsList, ",") {
			caps[cap] = true
		}
	}

	commands.Inspect(caps)
}
