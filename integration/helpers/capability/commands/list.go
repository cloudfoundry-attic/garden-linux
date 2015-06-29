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
}

func NewListCommand() *ListCommand {
	command := &ListCommand{}

	flagSet := flag.NewFlagSet(LIST_SUBCOMMAND, flag.ContinueOnError)
	command.flagSet = flagSet
	return command
}

func (cmd *ListCommand) PrintDefaults() {
	fmt.Println("List Command")
	fmt.Println("  Usage: capability list")
	cmd.flagSet.PrintDefaults()
}

func (cmd *ListCommand) Execute(args []string) {
	if !cmd.flagSet.Parsed() {
		if err := cmd.flagSet.Parse(args); err != nil {
			log.Fatal(fmt.Printf("Wrong command: %v", err))
		}
	}

	cmd.PrintAll()
}

func (cmd *ListCommand) PrintAll() {
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

	// Whitelist
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
