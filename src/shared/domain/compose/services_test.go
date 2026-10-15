package compose

import "testing"

func TestServices(t *testing.T) {
	src := "services:\n  web:\n    image: nginx\n    expose: [\"80\", 8080, \"9000/tcp\", \"100-200\", \"${P}\", \"80\", \"9000/udp\"]\n" +
		"  api:\n    image: x\n    expose: [3000]\n  worker:\n    image: y\n"
	got := Services(src)
	want := []ServiceInfo{{Name: "api", Ports: []int{3000}}, {Name: "web", Ports: []int{80, 8080, 9000}}, {Name: "worker", Ports: []int{}}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i].Name != want[i].Name || got[i].Ports == nil || !equalInts(got[i].Ports, want[i].Ports) {
			t.Errorf("[%d] = %+v, esperaba %+v", i, got[i], want[i])
		}
	}
	// Lee sin validar: un compose que la lista de permitidos rechaza (y que
	// pudo guardar la Fase 1) igual muestra sus servicios.
	if s := Services(web("    ports: [\"80:80\"]\n    privileged: true\n")); len(s) != 1 || s[0].Name != "web" {
		t.Errorf("Services de un compose con ports = %+v", s)
	}
	// Ilegible: nil, sin panic.
	for _, bad := range []string{"", "a: 1", "services: [", web("    networks: [1]\n"), "- 1\n", "services: {}\n"} {
		if s := Services(bad); s != nil {
			t.Errorf("Services(%q) = %+v, esperaba nil", bad, s)
		}
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
