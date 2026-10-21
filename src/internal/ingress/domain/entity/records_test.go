package entity

import (
	"reflect"
	"testing"
)

func TestRecords(t *testing.T) {
	for _, c := range []struct {
		name       string
		ipv4, ipv6 string
		want       []DNSRecord
	}{
		{"A y AAAA", "203.0.113.24", "2001:db8::24", []DNSRecord{
			{Type: "A", Name: "a.x.cl", Content: "203.0.113.24"},
			{Type: "AAAA", Name: "a.x.cl", Content: "2001:db8::24"},
		}},
		{"solo IPv4", "203.0.113.24", "", []DNSRecord{{Type: "A", Name: "a.x.cl", Content: "203.0.113.24"}}},
		{"solo IPv6", "", "2001:db8::24", []DNSRecord{{Type: "AAAA", Name: "a.x.cl", Content: "2001:db8::24"}}},
		{"sin IPs", "", "", []DNSRecord{}},
	} {
		got := Records("a.x.cl", c.ipv4, c.ipv6)
		if got == nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %#v, esperaba %#v", c.name, got, c.want)
		}
	}
}
