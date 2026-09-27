package firewall

import (
	"errors"
	"strings"
	"testing"
)

func TestFirewallRuleIsNarrowTCP(t *testing.T) {
	args := strings.Join(addArgs(47821), " ")
	for _, want := range []string{"protocol=TCP", "localport=47821", "profile=private", "dir=in", "action=allow", "name=BEAM"} {
		if !strings.Contains(args, want) {
			t.Fatalf("rule %q missing %s", args, want)
		}
	}
	banned := []string{"schtasks", "powershell", "wscript", "defender", "exclusion", "profile=any", "protocol=UDP"}
	for _, word := range banned {
		if strings.Contains(strings.ToLower(args), word) {
			t.Fatalf("rule %q contains %s", args, word)
		}
	}
}

func TestExistingRuleIsNotRecreated(t *testing.T) {
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	calls := []string{}
	run = func(args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		if strings.Contains(calls[len(calls)-1], "show") {
			return []byte("Rule Name: BEAM\n"), nil
		}
		return nil, errors.New("add should not run")
	}
	t.Cleanup(func() { run = platformRun })
	if err := apply(47821); err != nil {
		t.Fatal(err)
	}
	if err := apply(47821); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || !strings.Contains(calls[0], "show") {
		t.Fatalf("calls=%v", calls)
	}
}

func TestMissingRuleIsAddedOnce(t *testing.T) {
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	calls := 0
	run = func(args ...string) ([]byte, error) {
		calls++
		if strings.Contains(strings.Join(args, " "), "show") {
			return []byte("No rules match the specified criteria."), nil
		}
		return []byte("Ok."), nil
	}
	t.Cleanup(func() { run = platformRun })
	if err := apply(47821); err != nil {
		t.Fatal(err)
	}
	if err := apply(47821); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("netsh calls=%d, want show+add once", calls)
	}
}

func TestElevationIsOneTime(t *testing.T) {
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	run = func(args ...string) ([]byte, error) {
		if strings.Contains(strings.Join(args, " "), "show") {
			return []byte("No rules match the specified criteria."), nil
		}
		return []byte("Access is denied."), errors.New("Access is denied.")
	}
	t.Cleanup(func() { run = platformRun })
	if err := apply(47821); !errors.Is(err, ErrNeedsElevation) {
		t.Fatalf("first err=%v", err)
	}
	if err := RememberAsked(); err != nil {
		t.Fatal(err)
	}
	if err := apply(47821); err != nil {
		t.Fatalf("second prompt err=%v", err)
	}
}
