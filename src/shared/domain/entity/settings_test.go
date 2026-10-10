package entity

import "testing"

func TestBoolFormat(t *testing.T) {
	if FormatBool(true) != "true" || FormatBool(false) != "false" {
		t.Fatal("formato inesperado")
	}
	for in, want := range map[string]bool{"true": true, "false": false, "": false, "1": false, "TRUE": false} {
		if got := ParseBool(in); got != want {
			t.Errorf("ParseBool(%q)=%v, esperaba %v", in, got, want)
		}
	}
	if !ParseBool(FormatBool(true)) || ParseBool(FormatBool(false)) {
		t.Fatal("el formato no es reversible")
	}
}
