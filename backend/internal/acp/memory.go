package acp

import (
	"strconv"
	"strings"
)

func lowAvailableMemory(meminfo string) bool {
	values := make(map[string]uint64, 2)
	for line := range strings.SplitSeq(meminfo, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[2] != "kB" || (fields[0] != "MemTotal:" && fields[0] != "MemAvailable:") {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return false
		}
		values[fields[0]] = value
	}
	available, ok := values["MemAvailable:"]
	return ok && values["MemTotal:"] > 0 && available < values["MemTotal:"]/10
}
