package commands

import (
	"fmt"
	"os"

	"code.cloudfoundry.org/garden-linux/integration/helpers/capcheck/inspector"
)

func Inspect(caps map[string]bool) {

	var errors int

	probeAll := len(caps) == 0

	if probeAll || shouldProbe(caps, "CAP_SYS_ADMIN") {
		// Probe CAP_SYS_ADMIN because it is conditionally added to the whitelist.
		if probeError := inspector.ProbeCAP_SYS_ADMIN(); probeError != nil {
			errors++
		}
	}

	if probeAll || shouldProbe(caps, "CAP_MKNOD") {
		// Probe a capability not in the whitelist, e.g. CAP_MKNOD
		if probeError := inspector.ProbeCAP_MKNOD(); probeError != nil {
			errors++
		}
	}

	if probeAll || shouldProbe(caps, "CAP_NET_BIND_SERVICE") {
		// Probe a capability which is in the whitelist, e.g. CAP_NET_BIND_SERVICE
		if probeError := inspector.ProbeCAP_NET_BIND_SERVICE(); probeError != nil {
			errors++
		}
	}

	for cap := range caps {
		fmt.Printf("WARNING: %s is not supported and was not probed\n", cap)
	}

	os.Exit(errors)
}

func shouldProbe(caps map[string]bool, cap string) bool {
	result := caps[cap]
	delete(caps, cap)
	return result
}
