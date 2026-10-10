package vo

import (
	"bytes"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

func TestParseAppliesDefaults(t *testing.T) {
	s, err := ParseAppSpec([]byte(`{"image":"ghcr.io/aumi-app/backend-app:staging","port":3000,"healthcheck":{"type":"http","path":"/health"}}`))
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
	s, err := ParseAppSpec([]byte(`{"image":"redis:7","port":6379}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Healthcheck.Type != HealthNone || s.Healthcheck.IntervalS != 0 {
		t.Fatalf("healthcheck = %+v", s.Healthcheck)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	s, err := ParseAppSpec([]byte(`{"image":"redis:7","port":6379,"replicas":0,"volumes":[{"name":"data","mount":"/data"}],"healthcheck":{"type":"http","path":"/h"},"resources":{"memory_mb":64,"cpus":0.5}}`))
	if err != nil {
		t.Fatal(err)
	}
	again, err := ParseAppSpec(s.JSON())
	if err != nil {
		t.Fatalf("el JSON normalizado debe volver a parsear: %v", err)
	}
	if !reflect.DeepEqual(s, again) {
		t.Fatalf("got %+v, want %+v", again, s)
	}
	if !bytes.Equal(s.JSON(), again.JSON()) {
		t.Fatalf("el JSON no es estable:\n%s\n%s", s.JSON(), again.JSON())
	}
	// replicas 0 se conserva (no se confunde con "no indicado").
	if *again.Replicas != 0 || again.Volumes[0].Mount != "/data" {
		t.Fatalf("got %+v", again)
	}
}

func TestParseRejects(t *testing.T) {
	long := strings.Repeat("a", 5000)
	cases := []struct{ name, raw, field string }{
		{"vacío", ``, "spec"},
		{"null", `null`, "spec"},
		{"campo desconocido", `{"image":"x","port":80,"foo":1}`, "spec"},
		{"mayúsculas en la clave", `{"IMAGE":"x","port":80}`, "spec"},
		{"datos después del JSON", `{"image":"x","port":80} {}`, "spec"},
		{"clave duplicada", `{"image":"x","image":"y","port":80}`, "spec"},
		{"UTF-8 inválido", "{\"image\":\"x\xff\",\"port\":80}", "spec"},
		{"sin imagen", `{"port":80}`, "image"},
		{"imagen con espacio", `{"image":"x y","port":80}`, "image"},
		{"imagen con variable", `{"image":"${X}","port":80}`, "image"},
		{"imagen que parece flag", `{"image":"--help","port":80}`, "image"},
		{"imagen con mayúsculas", `{"image":"Ghcr.io/Foo/Bar:Latest","port":80}`, "image"},
		{"imagen con NUL", `{"image":"nginx\u0000","port":80}`, "image"},
		{"imagen con ruta", `{"image":"../../containers/json?","port":80}`, "image"},
		{"imagen demasiado larga", `{"image":"` + long + `","port":80}`, "image"},
		{"puerto 0", `{"image":"x","port":0}`, "port"},
		{"puerto alto", `{"image":"x","port":70000}`, "port"},
		{"réplicas negativas", `{"image":"x","port":80,"replicas":-1}`, "replicas"},
		{"tipo desconocido", `{"image":"x","port":80,"healthcheck":{"type":"tcp"}}`, "healthcheck.type"},
		{"path sin barra", `{"image":"x","port":80,"healthcheck":{"type":"http","path":"health"}}`, "healthcheck.path"},
		{"path con comilla simple", `{"image":"x","port":80,"healthcheck":{"type":"bun-http","path":"/h'"}}`, "healthcheck.path"},
		{"path con &", `{"image":"x","port":80,"healthcheck":{"type":"http","path":"/h?a=1&b=2"}}`, "healthcheck.path"},
		{"path con ;", `{"image":"x","port":80,"healthcheck":{"type":"http","path":"/h;id"}}`, "healthcheck.path"},
		{"path con |", `{"image":"x","port":80,"healthcheck":{"type":"http","path":"/h|id"}}`, "healthcheck.path"},
		{"path con $", `{"image":"x","port":80,"healthcheck":{"type":"http","path":"/$HOME"}}`, "healthcheck.path"},
		{"path con comilla invertida", "{\"image\":\"x\",\"port\":80,\"healthcheck\":{\"type\":\"http\",\"path\":\"/`id`\"}}", "healthcheck.path"},
		{"path con comilla doble", `{"image":"x","port":80,"healthcheck":{"type":"http","path":"/h\""}}`, "healthcheck.path"},
		{"path con espacio", `{"image":"x","port":80,"healthcheck":{"type":"http","path":"/a b"}}`, "healthcheck.path"},
		{"command vacío", `{"image":"x","port":80,"healthcheck":{"type":"command"}}`, "healthcheck.command"},
		{"none con path", `{"image":"x","port":80,"healthcheck":{"type":"none","path":"/h"}}`, "healthcheck"},
		{"none con intervalo", `{"image":"x","port":80,"healthcheck":{"type":"none","interval_s":5}}`, "healthcheck"},
		{"none con retries", `{"image":"x","port":80,"healthcheck":{"type":"none","retries":2}}`, "healthcheck"},
		{"timeout alto", `{"image":"x","port":80,"healthcheck":{"type":"http","path":"/h","timeout_s":601}}`, "healthcheck.timeout_s"},
		{"start period negativo", `{"image":"x","port":80,"healthcheck":{"type":"http","path":"/h","start_period_s":-1}}`, "healthcheck.start_period_s"},
		{"volumen sin nombre", `{"image":"x","port":80,"volumes":[{"name":"","mount":"/data"}]}`, "volumes[0].name"},
		{"mount relativo", `{"image":"x","port":80,"volumes":[{"name":"d","mount":"data"}]}`, "volumes[0].mount"},
		{"mount sucio", `{"image":"x","port":80,"volumes":[{"name":"d","mount":"/data/"}]}`, "volumes[0].mount"},
		{"mount raíz", `{"image":"x","port":80,"volumes":[{"name":"d","mount":"/"}]}`, "volumes[0].mount"},
		{"mount con opciones", `{"image":"x","port":80,"volumes":[{"name":"d","mount":"/data:ro"}]}`, "volumes[0].mount"},
		{"mount con salto de línea", `{"image":"x","port":80,"volumes":[{"name":"d","mount":"/a\nb"}]}`, "volumes[0].mount"},
		{"mount con variable", `{"image":"x","port":80,"volumes":[{"name":"d","mount":"/$HOME"}]}`, "volumes[0].mount"},
		{"mount larguísimo", `{"image":"x","port":80,"volumes":[{"name":"d","mount":"/` + long + `"}]}`, "volumes[0].mount"},
		{"volumen repetido", `{"image":"x","port":80,"volumes":[{"name":"d","mount":"/a"},{"name":"d","mount":"/b"}]}`, "volumes[1].name"},
		{"mount repetido", `{"image":"x","port":80,"volumes":[{"name":"a","mount":"/d"},{"name":"b","mount":"/d"}]}`, "volumes[1].mount"},
		{"memoria baja", `{"image":"x","port":80,"resources":{"memory_mb":8}}`, "resources.memory_mb"},
		{"cpus altas", `{"image":"x","port":80,"resources":{"cpus":1000}}`, "resources.cpus"},
		{"demasiados argumentos", `{"image":"x","port":80,"command":[` + strings.Repeat(`"a",`, 64) + `"a"]}`, "command"},
	}
	for _, c := range cases {
		_, err := ParseAppSpec([]byte(c.raw))
		var ve *validate.Error
		if !errors.As(err, &ve) {
			t.Errorf("%s: esperaba *validate.Error, got %v", c.name, err)
			continue
		}
		if ve.Field != c.field {
			t.Errorf("%s: field = %q, esperaba %q (%v)", c.name, ve.Field, c.field, ve)
		}
	}
}

func TestParseAcceptsImages(t *testing.T) {
	sha40 := strings.Repeat("a", 40)
	d64 := strings.Repeat("b", 64)
	for _, img := range []string{
		"nginx",
		"nginx:1.27",
		"ghcr.io/aumi-app/backend-app:sha-" + sha40,
		"registry:5000/a/b@sha256:" + d64,
		"ghcr.io/x/y:tag@sha256:" + d64,
	} {
		if _, err := ParseAppSpec([]byte(`{"image":"` + img + `","port":80}`)); err != nil {
			t.Errorf("%s: %v", img, err)
		}
	}
}

func TestParseAcceptsHealthPaths(t *testing.T) {
	for _, p := range []string{"/", "/health", "/a/b.c_d-e~f", "/h?x=1%20", "/h?a=b+c"} {
		raw := `{"image":"x","port":80,"healthcheck":{"type":"http","path":"` + p + `"}}`
		if _, err := ParseAppSpec([]byte(raw)); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}

func TestValidateRejectsNaNCPUs(t *testing.T) {
	s, err := ParseAppSpec([]byte(`{"image":"x","port":80}`))
	if err != nil {
		t.Fatal(err)
	}
	s.Resources = &Resources{CPUs: math.NaN()}
	if err := s.Validate(); err == nil {
		t.Fatal("NaN no es un valor de CPU válido")
	}
}
