package compose

import (
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
)

// convert copia del proyecto de compose-go al compose final solo los campos
// que conoce (la otra mitad de la lista de permitidos: lo que compose-go lea
// de más nunca llega al compose final). env son las variables con que se
// interpoló: una variable de environment sin valor toma la de env, como en
// compose con el entorno de la shell, y si no está, no va (docker stack
// deploy la tomaría del entorno de muelle).
func convert(p *types.Project, env map[string]string) (*File, error) {
	f := &File{Services: make(map[string]Service, len(p.Services))}
	for name, s := range p.Services {
		svc, err := convertService(s, env)
		if err != nil {
			return nil, err
		}
		f.Services[name] = svc
	}
	if len(p.Volumes) > 0 {
		f.Volumes = make(map[string]Volume, len(p.Volumes))
		for name, v := range p.Volumes {
			f.Volumes[name] = Volume{Labels: stringMap(v.Labels)}
		}
	}
	return f, nil
}

func convertService(s types.ServiceConfig, env map[string]string) (Service, error) {
	dep, err := convertDeploy(s.Deploy, s.Name)
	if err != nil {
		return Service{}, err
	}
	out := Service{
		Image:           s.Image,
		Command:         stringList(s.Command),
		Entrypoint:      stringList(s.Entrypoint),
		Environment:     environment(s.Environment, env),
		Healthcheck:     healthcheck(s.HealthCheck),
		Deploy:          dep,
		Labels:          stringMap(s.Labels),
		Volumes:         mounts(s.Volumes),
		Tmpfs:           stringList(s.Tmpfs),
		User:            s.User,
		WorkingDir:      s.WorkingDir,
		StopGracePeriod: durationPtr(s.StopGracePeriod),
		StopSignal:      s.StopSignal,
		ReadOnly:        s.ReadOnly,
		Ulimits:         ulimits(s.Ulimits),
		CapDrop:         stringList(s.CapDrop),
		Hostname:        s.Hostname,
		Domainname:      s.DomainName,
		ExtraHosts:      hosts(s.ExtraHosts),
		DNS:             stringList(s.DNS),
		DNSSearch:       stringList(s.DNSSearch),
		Expose:          stringList(s.Expose),
		Tty:             s.Tty,
		StdinOpen:       s.StdinOpen,
	}
	if s.Init != nil {
		v := *s.Init
		out.Init = &v
	}
	if s.Logging != nil {
		out.Logging = &Logging{Driver: s.Logging.Driver, Options: stringMap(s.Logging.Options)}
	}
	return out, nil
}

func environment(m types.MappingWithEquals, env map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		if v != nil {
			out[k] = *v
		} else if val, ok := env[k]; ok {
			out[k] = val
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func healthcheck(h *types.HealthCheckConfig) *Healthcheck {
	if h == nil {
		return nil
	}
	return &Healthcheck{
		Test:          stringList(h.Test),
		Interval:      durationPtr(h.Interval),
		Timeout:       durationPtr(h.Timeout),
		Retries:       uintPtr(h.Retries),
		StartPeriod:   durationPtr(h.StartPeriod),
		StartInterval: durationPtr(h.StartInterval),
		Disable:       h.Disable,
	}
}

func convertDeploy(d *types.DeployConfig, service string) (*Deploy, error) {
	if d == nil {
		return nil, nil
	}
	out := &Deploy{Mode: d.Mode, EndpointMode: d.EndpointMode, Labels: stringMap(d.Labels)}
	if d.Replicas != nil {
		if *d.Replicas < 0 {
			return nil, fail("services."+service+".deploy.replicas", "no puede ser negativo")
		}
		r := uint64(*d.Replicas)
		out.Replicas = &r
	}
	pl := d.Placement
	if len(pl.Constraints) > 0 || len(pl.Preferences) > 0 || pl.MaxReplicas > 0 {
		out.Placement = &Placement{Constraints: stringList(pl.Constraints), MaxReplicasPerNode: pl.MaxReplicas}
		for _, pref := range pl.Preferences {
			out.Placement.Preferences = append(out.Placement.Preferences, Preference{Spread: pref.Spread})
		}
	}
	limits, reservations := resource(d.Resources.Limits), resource(d.Resources.Reservations)
	if limits != nil || reservations != nil {
		out.Resources = &Resources{Limits: limits, Reservations: reservations}
	}
	if rp := d.RestartPolicy; rp != nil {
		out.RestartPolicy = &RestartPolicy{
			Condition:   rp.Condition,
			Delay:       durationPtr(rp.Delay),
			MaxAttempts: uintPtr(rp.MaxAttempts),
			Window:      durationPtr(rp.Window),
		}
	}
	out.UpdateConfig = updateConfig(d.UpdateConfig)
	out.RollbackConfig = updateConfig(d.RollbackConfig)
	return out, nil
}

// resource copia cpus y memory. compose-go da los cpus como float32: con
// precisión 32 al formatear, "0.1" sigue siendo "0.1" (en 64 bits saldría
// "0.10000000149011612"). La memoria va en bytes, como string (ver Resource).
func resource(r *types.Resource) *Resource {
	if r == nil {
		return nil
	}
	out := &Resource{}
	if r.NanoCPUs != 0 {
		out.CPUs = strconv.FormatFloat(float64(r.NanoCPUs), 'f', -1, 32)
	}
	if r.MemoryBytes != 0 {
		out.Memory = strconv.FormatInt(int64(r.MemoryBytes), 10)
	}
	if *out == (Resource{}) {
		return nil
	}
	return out
}

func updateConfig(u *types.UpdateConfig) *UpdateConfig {
	if u == nil {
		return nil
	}
	out := &UpdateConfig{
		Parallelism:   uintPtr(u.Parallelism),
		Delay:         duration(u.Delay),
		FailureAction: u.FailureAction,
		Monitor:       duration(u.Monitor),
		Order:         u.Order,
	}
	if u.MaxFailureRatio != 0 {
		// float32 → float64 sin el ruido de los bits de más (0.1, no 0.1000000015).
		r, _ := strconv.ParseFloat(strconv.FormatFloat(float64(u.MaxFailureRatio), 'f', -1, 32), 64)
		out.MaxFailureRatio = &r
	}
	return out
}

func mounts(vs []types.ServiceVolumeConfig) []Mount {
	var out []Mount
	for _, v := range vs {
		m := Mount{Type: v.Type, Source: v.Source, Target: v.Target, ReadOnly: v.ReadOnly}
		if v.Volume != nil && v.Volume.NoCopy {
			m.Volume = &MountVolume{NoCopy: true}
		}
		if v.Tmpfs != nil && (v.Tmpfs.Size != 0 || v.Tmpfs.Mode != 0) {
			m.Tmpfs = &MountTmpfs{Size: int64(v.Tmpfs.Size), Mode: v.Tmpfs.Mode}
		}
		out = append(out, m)
	}
	return out
}

// ulimits: un entero solo vale para el límite blando y el duro.
func ulimits(m map[string]*types.UlimitsConfig) map[string]Ulimit {
	out := map[string]Ulimit{}
	for k, u := range m {
		switch {
		case u == nil:
		case u.Single != 0:
			out[k] = Ulimit{Soft: u.Single, Hard: u.Single}
		default:
			out[k] = Ulimit{Soft: u.Soft, Hard: u.Hard}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// hosts son los extra_hosts como "host:ip" (la forma que entiende docker
// stack deploy), ordenados.
func hosts(h types.HostsList) []string {
	if len(h) == 0 {
		return nil
	}
	l := h.AsList(":")
	slices.Sort(l)
	return l
}

func stringList[S ~[]string](s S) []string {
	if len(s) == 0 {
		return nil
	}
	return slices.Clone([]string(s))
}

func stringMap[M ~map[string]string](m M) map[string]string {
	if len(m) == 0 {
		return nil
	}
	return maps.Clone(map[string]string(m))
}

func uintPtr(p *uint64) *uint64 {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func duration(d types.Duration) string {
	if d == 0 {
		return ""
	}
	return time.Duration(d).String()
}

func durationPtr(d *types.Duration) string {
	if d == nil {
		return ""
	}
	return time.Duration(*d).String()
}
