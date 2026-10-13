package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/client"
)

// Options configura el cliente. Todo es opcional.
type Options struct {
	// Host es el daemon (DOCKER_HOST); vacío usa DOCKER_HOST del entorno o el
	// socket por defecto (unix:///var/run/docker.sock). El CLI usa el mismo.
	Host string
	// CLI es el binario de docker para stack deploy/rm; vacío es "docker" del PATH.
	CLI string
	// TempDir es donde se crea el DOCKER_CONFIG temporal; vacío es os.TempDir().
	TempDir string
}

// Client implementa Engine con el SDK oficial (github.com/moby/moby/client)
// y el CLI de docker para los stacks. Crearlo no conecta: el primer uso
// negocia la versión de la API, así muelle arranca aunque Docker no responda.
type Client struct {
	api     *client.Client
	host    string
	cli     string
	tempDir string
}

var _ Engine = (*Client)(nil)

func New(o Options) (*Client, error) {
	opts := []client.Opt{client.FromEnv}
	if o.Host != "" {
		opts = append(opts, client.WithHost(o.Host))
	}
	api, err := client.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("docker: armando el cliente: %w", err)
	}
	cli := o.CLI
	if cli == "" {
		cli = "docker"
	}
	return &Client{api: api, host: o.Host, cli: cli, tempDir: o.TempDir}, nil
}

func (c *Client) Close() error { return c.api.Close() }

// wrap agrega la operación al error y lo marca con el error del paquete que
// corresponde, para errors.Is. El error del daemon queda envuelto también.
func wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	var kind error
	switch {
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("docker: %s: %w", op, err)
	// 503 del daemon: el nodo no está en un Swarm o no es manager. Para muelle
	// es lo mismo que Docker caído.
	case client.IsErrConnectionFailed(err) || cerrdefs.IsUnavailable(err):
		kind = ErrUnavailable
	case cerrdefs.IsNotFound(err):
		kind = ErrNotFound
	case cerrdefs.IsConflict(err) || cerrdefs.IsAlreadyExists(err):
		kind = ErrConflict
	case cerrdefs.IsUnauthorized(err) || cerrdefs.IsPermissionDenied(err):
		kind = ErrDenied
	default:
		return fmt.Errorf("docker: %s: %w", op, err)
	}
	return fmt.Errorf("docker: %s: %w: %w", op, kind, err)
}

// wrapNetwork es wrap para las redes: Docker responde 403 a "ya está
// conectado", "no está conectado" y "tiene contenedores conectados".
func wrapNetwork(op string, err error, forbiddenAs error) error {
	if err != nil && cerrdefs.IsPermissionDenied(err) {
		return fmt.Errorf("docker: %s: %w: %w", op, forbiddenAs, err)
	}
	return wrap(op, err)
}

// --- Sistema --------------------------------------------------------------------

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.api.Ping(ctx, client.PingOptions{})
	return wrap("ping", err)
}

func (c *Client) Info(ctx context.Context) (Info, error) {
	res, err := c.api.Info(ctx, client.InfoOptions{})
	if err != nil {
		return Info{}, wrap("info", err)
	}
	i := res.Info
	return Info{
		ServerVersion: i.ServerVersion,
		Hostname:      i.Name,
		NCPU:          i.NCPU,
		MemTotal:      i.MemTotal,
		SwarmState:    string(i.Swarm.LocalNodeState),
		NodeID:        i.Swarm.NodeID,
		NodeAddr:      i.Swarm.NodeAddr,
		IsManager:     i.Swarm.ControlAvailable,
	}, nil
}

func (c *Client) Node(ctx context.Context, id string) (Node, error) {
	res, err := c.api.NodeInspect(ctx, id, client.NodeInspectOptions{})
	if err != nil {
		return Node{}, wrap("inspeccionando el nodo "+id, err)
	}
	n := res.Node
	return Node{
		ID:           n.ID,
		Hostname:     n.Description.Hostname,
		Role:         string(n.Spec.Role),
		State:        string(n.Status.State),
		Availability: string(n.Spec.Availability),
		Addr:         n.Status.Addr,
	}, nil
}

func (c *Client) InitSwarm(ctx context.Context, advertiseAddr string) (string, error) {
	res, err := c.api.SwarmInit(ctx, client.SwarmInitOptions{ListenAddr: "0.0.0.0:2377", AdvertiseAddr: advertiseAddr})
	if err != nil {
		return "", wrap("iniciando el swarm", err)
	}
	return res.NodeID, nil
}

// --- Servicios y tareas ----------------------------------------------------------

func toService(s swarm.Service) Service {
	out := Service{
		ID:        s.ID,
		Name:      s.Spec.Name,
		Labels:    s.Spec.Labels,
		Version:   s.Version.Index,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
	if cs := s.Spec.TaskTemplate.ContainerSpec; cs != nil {
		out.Image = cs.Image
		if hc := cs.Healthcheck; hc != nil && len(hc.Test) > 0 && hc.Test[0] != "NONE" {
			out.HasHealthcheck = true
		}
	}
	if r := s.Spec.Mode.Replicated; r != nil && r.Replicas != nil {
		n := *r.Replicas
		out.Replicas = &n
	}
	if st := s.ServiceStatus; st != nil {
		out.RunningTasks, out.DesiredTasks = st.RunningTasks, st.DesiredTasks
	}
	if u := s.UpdateStatus; u != nil {
		out.Update = &UpdateStatus{State: string(u.State), Message: u.Message, StartedAt: u.StartedAt, CompletedAt: u.CompletedAt}
	}
	return out
}

func (c *Client) ListServices(ctx context.Context, labels map[string]string) ([]Service, error) {
	res, err := c.api.ServiceList(ctx, client.ServiceListOptions{Filters: labelFilters(labels), Status: true})
	if err != nil {
		return nil, wrap("listando servicios", err)
	}
	out := make([]Service, 0, len(res.Items))
	for _, s := range res.Items {
		out = append(out, toService(s))
	}
	slices.SortFunc(out, func(a, b Service) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// InspectService resuelve el servicio por nombre o id exacto y después lo
// lista con ServiceStatus (la inspección no trae las tareas corriendo).
func (c *Client) InspectService(ctx context.Context, nameOrID string) (Service, error) {
	res, err := c.api.ServiceInspect(ctx, nameOrID, client.ServiceInspectOptions{})
	if err != nil {
		return Service{}, wrap("inspeccionando el servicio "+nameOrID, err)
	}
	out := toService(res.Service)
	list, err := c.api.ServiceList(ctx, client.ServiceListOptions{Filters: client.Filters{}.Add("id", res.Service.ID), Status: true})
	if err != nil {
		return Service{}, wrap("inspeccionando el servicio "+nameOrID, err)
	}
	for _, s := range list.Items {
		if s.ID == res.Service.ID && s.ServiceStatus != nil {
			out.RunningTasks, out.DesiredTasks = s.ServiceStatus.RunningTasks, s.ServiceStatus.DesiredTasks
		}
	}
	return out, nil
}

func (c *Client) RemoveService(ctx context.Context, nameOrID string) error {
	_, err := c.api.ServiceRemove(ctx, nameOrID, client.ServiceRemoveOptions{})
	return wrap("borrando el servicio "+nameOrID, err)
}

func toTask(t swarm.Task) Task {
	out := Task{
		ID:           t.ID,
		ServiceID:    t.ServiceID,
		NodeID:       t.NodeID,
		Slot:         t.Slot,
		State:        string(t.Status.State),
		DesiredState: string(t.DesiredState),
		Message:      t.Status.Message,
		Err:          t.Status.Err,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.Status.Timestamp,
	}
	if cs := t.Status.ContainerStatus; cs != nil {
		out.ContainerID, out.ExitCode = cs.ContainerID, cs.ExitCode
	}
	if cs := t.Spec.ContainerSpec; cs != nil {
		out.Image = cs.Image
	}
	return out
}

// ListTasks devuelve las tareas, de la más nueva a la más vieja. El filtro
// "label" de Docker mira las etiquetas de la tarea, no las del servicio, así
// que con f.Labels se buscan primero los servicios que las tienen.
func (c *Client) ListTasks(ctx context.Context, f TaskFilter) ([]Task, error) {
	filters := client.Filters{}
	var services map[string]bool // nil: sin filtro por etiquetas
	if len(f.Labels) > 0 {
		list, err := c.ListServices(ctx, f.Labels)
		if err != nil {
			return nil, err
		}
		if len(list) == 0 {
			return []Task{}, nil
		}
		services = map[string]bool{}
		for _, s := range list {
			services[s.ID] = true
			if f.Service == "" { // con f.Service, los dos filtros "service" se sumarían
				filters.Add("service", s.ID)
			}
		}
	}
	if f.Service != "" {
		filters.Add("service", f.Service)
	}
	if f.DesiredState != "" {
		filters.Add("desired-state", f.DesiredState)
	}
	res, err := c.api.TaskList(ctx, client.TaskListOptions{Filters: filters})
	if err != nil {
		return nil, wrap("listando tareas", err)
	}
	out := make([]Task, 0, len(res.Items))
	for _, t := range res.Items {
		if services != nil && !services[t.ServiceID] {
			continue
		}
		out = append(out, toTask(t))
	}
	slices.SortFunc(out, func(a, b Task) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out, nil
}

func logArgs(o LogOptions) (tail, since string) {
	tail = "all"
	if o.Tail >= 0 {
		tail = strconv.Itoa(o.Tail)
	}
	if !o.Since.IsZero() {
		since = fmt.Sprintf("%d.%09d", o.Since.Unix(), o.Since.Nanosecond())
	}
	return tail, since
}

func (c *Client) ServiceLogs(ctx context.Context, nameOrID string, o LogOptions) (LogStream, error) {
	svc, err := c.api.ServiceInspect(ctx, nameOrID, client.ServiceInspectOptions{})
	if err != nil {
		return nil, wrap("logs del servicio "+nameOrID, err)
	}
	tty := false
	if cs := svc.Service.Spec.TaskTemplate.ContainerSpec; cs != nil {
		tty = cs.TTY
	}
	tail, since := logArgs(o)
	rc, err := c.api.ServiceLogs(ctx, svc.Service.ID, client.ServiceLogsOptions{
		ShowStdout: true, ShowStderr: true, Timestamps: true, Details: true,
		Follow: o.Follow, Tail: tail, Since: since,
	})
	if err != nil {
		return nil, wrap("logs del servicio "+nameOrID, err)
	}
	return newLogStream(rc, tty), nil
}

func (c *Client) TaskLogs(ctx context.Context, taskID string, o LogOptions) (LogStream, error) {
	task, err := c.api.TaskInspect(ctx, taskID, client.TaskInspectOptions{})
	if err != nil {
		return nil, wrap("logs de la tarea "+taskID, err)
	}
	tty := false
	if cs := task.Task.Spec.ContainerSpec; cs != nil {
		tty = cs.TTY
	}
	tail, since := logArgs(o)
	rc, err := c.api.TaskLogs(ctx, taskID, client.TaskLogsOptions{
		ShowStdout: true, ShowStderr: true, Timestamps: true, Details: true,
		Follow: o.Follow, Tail: tail, Since: since,
	})
	if err != nil {
		return nil, wrap("logs de la tarea "+taskID, err)
	}
	return newLogStream(rc, tty), nil
}

// --- Redes ------------------------------------------------------------------------

func toNetwork(n network.Network) Network {
	out := Network{
		ID: n.ID, Name: n.Name, Driver: n.Driver, Scope: n.Scope,
		Attachable: n.Attachable, Internal: n.Internal, Labels: n.Labels,
	}
	for _, cfg := range n.IPAM.Config {
		if cfg.Subnet.IsValid() {
			out.Subnets = append(out.Subnets, cfg.Subnet.String())
		}
	}
	return out
}

func (c *Client) CreateNetwork(ctx context.Context, spec NetworkSpec) (string, error) {
	res, err := c.api.NetworkCreate(ctx, spec.Name, client.NetworkCreateOptions{
		Driver: spec.Driver, Attachable: spec.Attachable, Internal: spec.Internal, Labels: spec.Labels,
	})
	if err != nil {
		return "", wrap("creando la red "+spec.Name, err)
	}
	return res.ID, nil
}

func (c *Client) InspectNetwork(ctx context.Context, nameOrID string) (Network, error) {
	res, err := c.api.NetworkInspect(ctx, nameOrID, client.NetworkInspectOptions{})
	if err != nil {
		return Network{}, wrap("inspeccionando la red "+nameOrID, err)
	}
	out := toNetwork(res.Network.Network)
	for _, ep := range res.Network.Containers {
		out.Containers = append(out.Containers, ep.Name)
	}
	slices.Sort(out.Containers)
	return out, nil
}

func (c *Client) ListNetworks(ctx context.Context, labels map[string]string) ([]Network, error) {
	res, err := c.api.NetworkList(ctx, client.NetworkListOptions{Filters: labelFilters(labels)})
	if err != nil {
		return nil, wrap("listando redes", err)
	}
	out := make([]Network, 0, len(res.Items))
	for _, n := range res.Items {
		out = append(out, toNetwork(n.Network))
	}
	slices.SortFunc(out, func(a, b Network) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// networkByText reconoce los errores de las redes de Swarm que el daemon no
// marca con un código propio y solo se distinguen por el texto (Docker 29):
//   - borrar una red con un contenedor conectado, o con las tareas de un stack
//     recién borrado: 400 "… is in use by task …";
//   - borrar una red recién borrada, que el nodo todavía conoce por nombre:
//     "network … not found";
//   - desconectar un contenedor que ya no está conectado: 500 "… is not connected to network …".
func networkByText(op string, err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "is in use by"):
		return fmt.Errorf("docker: %s: %w: %w", op, ErrConflict, err)
	case strings.Contains(msg, "is not connected to network"), strings.Contains(msg, "not found"):
		if !cerrdefs.IsNotFound(err) {
			return fmt.Errorf("docker: %s: %w: %w", op, ErrNotFound, err)
		}
	}
	return nil
}

func (c *Client) RemoveNetwork(ctx context.Context, nameOrID string) error {
	_, err := c.api.NetworkRemove(ctx, nameOrID, client.NetworkRemoveOptions{})
	op := "borrando la red " + nameOrID
	if e := networkByText(op, err); e != nil {
		return e
	}
	return wrapNetwork(op, err, ErrConflict)
}

func (c *Client) ConnectNetwork(ctx context.Context, net, ctr string, aliases []string) error {
	_, err := c.api.NetworkConnect(ctx, net, client.NetworkConnectOptions{
		Container: ctr, EndpointConfig: &network.EndpointSettings{Aliases: aliases},
	})
	return wrapNetwork("conectando "+ctr+" a "+net, err, ErrConflict)
}

func (c *Client) DisconnectNetwork(ctx context.Context, net, ctr string) error {
	_, err := c.api.NetworkDisconnect(ctx, net, client.NetworkDisconnectOptions{Container: ctr})
	op := "desconectando " + ctr + " de " + net
	if e := networkByText(op, err); e != nil {
		return e
	}
	return wrapNetwork(op, err, ErrNotFound)
}

// --- Contenedores --------------------------------------------------------------------

func (c *Client) CreateContainer(ctx context.Context, spec ContainerSpec) (string, error) {
	cfg := &container.Config{
		Image: spec.Image, Cmd: spec.Cmd, Env: spec.Env, Labels: spec.Labels,
		ExposedPorts: network.PortSet{},
	}
	host := &container.HostConfig{
		PortBindings:  network.PortMap{},
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyMode(spec.RestartPolicy)},
	}
	for _, p := range spec.Ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		port, err := network.ParsePort(fmt.Sprintf("%d/%s", p.ContainerPort, proto))
		if err != nil {
			return "", fmt.Errorf("docker: el puerto %d/%s no es válido: %w", p.ContainerPort, proto, err)
		}
		b := network.PortBinding{HostPort: strconv.Itoa(p.HostPort)}
		if p.HostIP != "" {
			ip, err := netip.ParseAddr(p.HostIP)
			if err != nil {
				return "", fmt.Errorf("docker: la IP %q no es válida", p.HostIP)
			}
			b.HostIP = ip
		}
		cfg.ExposedPorts[port] = struct{}{}
		host.PortBindings[port] = append(host.PortBindings[port], b)
	}
	for _, m := range spec.Mounts {
		host.Mounts = append(host.Mounts, mount.Mount{Type: mount.TypeVolume, Source: m.Volume, Target: m.Target, ReadOnly: m.ReadOnly})
	}
	var netCfg *network.NetworkingConfig
	if spec.Network != "" {
		netCfg = &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{
			spec.Network: {Aliases: spec.NetworkAliases},
		}}
	}
	res, err := c.api.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: spec.Name, Config: cfg, HostConfig: host, NetworkingConfig: netCfg,
	})
	if err != nil {
		return "", wrap("creando el contenedor "+spec.Name, err)
	}
	return res.ID, nil
}

func (c *Client) InspectContainer(ctx context.Context, nameOrID string) (Container, error) {
	res, err := c.api.ContainerInspect(ctx, nameOrID, client.ContainerInspectOptions{})
	if err != nil {
		return Container{}, wrap("inspeccionando el contenedor "+nameOrID, err)
	}
	r := res.Container
	out := Container{ID: r.ID, Name: strings.TrimPrefix(r.Name, "/"), ImageID: r.Image}
	if r.Config != nil {
		out.Image, out.Cmd, out.Labels = r.Config.Image, r.Config.Cmd, r.Config.Labels
	}
	if s := r.State; s != nil {
		out.Status, out.Running = string(s.Status), s.Running
		if s.Health != nil {
			out.Health = string(s.Health.Status)
		}
		if t, err := time.Parse(time.RFC3339Nano, s.StartedAt); err == nil && t.Year() > 1 {
			out.StartedAt = t.UTC()
		}
	}
	if h := r.HostConfig; h != nil {
		out.RestartPolicy = string(h.RestartPolicy.Name)
		for port, bindings := range h.PortBindings {
			for _, b := range bindings {
				hp, _ := strconv.Atoi(b.HostPort)
				pb := PortBinding{HostPort: hp, ContainerPort: int(port.Num()), Protocol: string(port.Proto())}
				if b.HostIP.IsValid() {
					pb.HostIP = b.HostIP.String()
				}
				out.Ports = append(out.Ports, pb)
			}
		}
		slices.SortFunc(out.Ports, func(a, b PortBinding) int {
			return strings.Compare(fmt.Sprintf("%05d/%s", a.ContainerPort, a.Protocol), fmt.Sprintf("%05d/%s", b.ContainerPort, b.Protocol))
		})
	}
	for _, m := range r.Mounts {
		if m.Type == mount.TypeVolume {
			out.Mounts = append(out.Mounts, Mount{Volume: m.Name, Target: m.Destination, ReadOnly: !m.RW})
		}
	}
	if ns := r.NetworkSettings; ns != nil {
		for name := range ns.Networks {
			out.Networks = append(out.Networks, name)
		}
		slices.Sort(out.Networks)
	}
	return out, nil
}

func (c *Client) StartContainer(ctx context.Context, nameOrID string) error {
	_, err := c.api.ContainerStart(ctx, nameOrID, client.ContainerStartOptions{})
	return wrap("arrancando el contenedor "+nameOrID, err)
}

func (c *Client) StopContainer(ctx context.Context, nameOrID string, timeout time.Duration) error {
	secs := int(timeout / time.Second)
	_, err := c.api.ContainerStop(ctx, nameOrID, client.ContainerStopOptions{Timeout: &secs})
	return wrap("deteniendo el contenedor "+nameOrID, err)
}

func (c *Client) RemoveContainer(ctx context.Context, nameOrID string, force bool) error {
	_, err := c.api.ContainerRemove(ctx, nameOrID, client.ContainerRemoveOptions{Force: force})
	return wrap("borrando el contenedor "+nameOrID, err)
}

func (c *Client) ContainerStats(ctx context.Context, nameOrID string) (Stats, error) {
	res, err := c.api.ContainerStats(ctx, nameOrID, client.ContainerStatsOptions{IncludePreviousSample: true})
	if err != nil {
		return Stats{}, wrap("stats de "+nameOrID, err)
	}
	defer res.Body.Close()
	var s container.StatsResponse
	if err := json.NewDecoder(res.Body).Decode(&s); err != nil {
		return Stats{}, fmt.Errorf("docker: stats de %s: %w", nameOrID, err)
	}
	return computeStats(s), nil
}

// --- Volúmenes ---------------------------------------------------------------------

func (c *Client) ListVolumes(ctx context.Context, labels map[string]string) ([]Volume, error) {
	res, err := c.api.VolumeList(ctx, client.VolumeListOptions{Filters: labelFilters(labels)})
	if err != nil {
		return nil, wrap("listando volúmenes", err)
	}
	out := make([]Volume, 0, len(res.Items))
	for _, v := range res.Items {
		out = append(out, Volume{Name: v.Name, Driver: v.Driver, Labels: v.Labels})
	}
	slices.SortFunc(out, func(a, b Volume) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func (c *Client) RemoveVolume(ctx context.Context, name string, force bool) error {
	_, err := c.api.VolumeRemove(ctx, name, client.VolumeRemoveOptions{Force: force})
	return wrap("borrando el volumen "+name, err)
}

// --- Imágenes ------------------------------------------------------------------------

func (c *Client) InspectImage(ctx context.Context, ref string) (Image, error) {
	res, err := c.api.ImageInspect(ctx, ref)
	if err != nil {
		return Image{}, wrap("inspeccionando la imagen "+ref, err)
	}
	return Image{ID: res.ID, RepoTags: res.RepoTags, RepoDigests: res.RepoDigests}, nil
}

func (c *Client) DistributionDigest(ctx context.Context, ref string, auth *RegistryAuth) (string, error) {
	enc, err := encodeRegistryAuth(auth)
	if err != nil {
		return "", fmt.Errorf("docker: credenciales de %s: %w", ref, err)
	}
	res, err := c.api.DistributionInspect(ctx, ref, client.DistributionInspectOptions{EncodedRegistryAuth: enc})
	if err != nil {
		return "", wrap("resolviendo "+ref, err)
	}
	return res.Descriptor.Digest.String(), nil
}

func (c *Client) PullImage(ctx context.Context, ref string, auth *RegistryAuth) error {
	enc, err := encodeRegistryAuth(auth)
	if err != nil {
		return fmt.Errorf("docker: credenciales de %s: %w", ref, err)
	}
	res, err := c.api.ImagePull(ctx, ref, client.ImagePullOptions{RegistryAuth: enc})
	if err != nil {
		return wrap("bajando "+ref, err)
	}
	defer res.Close()
	return wrap("bajando "+ref, res.Wait(ctx))
}

// --- Secretos ------------------------------------------------------------------------

func (c *Client) CreateSecret(ctx context.Context, name string, data []byte, labels map[string]string) (string, error) {
	res, err := c.api.SecretCreate(ctx, client.SecretCreateOptions{Spec: swarm.SecretSpec{
		Annotations: swarm.Annotations{Name: name, Labels: labels},
		Data:        data,
	}})
	if err != nil {
		return "", wrap("creando el secreto "+name, err)
	}
	return res.ID, nil
}

func (c *Client) InspectSecret(ctx context.Context, name string) (Secret, error) {
	res, err := c.api.SecretInspect(ctx, name, client.SecretInspectOptions{})
	if err != nil {
		return Secret{}, wrap("inspeccionando el secreto "+name, err)
	}
	s := res.Secret
	return Secret{ID: s.ID, Name: s.Spec.Name, Labels: s.Spec.Labels, CreatedAt: s.CreatedAt}, nil
}

func (c *Client) RemoveSecret(ctx context.Context, name string) error {
	_, err := c.api.SecretRemove(ctx, name, client.SecretRemoveOptions{})
	return wrap("borrando el secreto "+name, err)
}
