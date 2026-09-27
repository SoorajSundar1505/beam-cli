package firewall

import (
	"errors"
	"strconv"
)

const ruleName = "BEAM"

var ErrNeedsElevation = errors.New("windows firewall setup needs permission once")

func showArgs() []string {
	return []string{"netsh", "advfirewall", "firewall", "show", "rule", "name=" + ruleName}
}

func addArgs(port int) []string {
	if port <= 0 {
		port = 47821
	}
	return []string{
		"netsh", "advfirewall", "firewall", "add", "rule",
		"name=" + ruleName,
		"dir=in",
		"action=allow",
		"protocol=TCP",
		"localport=" + strconv.Itoa(port),
		"profile=private",
		"enable=yes",
	}
}
