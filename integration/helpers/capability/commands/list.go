package commands

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/syndtr/gocapability/capability"
)

const LIST_SUBCOMMAND = "list"

type ListCommand struct {
	flagSet *flag.FlagSet
	filter  *string
}

func NewListCommand() *ListCommand {
	command := &ListCommand{}

	flagSet := flag.NewFlagSet(LIST_SUBCOMMAND, flag.ContinueOnError)
	filter := flagSet.String("filter", "all", "List (all/whitelist/blacklist) capabilities in linux")

	command.flagSet = flagSet
	command.filter = filter
	return command
}

func (cmd *ListCommand) PrintDefaults() {
	fmt.Println("List Command")
	fmt.Println("  Usage: capability list [ARGUMENTS]")
	cmd.flagSet.PrintDefaults()
}

func (cmd *ListCommand) Execute(args []string) {
	if !cmd.flagSet.Parsed() {
		if err := cmd.flagSet.Parse(args); err != nil {
			log.Fatal(fmt.Printf("Wrong command: %v", err))
		}
	}

	switch *cmd.filter {
	case "all":
		cmd.PrintAll()
		break
	case "whitelist":
		cmd.PrintWhitelist()
		break
	case "blacklist":
		cmd.PrintBlacklist()
		break
	default:
		log.Fatal(fmt.Sprintf("Wrong argument for LIST subcommand: %s", *cmd.filter))
	}
}

func (cmd *ListCommand) PrintAll() {
	cmd.PrintBlacklist()
	fmt.Println()
	cmd.PrintWhitelist()
}

func (cmd *ListCommand) PrintBlacklist() {
	fmt.Println("Blacklist capabilities\n")

	PrintCapability(capability.CAP_AUDIT_CONTROL)
	PrintCapability(capability.CAP_BLOCK_SUSPEND)
	PrintCapability(capability.CAP_CHOWN)
	PrintCapability(capability.CAP_DAC_READ_SEARCH)
	PrintCapability(capability.CAP_IPC_OWNER)
	PrintCapability(capability.CAP_LEASE)
	PrintCapability(capability.CAP_LINUX_IMMUTABLE)
	PrintCapability(capability.CAP_MAC_ADMIN)
	PrintCapability(capability.CAP_MAC_OVERRIDE)
	PrintCapability(capability.CAP_NET_ADMIN)
	PrintCapability(capability.CAP_NET_BROADCAST)
	PrintCapability(capability.CAP_SYS_ADMIN)
	PrintCapability(capability.CAP_SYS_BOOT)
	PrintCapability(capability.CAP_SYS_MODULE)
	PrintCapability(capability.CAP_SYS_NICE)
	PrintCapability(capability.CAP_SYS_PACCT)
	PrintCapability(capability.CAP_SYS_PTRACE)
	PrintCapability(capability.CAP_SYS_RAWIO)
	PrintCapability(capability.CAP_SYS_RESOURCE)
	PrintCapability(capability.CAP_SYS_TIME)
	PrintCapability(capability.CAP_SYS_TTY_CONFIG)
	PrintCapability(capability.CAP_SYSLOG)
	PrintCapability(capability.CAP_WAKE_ALARM)
}

func (cmd *ListCommand) PrintWhitelist() {
	fmt.Println("Whitelist capabilities\n")

	PrintCapability(capability.CAP_DAC_OVERRIDE)
	PrintCapability(capability.CAP_FSETID)
	PrintCapability(capability.CAP_FOWNER)
	PrintCapability(capability.CAP_MKNOD)
	PrintCapability(capability.CAP_NET_RAW)
	PrintCapability(capability.CAP_SETGID)
	PrintCapability(capability.CAP_SETUID)
	PrintCapability(capability.CAP_SETFCAP)
	PrintCapability(capability.CAP_SETPCAP)
	PrintCapability(capability.CAP_NET_BIND_SERVICE)
	PrintCapability(capability.CAP_SYS_CHROOT)
	PrintCapability(capability.CAP_KILL)
	PrintCapability(capability.CAP_AUDIT_WRITE)
}

func PrintCapability(capabilityFlag capability.Cap) {
	fmt.Printf("CAP_%s\n", strings.ToUpper(capabilityFlag.String()))
}
