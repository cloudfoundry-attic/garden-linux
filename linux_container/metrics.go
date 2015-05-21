package linux_container

import (
	"bufio"
	"strconv"
	"strings"

	"github.com/cloudfoundry-incubator/garden"
)

func (c *LinuxContainer) Metrics() (garden.Metrics, error) {
	cLog := c.logger.Session("metrics")

	diskStat, err := c.quotaManager.GetUsage(cLog, c.resources.UserUID)
	if err != nil {
		return garden.Metrics{}, err
	}

	cpuStat, err := c.cgroupsManager.Get("cpuacct", "cpuacct.stat")
	if err != nil {
		return garden.Metrics{}, err
	}

	cpuUsage, err := c.cgroupsManager.Get("cpuacct", "cpuacct.usage")
	if err != nil {
		return garden.Metrics{}, err
	}

	memoryStat, err := c.cgroupsManager.Get("memory", "memory.stat")
	if err != nil {
		return garden.Metrics{}, err
	}

	hostNetworkStat, err := c.NetworkStatisticser.Statistics()
	if err != nil {
		return garden.Metrics{}, err
	}

	// tx for host_intf is rx for cont_intf and vice-versa
	var contNetworkStat garden.ContainerNetworkStat
	contNetworkStat.RxBytes = hostNetworkStat.TxBytes
	contNetworkStat.TxBytes = hostNetworkStat.RxBytes

	return garden.Metrics{
		MemoryStat:  parseMemoryStat(memoryStat),
		CPUStat:     parseCPUStat(cpuUsage, cpuStat),
		DiskStat:    diskStat,
		NetworkStat: contNetworkStat,
	}, nil
}

func parseMemoryStat(contents string) (stat garden.ContainerMemoryStat) {
	scanner := bufio.NewScanner(strings.NewReader(contents))

	scanner.Split(bufio.ScanWords)

	for scanner.Scan() {
		field := scanner.Text()

		if !scanner.Scan() {
			break
		}

		value, err := strconv.ParseUint(scanner.Text(), 10, 0)
		if err != nil {
			continue
		}

		switch field {
		case "cache":
			stat.Cache = value
		case "rss":
			stat.Rss = value
		case "mapped_file":
			stat.MappedFile = value
		case "pgpgin":
			stat.Pgpgin = value
		case "pgpgout":
			stat.Pgpgout = value
		case "swap":
			stat.Swap = value
		case "pgfault":
			stat.Pgfault = value
		case "pgmajfault":
			stat.Pgmajfault = value
		case "inactive_anon":
			stat.InactiveAnon = value
		case "active_anon":
			stat.ActiveAnon = value
		case "inactive_file":
			stat.InactiveFile = value
		case "active_file":
			stat.ActiveFile = value
		case "unevictable":
			stat.Unevictable = value
		case "hierarchical_memory_limit":
			stat.HierarchicalMemoryLimit = value
		case "hierarchical_memsw_limit":
			stat.HierarchicalMemswLimit = value
		case "total_cache":
			stat.TotalCache = value
		case "total_rss":
			stat.TotalRss = value
		case "total_mapped_file":
			stat.TotalMappedFile = value
		case "total_pgpgin":
			stat.TotalPgpgin = value
		case "total_pgpgout":
			stat.TotalPgpgout = value
		case "total_swap":
			stat.TotalSwap = value
		case "total_pgfault":
			stat.TotalPgfault = value
		case "total_pgmajfault":
			stat.TotalPgmajfault = value
		case "total_inactive_anon":
			stat.TotalInactiveAnon = value
		case "total_active_anon":
			stat.TotalActiveAnon = value
		case "total_inactive_file":
			stat.TotalInactiveFile = value
		case "total_active_file":
			stat.TotalActiveFile = value
		case "total_unevictable":
			stat.TotalUnevictable = value
		}
	}

	stat.TotalUsageTowardLimit = stat.TotalRss + (stat.TotalCache - stat.TotalInactiveFile)

	return
}

func parseCPUStat(usage, statContents string) (stat garden.ContainerCPUStat) {
	cpuUsage, err := strconv.ParseUint(strings.Trim(usage, "\n"), 10, 0)
	if err != nil {
		return
	}

	stat.Usage = cpuUsage

	scanner := bufio.NewScanner(strings.NewReader(statContents))

	scanner.Split(bufio.ScanWords)

	for scanner.Scan() {
		field := scanner.Text()

		if !scanner.Scan() {
			break
		}

		value, err := strconv.ParseUint(scanner.Text(), 10, 0)
		if err != nil {
			continue
		}

		switch field {
		case "user":
			stat.User = value
		case "system":
			stat.System = value
		}
	}

	return
}
