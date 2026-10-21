package compose

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestMarshalEscapesDollar(t *testing.T) {
	f := &File{Services: map[string]Service{"web": {
		Image:       "nginx",
		Environment: map[string]string{"PRICE": "$5", "K$Y": "v"},
		Command:     []string{"sh", "-c", "echo $HOME"},
		Labels:      map[string]string{"com.example.p": "a$b"},
		Healthcheck: &Healthcheck{Test: []string{"CMD-SHELL", "test $X"}},
	}}}
	out, err := Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`"PRICE": "$$5"`, `"echo $$HOME"`, `"com.example.p": "a$$b"`, `"test $$X"`, `"K$Y": "v"`} {
		if !strings.Contains(s, want) {
			t.Errorf("falta %s en:\n%s", want, s)
		}
	}
	if strings.Contains(s, "K$$Y") {
		t.Errorf("una clave no se escapa:\n%s", s)
	}
	// El File no cambia.
	if f.Services["web"].Environment["PRICE"] != "$5" {
		t.Fatal("Marshal no debe modificar el File")
	}
}

func TestMarshalDeterministic(t *testing.T) {
	u := uint64(2)
	f := &File{
		Services: map[string]Service{
			"b": {Image: "nginx", Environment: map[string]string{"Z": "1", "A": "2", "M": "<&>"}, Deploy: &Deploy{Replicas: &u}},
			"a": {Image: "redis", Volumes: []Mount{{Type: "volume", Source: "data", Target: "/data"}}},
		},
		Volumes:  map[string]Volume{"data": {}},
		Networks: map[string]Network{"p_s": {Name: "p_s", External: true}},
	}
	a, err := Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		b, err := Marshal(f)
		if err != nil || !bytes.Equal(a, b) {
			t.Fatalf("Marshal no es determinista:\n%s\n%s", a, b)
		}
	}
	if !json.Valid(a) || !bytes.HasSuffix(a, []byte("}\n")) {
		t.Fatalf("no es JSON válido con \\n final:\n%s", a)
	}
	if !bytes.Contains(a, []byte("\n  \"services\": {\n    \"a\": {")) {
		t.Fatalf("esperaba claves ordenadas e indentadas con dos espacios:\n%s", a)
	}
	if !bytes.Contains(a, []byte(`"M": "<&>"`)) {
		t.Fatalf("sin escapes de HTML:\n%s", a)
	}

	// Sin redes ni volúmenes, esas claves no aparecen.
	out, err := Marshal(&File{Services: map[string]Service{"web": {Image: "nginx"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"networks", "volumes", "deploy", "null", "{}"} {
		if bytes.Contains(out, []byte(k)) {
			t.Errorf("no debe aparecer %q:\n%s", k, out)
		}
	}
}
