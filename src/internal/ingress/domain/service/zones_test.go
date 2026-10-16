package service

import "testing"

func TestMatchZone(t *testing.T) {
	zones := []Zone{{ID: "z1", Name: "example.com"}, {ID: "z2", Name: "a.example.com"}, {ID: "z3", Name: "otro.cl"}}
	for _, c := range []struct {
		name, hostname string
		zones          []Zone
		want           string // id; "" = ninguna
	}{
		{"el sufijo más largo", "api.a.example.com", zones, "z2"},
		{"el más largo aunque venga primero", "api.a.example.com", []Zone{zones[1], zones[0]}, "z2"},
		{"un subdominio", "api.example.com", zones, "z1"},
		{"el ápice", "example.com", zones, "z1"},
		{"el ápice de la zona más larga", "a.example.com", zones, "z2"},
		{"no es sufijo por etiquetas", "badexample.com", zones, ""},
		{"otra zona", "api.otro.com", zones, ""},
		{"mayúsculas en la zona", "api.tudominio.cl", []Zone{{ID: "z9", Name: "TuDominio.CL"}}, "z9"},
		{"sin zonas", "api.example.com", nil, ""},
	} {
		z, ok := MatchZone(c.hostname, c.zones)
		if ok != (c.want != "") || z.ID != c.want {
			t.Errorf("%s: MatchZone(%q) = %+v, %v; esperaba %q", c.name, c.hostname, z, ok, c.want)
		}
	}
}
