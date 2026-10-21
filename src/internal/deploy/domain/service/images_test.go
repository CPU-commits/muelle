package service

import (
	"maps"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
)

var (
	digA = "sha256:" + strings.Repeat("a", 64)
	digB = "sha256:" + strings.Repeat("b", 64)
	digC = "sha256:" + strings.Repeat("c", 64)
)

// done arma el deploy actual con lo que guardó su paso 2.
func done(requested, config map[string]string) *entity.Deploy {
	return &entity.Deploy{Status: entity.StatusSucceeded, RequestedImages: requested, ConfigImages: config}
}

func TestChooseImages(t *testing.T) {
	cfg := map[string]string{"api": "ghcr.io/x/api:staging"}
	cases := []struct {
		name      string
		config    map[string]string
		overrides map[string]string
		current   *entity.Deploy
		want      map[string]string
	}{
		{"primer deploy: la de la configuración", cfg, nil, nil, cfg},
		{"override", cfg, map[string]string{"api": "ghcr.io/x/api:sha-1"}, nil,
			map[string]string{"api": "ghcr.io/x/api:sha-1"}},
		{"sin cuerpo después de CI con tag: el tag, que se vuelve a resolver", cfg, map[string]string{},
			done(map[string]string{"api": "ghcr.io/x/api:sha-1"}, cfg),
			map[string]string{"api": "ghcr.io/x/api:sha-1"}},
		{"sin cuerpo después de CI por digest: el digest", cfg, nil,
			done(map[string]string{"api": "ghcr.io/x/api@" + digA}, cfg),
			map[string]string{"api": "ghcr.io/x/api@" + digA}},
		{"digest fijado en la configuración: se mantiene",
			map[string]string{"api": "ghcr.io/x/api:1.2@" + digB}, nil,
			done(map[string]string{"api": "ghcr.io/x/api:1.2@" + digB}, map[string]string{"api": "ghcr.io/x/api:1.2@" + digB}),
			map[string]string{"api": "ghcr.io/x/api:1.2@" + digB}},
		{"la configuración cambió: la nueva",
			map[string]string{"api": "ghcr.io/x/api:v2"}, nil,
			done(map[string]string{"api": "ghcr.io/x/api:sha-1"}, map[string]string{"api": "ghcr.io/x/api:v1"}),
			map[string]string{"api": "ghcr.io/x/api:v2"}},
		{"un servicio nuevo del compose: el de la configuración",
			map[string]string{"api": "ghcr.io/x/api:staging", "worker": "ghcr.io/x/worker:staging"}, nil,
			done(map[string]string{"api": "ghcr.io/x/api:sha-1"}, cfg),
			map[string]string{"api": "ghcr.io/x/api:sha-1", "worker": "ghcr.io/x/worker:staging"}},
		{"override de un servicio y los demás de arrastre",
			map[string]string{"api": "ghcr.io/x/api:staging", "web": "ghcr.io/x/web:staging"},
			map[string]string{"web": "ghcr.io/x/web:sha-9"},
			done(map[string]string{"api": "ghcr.io/x/api@" + digA, "web": "ghcr.io/x/web:sha-8"},
				map[string]string{"api": "ghcr.io/x/api:staging", "web": "ghcr.io/x/web:staging"}),
			map[string]string{"api": "ghcr.io/x/api@" + digA, "web": "ghcr.io/x/web:sha-9"}},
		{"un override de un servicio que no está en la configuración se ignora", cfg,
			map[string]string{"viejo": "ghcr.io/x/viejo:1"}, nil, cfg},
		{"un servicio que el deploy actual no guardó en config_images: el de la configuración", cfg, nil,
			done(map[string]string{"api": "ghcr.io/x/api:sha-1"}, map[string]string{}),
			cfg},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := maps.Clone(c.overrides)
			got := ChooseImages(c.config, c.overrides, c.current)
			if !maps.Equal(got, c.want) {
				t.Fatalf("got %v, esperaba %v", got, c.want)
			}
			if !maps.Equal(in, c.overrides) {
				t.Fatal("modificó los overrides")
			}
		})
	}
}

// step2 hace lo que el paso 2 guarda de un deploy: las refs pedidas, las de la
// configuración y las resueltas con el digest que da el registry.
func step2(t *testing.T, config, overrides map[string]string, current *entity.Deploy, registry map[string]string) *entity.Deploy {
	t.Helper()
	requested := ChooseImages(config, overrides, current)
	images := map[string]string{}
	for svc, ref := range requested {
		d := PinnedDigest(ref)
		if d == "" {
			d = registry[ref]
		}
		r, err := ResolvedRef(ref, d)
		if err != nil {
			t.Fatal(err)
		}
		images[svc] = r
	}
	return &entity.Deploy{Status: entity.StatusSucceeded, Images: images, RequestedImages: requested, ConfigImages: maps.Clone(config)}
}

// Contrato §2.2: "un rollback no se deshace con el próximo deploy sin cuerpo".
// El rollback pide las refs resueltas (con digest) del deploy al que vuelve, y
// el deploy sin cuerpo que sigue repite esa ref pedida, aunque el tag se movió.
func TestRollbackNotUndoneByBodylessDeploy(t *testing.T) {
	config := map[string]string{"api": "ghcr.io/x/api:staging"}
	d1 := step2(t, config, nil, nil, map[string]string{"ghcr.io/x/api:staging": digA})
	if d1.Images["api"] != "ghcr.io/x/api:staging@"+digA {
		t.Fatalf("D1 = %v", d1.Images)
	}
	d2 := step2(t, config, nil, d1, map[string]string{"ghcr.io/x/api:staging": digB})
	if d2.Images["api"] != "ghcr.io/x/api:staging@"+digB {
		t.Fatalf("D2 = %v", d2.Images)
	}
	// El rollback: los overrides son las imágenes resueltas de D1 (contrato §2.7).
	r := step2(t, config, maps.Clone(d1.Images), d2, map[string]string{"ghcr.io/x/api:staging": digC})
	if r.Images["api"] != "ghcr.io/x/api:staging@"+digA {
		t.Fatalf("rollback = %v", r.Images)
	}
	// El tag siguió moviéndose (C), y el próximo deploy sin cuerpo repite A.
	next := step2(t, config, map[string]string{}, r, map[string]string{"ghcr.io/x/api:staging": digC})
	if next.Images["api"] != "ghcr.io/x/api:staging@"+digA {
		t.Fatalf("el deploy sin cuerpo deshizo el rollback: %v", next.Images)
	}
	// Para volver a seguir el tag: un override, o cambiar la configuración.
	over := step2(t, config, map[string]string{"api": "ghcr.io/x/api:staging"}, next, map[string]string{"ghcr.io/x/api:staging": digC})
	if over.Images["api"] != "ghcr.io/x/api:staging@"+digC {
		t.Fatalf("con override = %v", over.Images)
	}
	changed := map[string]string{"api": "ghcr.io/x/api:prod"}
	cfgChanged := step2(t, changed, nil, next, map[string]string{"ghcr.io/x/api:prod": digB})
	if cfgChanged.Images["api"] != "ghcr.io/x/api:prod@"+digB {
		t.Fatalf("con la configuración cambiada = %v", cfgChanged.Images)
	}
}

func TestResolvedRef(t *testing.T) {
	cases := []struct{ requested, digest, want string }{
		{"nginx", digA, "nginx:latest@" + digA},
		{"nginx:1.27", digA, "nginx:1.27@" + digA},
		{"ghcr.io/x/y:sha-1", digA, "ghcr.io/x/y:sha-1@" + digA},
		{"ghcr.io/x/y@" + digA, digA, "ghcr.io/x/y@" + digA},
		{"x:tag@" + digA, digA, "x:tag@" + digA},
		{"docker.io/library/nginx:1", digA, "nginx:1@" + digA},
		{"localhost:5000/a/b", digB, "localhost:5000/a/b:latest@" + digB},
	}
	for _, c := range cases {
		got, err := ResolvedRef(c.requested, c.digest)
		if err != nil || got != c.want {
			t.Errorf("ResolvedRef(%q) = %q, %v; esperaba %q", c.requested, got, err, c.want)
		}
	}
	// Errores: una ref inválida, un digest inválido o vacío, o un digest
	// distinto del que la ref ya trae.
	for _, c := range []struct{ requested, digest string }{
		{"Nginx", digA}, {"nginx", "sha256:corto"}, {"nginx", ""},
		{"ghcr.io/x/y@" + digA, digB},
	} {
		if got, err := ResolvedRef(c.requested, c.digest); err == nil {
			t.Errorf("ResolvedRef(%q, %q) = %q: debió fallar", c.requested, c.digest, got)
		}
	}
}

func TestImageHost(t *testing.T) {
	for ref, want := range map[string]string{
		"nginx":                          "docker.io",
		"library/nginx:1":                "docker.io",
		"index.docker.io/x/y":            "docker.io",
		"ghcr.io/x":                      "ghcr.io",
		"ghcr.io/x/y:1@" + digA:          "ghcr.io",
		"localhost:5000/x":               "localhost:5000",
		"registry.example.com:5000/a/b":  "registry.example.com:5000",
		"registry.example.com/a/b:tag-1": "registry.example.com",
	} {
		got, err := ImageHost(ref)
		if err != nil || got != want {
			t.Errorf("ImageHost(%q) = %q, %v; esperaba %q", ref, got, err, want)
		}
	}
	if _, err := ImageHost("NO VALE"); err == nil {
		t.Error("una ref inválida debió fallar")
	}
}

func TestPinnedDigest(t *testing.T) {
	for ref, want := range map[string]string{
		"nginx":                 "",
		"nginx:1":               "",
		"ghcr.io/x/y@" + digA:   digA,
		"ghcr.io/x/y:1@" + digB: digB,
		"no vale":               "",
	} {
		if got := PinnedDigest(ref); got != want {
			t.Errorf("PinnedDigest(%q) = %q, esperaba %q", ref, got, want)
		}
	}
}
