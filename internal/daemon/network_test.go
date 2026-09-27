package daemon

import "testing"

func TestSameAddressSet(t *testing.T) {
	if !sameAddressSet([]string{"10.0.0.2", "192.168.1.8"}, []string{"192.168.1.8", "10.0.0.2"}) {
		t.Fatal("order changed but the address set did not")
	}
	if sameAddressSet([]string{"192.168.1.8"}, []string{"10.0.0.9"}) {
		t.Fatal("a new address was treated as unchanged")
	}
	if sameAddressSet([]string{"192.168.1.8"}, nil) {
		t.Fatal("missing addresses were treated as unchanged")
	}
}
