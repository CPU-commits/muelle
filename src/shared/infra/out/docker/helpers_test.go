package docker

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"testing"

	"github.com/moby/moby/api/pkg/authconfig"
	"github.com/moby/moby/api/types/container"
)

func TestComputeStats(t *testing.T) {
	var s container.StatsResponse
	s.PreCPUStats.CPUUsage.TotalUsage = 2_000_000_000
	s.PreCPUStats.SystemUsage = 10_000_000_000
	s.CPUStats.CPUUsage.TotalUsage = 3_000_000_000
	s.CPUStats.SystemUsage = 14_000_000_000
	s.CPUStats.OnlineCPUs = 4
	s.MemoryStats.Usage = 100 << 20
	s.MemoryStats.Limit = 512 << 20
	s.MemoryStats.Stats = map[string]uint64{"inactive_file": 20 << 20} // cgroup v2

	got := computeStats(s)
	// 1e9 de 4e9 del sistema, con 4 CPUs: un core entero.
	if math.Abs(got.CPUPercent-100) > 1e-9 || got.MemoryBytes != 80<<20 || got.MemoryLimitBytes != 512<<20 {
		t.Fatalf("v2: %+v", got)
	}

	// cgroup v1: la caché inactiva se llama total_inactive_file.
	s.MemoryStats.Stats = map[string]uint64{"total_inactive_file": 30 << 20}
	if got := computeStats(s); got.MemoryBytes != 70<<20 {
		t.Fatalf("v1: %+v", got)
	}

	// Sin la muestra anterior no se puede calcular el CPU: 0, no NaN.
	s.PreCPUStats = container.CPUStats{}
	s.CPUStats.SystemUsage = 0
	if got := computeStats(s); got.CPUPercent != 0 {
		t.Fatalf("sin muestra anterior: %+v", got)
	}

	// Una caché mayor que el uso (no debería pasar) no da un número gigante.
	s.MemoryStats.Stats = map[string]uint64{"inactive_file": 200 << 20}
	if got := computeStats(s); got.MemoryBytes != 0 {
		t.Fatalf("caché mayor que el uso: %+v", got)
	}
}

func TestRegistryAddress(t *testing.T) {
	for host, want := range map[string]string{
		"docker.io":            "https://index.docker.io/v1/",
		"index.docker.io":      "https://index.docker.io/v1/",
		"registry-1.docker.io": "https://index.docker.io/v1/",
		"ghcr.io":              "ghcr.io",
		"registry:5000":        "registry:5000",
	} {
		if got := registryAddress(host); got != want {
			t.Errorf("registryAddress(%q) = %q, esperaba %q", host, got, want)
		}
	}
}

func TestEncodeRegistryAuth(t *testing.T) {
	if got, err := encodeRegistryAuth(nil); err != nil || got != "" {
		t.Fatalf("anónimo: %q %v", got, err)
	}
	enc, err := encodeRegistryAuth(&RegistryAuth{Host: "docker.io", Username: "bot", Password: "s3cr3t"})
	if err != nil {
		t.Fatal(err)
	}
	ac, err := authconfig.Decode(enc)
	if err != nil {
		t.Fatal(err)
	}
	if ac.Username != "bot" || ac.Password != "s3cr3t" || ac.ServerAddress != "https://index.docker.io/v1/" {
		t.Fatalf("X-Registry-Auth: %+v", ac)
	}
}

func TestDockerConfig(t *testing.T) {
	raw, err := dockerConfig([]RegistryAuth{
		{Host: "ghcr.io", Username: "bot", Password: "p:a:ss"},
		{Host: "docker.io", Username: "hub", Password: "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	decode := func(s string) string {
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if len(cfg.Auths) != 2 || decode(cfg.Auths["ghcr.io"].Auth) != "bot:p:a:ss" ||
		decode(cfg.Auths["https://index.docker.io/v1/"].Auth) != "hub:x" {
		t.Fatalf("config.json: %s", raw)
	}
	// Sin registries, igual un config.json válido (el CLI no lee el del usuario).
	raw, err = dockerConfig(nil)
	if err != nil || string(raw) != `{"auths":{}}` {
		t.Fatalf("vacío: %s %v", raw, err)
	}
}

func TestLabelFilters(t *testing.T) {
	f := labelFilters(map[string]string{"muelle.app": "api", "muelle.stage": "staging"})
	if len(f["label"]) != 2 || !f["label"]["muelle.app=api"] || !f["label"]["muelle.stage=staging"] {
		t.Fatalf("filtros: %v", f)
	}
	if f := labelFilters(nil); len(f) != 0 {
		t.Fatalf("sin etiquetas no filtra: %v", f)
	}
}
