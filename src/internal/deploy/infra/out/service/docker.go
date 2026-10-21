package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

// DockerReader es lo que Docker usa del cliente compartido.
type DockerReader interface {
	docker.System
	docker.Services
	docker.Images
	docker.Containers
}

// Docker implementa service.Images y service.Cluster con el cliente
// compartido: traduce sus tipos a los del puerto y sus errores
// (docker.ErrNotFound, docker.ErrDenied) a los de deploy. docker.ErrUnavailable
// pasa tal cual: es entity.ErrDockerUnavailable.
type Docker struct {
	docker DockerReader
}

var (
	_ service.Images  = (*Docker)(nil)
	_ service.Cluster = (*Docker)(nil)
)

func NewDocker(d DockerReader) *Docker { return &Docker{docker: d} }

// Digest consulta el registry (GET /distribution/{ref}/json), con la
// credencial de cred o anónimo.
func (d *Docker) Digest(ctx context.Context, ref string, cred *service.Credential) (string, error) {
	var auth *docker.RegistryAuth
	if cred != nil {
		auth = &docker.RegistryAuth{Host: cred.Host, Username: cred.Username, Password: cred.Password}
	}
	dg, err := d.docker.DistributionDigest(ctx, ref, auth)
	switch {
	case err == nil:
		return dg, nil
	case errors.Is(err, docker.ErrNotFound):
		return "", fmt.Errorf("%w: %w", service.ErrImageNotFound, err)
	case errors.Is(err, docker.ErrDenied):
		return "", fmt.Errorf("%w: %w", service.ErrImageDenied, err)
	}
	return "", err
}

func (d *Docker) AppServices(ctx context.Context, project, stage, app string) ([]service.SwarmService, error) {
	list, err := d.docker.ListServices(ctx, naming.AppLabels(project, stage, app))
	if err != nil {
		return nil, err
	}
	out := make([]service.SwarmService, len(list))
	for i, s := range list {
		out[i] = toSwarmService(s)
	}
	slices.SortFunc(out, func(a, b service.SwarmService) int { return cmp.Compare(a.Name, b.Name) })
	return out, nil
}

func (d *Docker) Service(ctx context.Context, name string) (service.SwarmService, error) {
	s, err := d.docker.InspectService(ctx, name)
	if err != nil {
		return service.SwarmService{}, swarmErr(err)
	}
	return toSwarmService(s), nil
}

func (d *Docker) Tasks(ctx context.Context, serviceID string) ([]service.SwarmTask, error) {
	list, err := d.docker.ListTasks(ctx, docker.TaskFilter{Service: serviceID})
	if err != nil {
		return nil, swarmErr(err)
	}
	out := make([]service.SwarmTask, len(list))
	for i, t := range list {
		out[i] = service.SwarmTask{
			ID: t.ID, ServiceID: t.ServiceID, NodeID: t.NodeID, Slot: t.Slot,
			State: t.State, DesiredState: t.DesiredState, Err: t.Err, ContainerID: t.ContainerID,
			CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
		}
	}
	// De la más nueva a la más vieja, sin depender del orden del daemon.
	slices.SortStableFunc(out, func(a, b service.SwarmTask) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out, nil
}

// TaskLogs lee las últimas tail líneas y cierra el stream.
func (d *Docker) TaskLogs(ctx context.Context, taskID string, tail int) ([]service.LogLine, error) {
	st, err := d.docker.TaskLogs(ctx, taskID, docker.LogOptions{Tail: tail})
	if err != nil {
		return nil, swarmErr(err)
	}
	defer st.Close()
	var out []service.LogLine
	for {
		l, err := st.Next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, service.LogLine{Time: l.Time, Stream: l.Stream, TaskID: l.TaskID, Text: l.Text})
	}
}

// LocalNodeID es el id del nodo local; sin Swarm activo, para muelle es lo
// mismo que Docker caído.
func (d *Docker) LocalNodeID(ctx context.Context) (string, error) {
	info, err := d.docker.Info(ctx)
	if err != nil {
		return "", err
	}
	if info.SwarmState != docker.SwarmActive || info.NodeID == "" {
		return "", fmt.Errorf("deploy: el nodo no está en un Swarm activo (%s): %w", info.SwarmState, docker.ErrUnavailable)
	}
	return info.NodeID, nil
}

// ServiceLogs abre el log del servicio; las líneas pasan a los tipos del puerto.
func (d *Docker) ServiceLogs(ctx context.Context, serviceID string, o service.LogOptions) (service.LogStream, error) {
	st, err := d.docker.ServiceLogs(ctx, serviceID, docker.LogOptions{Tail: o.Tail, Follow: o.Follow, Since: o.Since})
	if err != nil {
		return nil, swarmErr(err)
	}
	return logStream{st}, nil
}

// logStream es un docker.LogStream con las líneas del puerto.
type logStream struct{ s docker.LogStream }

func (l logStream) Next() (service.LogLine, error) {
	x, err := l.s.Next()
	if err != nil {
		return service.LogLine{}, err
	}
	return service.LogLine{Time: x.Time, Stream: x.Stream, TaskID: x.TaskID, Text: x.Text}, nil
}

func (l logStream) Close() error { return l.s.Close() }

func (d *Docker) NodeHostname(ctx context.Context, nodeID string) (string, error) {
	n, err := d.docker.Node(ctx, nodeID)
	if err != nil {
		return "", swarmErr(err)
	}
	return n.Hostname, nil
}

func (d *Docker) ContainerHealth(ctx context.Context, containerID string) (string, error) {
	c, err := d.docker.InspectContainer(ctx, containerID)
	if err != nil {
		return "", swarmErr(err)
	}
	return c.Health, nil
}

func (d *Docker) ContainerStats(ctx context.Context, containerID string) (service.Stats, error) {
	s, err := d.docker.ContainerStats(ctx, containerID)
	if err != nil {
		return service.Stats{}, swarmErr(err)
	}
	return service.Stats{CPUPercent: s.CPUPercent, MemoryBytes: s.MemoryBytes}, nil
}

func swarmErr(err error) error {
	if errors.Is(err, docker.ErrNotFound) {
		return fmt.Errorf("%w: %w", service.ErrSwarmNotFound, err)
	}
	return err
}

func toSwarmService(s docker.Service) service.SwarmService {
	out := service.SwarmService{
		ID: s.ID, Name: s.Name, Labels: s.Labels, Image: s.Image, Replicas: s.Replicas,
		RunningTasks: s.RunningTasks, DesiredTasks: s.DesiredTasks, MemoryLimitBytes: s.MemoryLimitBytes,
		CreatedAt: s.CreatedAt,
	}
	if u := s.Update; u != nil {
		out.Update = &service.UpdateStatus{State: u.State, Message: u.Message, StartedAt: u.StartedAt}
	}
	return out
}
