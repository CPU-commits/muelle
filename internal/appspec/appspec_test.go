package appspec

import (
	"errors"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/internal/validate"
)

func TestParseAppliesDefaults(t *testing.T) {
	s, err := Parse([]byte(`{"image":"ghcr.io/aumi-app/backend-app:staging","port":3000,"healthcheck":{"type":"http","path":"/health"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if *s.Replicas != 1 {
		t.Errorf("replicas = %d", *s.Replicas)
	}
	h := s.Healthcheck
	if h.IntervalS != 10 || h.TimeoutS != 5 || h.Retries != 3 || h.StartPeriodS != 20 {
		t.Errorf("defaults del healthcheck: %+v", h)
	}
	if s.Command == nil || s.Volumes == nil {
		t.Error("command y volumes deben normalizarse a listas vacías")
	}
}

func TestParseWithoutHealthcheckIsNone(t *testing.T) {
	s, err := Parse([]byte(`{"image":"redis:7","port":6379}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Healthcheck.Type != HealthNone || s.Healthcheck.IntervalS != 0 {
		t.Fatalf("healthcheck = %+v", s.Healthcheck)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	s, err := Parse([]byte(`{"image":"redis:7","port":6379,"replicas":0,"volumes":[{"name":"data","mount":"/data"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(s.JSON())
	if err != nil {
		t.Fatalf("el JSON normalizado debe volver a parsear: %v", err)
	}
	if *again.Replicas != 0 || again.Volumes[0].Mount != "/data" {
		t.Fatalf("got %+v", again)
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]string{
		"vacío":                 ``,
		"null":                  `null`,
		"campo desconocido":     `{"image":"x","port":80,"foo":1}`,
		"sin imagen":            `{"port":80}`,
		"imagen con espacio":    `{"image":"x y","port":80}`,
		"puerto 0":              `{"image":"x","port":0}`,
		"puerto alto":           `{"image":"x","port":70000}`,
		"réplicas negativas":    `{"image":"x","port":80,"replicas":-1}`,
		"tipo desconocido":      `{"image":"x","port":80,"healthcheck":{"type":"tcp"}}`,
		"path sin barra":        `{"image":"x","port":80,"healthcheck":{"type":"http","path":"health"}}`,
		"path con comilla":      `{"image":"x","port":80,"healthcheck":{"type":"bun-http","path":"/h'"}}`,
		"command vacío":         `{"image":"x","port":80,"healthcheck":{"type":"command"}}`,
		"none con path":         `{"image":"x","port":80,"healthcheck":{"type":"none","path":"/h"}}`,
		"timeout alto":          `{"image":"x","port":80,"healthcheck":{"type":"http","path":"/h","timeout_s":601}}`,
		"volumen sin nombre":    `{"image":"x","port":80,"volumes":[{"name":"","mount":"/data"}]}`,
		"mount relativo":        `{"image":"x","port":80,"volumes":[{"name":"d","mount":"data"}]}`,
		"mount sucio":           `{"image":"x","port":80,"volumes":[{"name":"d","mount":"/data/"}]}`,
		"mount raíz":            `{"image":"x","port":80,"volumes":[{"name":"d","mount":"/"}]}`,
		"volumen repetido":      `{"image":"x","port":80,"volumes":[{"name":"d","mount":"/a"},{"name":"d","mount":"/b"}]}`,
		"mount repetido":        `{"image":"x","port":80,"volumes":[{"name":"a","mount":"/d"},{"name":"b","mount":"/d"}]}`,
		"memoria baja":          `{"image":"x","port":80,"resources":{"memory_mb":8}}`,
		"cpus altas":            `{"image":"x","port":80,"resources":{"cpus":1000}}`,
		"demasiados argumentos": `{"image":"x","port":80,"command":[` + strings.Repeat(`"a",`, 64) + `"a"]}`,
	}
	for name, raw := range cases {
		_, err := Parse([]byte(raw))
		var ve *validate.Error
		if !errors.As(err, &ve) {
			t.Errorf("%s: esperaba *validate.Error, got %v", name, err)
		}
	}
}
