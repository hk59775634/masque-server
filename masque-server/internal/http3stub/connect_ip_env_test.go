package http3stub

import "testing"

func TestParseConnectIPEchoContextAllowlist(t *testing.T) {
	m := ParseConnectIPEchoContextAllowlist(" 2 , 4 , bogus ,0 , 6 ")
	if len(m) != 3 {
		t.Fatalf("len=%d %+v", len(m), m)
	}
	for _, id := range []uint64{2, 4, 6} {
		if _, ok := m[id]; !ok {
			t.Fatalf("missing %d", id)
		}
	}
}

func TestParseConnectIPEchoContextAllowlistEmpty(t *testing.T) {
	if ParseConnectIPEchoContextAllowlist("") != nil {
		t.Fatal()
	}
	if ParseConnectIPEchoContextAllowlist("0,0") != nil {
		t.Fatal()
	}
}
