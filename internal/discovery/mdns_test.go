package discovery

import (
	"testing"

	"github.com/grandcat/zeroconf"
)

func TestFromEntry(t *testing.T) {
	e := &zeroconf.ServiceEntry{}
	e.Instance = "MacBook"
	e.Port = 47821
	e.Text = []string{"id=abc", "name=MacBook", "type=mac", "proto=1", "pair=1"}
	e.AddrIPv4 = nil
	r, ok := fromEntry(e)
	if !ok || r.ID != "abc" || r.Name != "MacBook" || r.Type != "mac" || !r.Pair || r.Port != 47821 {
		t.Fatalf("%+v %v", r, ok)
	}
}
