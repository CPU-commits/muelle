package docker

import (
	"encoding/base64"
	"encoding/json"

	"github.com/moby/moby/api/pkg/authconfig"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
)

// computeStats calcula el CPU y la memoria como docker stats: el CPU es la
// parte del tiempo de CPU del sistema que usó el contenedor entre las dos
// muestras, por la cantidad de CPUs (100 = un core); la memoria no cuenta la
// caché inactiva (inactive_file en cgroup v2, total_inactive_file en v1).
func computeStats(s container.StatsResponse) Stats {
	var out Stats
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(s.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s.CPUStats.SystemUsage) - float64(s.PreCPUStats.SystemUsage)
	cpus := float64(s.CPUStats.OnlineCPUs)
	if cpus == 0 {
		cpus = float64(len(s.CPUStats.CPUUsage.PercpuUsage))
	}
	if s.PreCPUStats.SystemUsage > 0 && cpuDelta > 0 && sysDelta > 0 && cpus > 0 {
		out.CPUPercent = cpuDelta / sysDelta * cpus * 100
	}
	cache := s.MemoryStats.Stats["inactive_file"]
	if v, ok := s.MemoryStats.Stats["total_inactive_file"]; ok {
		cache = v
	}
	if s.MemoryStats.Usage > cache {
		out.MemoryBytes = s.MemoryStats.Usage - cache
	}
	out.MemoryLimitBytes = s.MemoryStats.Limit
	return out
}

// dockerHubAddress es como Docker nombra a Docker Hub en las credenciales.
const dockerHubAddress = "https://index.docker.io/v1/"

// registryAddress traduce el dominio de una imagen al nombre del registry en
// X-Registry-Auth y en config.json: Docker Hub tiene uno propio.
func registryAddress(host string) string {
	switch host {
	case "docker.io", "index.docker.io", "registry-1.docker.io":
		return dockerHubAddress
	}
	return host
}

// encodeRegistryAuth arma el header X-Registry-Auth; nil es anónimo ("").
func encodeRegistryAuth(a *RegistryAuth) (string, error) {
	if a == nil {
		return "", nil
	}
	return authconfig.Encode(registry.AuthConfig{
		Username:      a.Username,
		Password:      a.Password,
		ServerAddress: registryAddress(a.Host),
	})
}

// dockerConfig arma el config.json del DOCKER_CONFIG temporal de docker stack
// deploy: una entrada "auth" (base64 de usuario:contraseña) por registry.
func dockerConfig(auths []RegistryAuth) ([]byte, error) {
	type entry struct {
		Auth string `json:"auth"`
	}
	cfg := struct {
		Auths map[string]entry `json:"auths"`
	}{Auths: map[string]entry{}}
	for _, a := range auths {
		cfg.Auths[registryAddress(a.Host)] = entry{
			Auth: base64.StdEncoding.EncodeToString([]byte(a.Username + ":" + a.Password)),
		}
	}
	return json.Marshal(cfg)
}

// labelFilters convierte etiquetas a filtros "label=k=v" (todas tienen que estar).
func labelFilters(labels map[string]string) client.Filters {
	f := client.Filters{}
	for k, v := range labels {
		f.Add("label", k+"="+v)
	}
	return f
}
