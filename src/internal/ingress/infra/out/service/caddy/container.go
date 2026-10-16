package caddy

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

// stopTimeout es lo que espera StopContainer antes de matarlo: Caddy cierra
// bien y suelta el socket de la admin API.
const stopTimeout = 10 * time.Second

// probeSuffix es el sufijo del contenedor descartable con que se prueban los
// puertos antes de crear muelle-caddy (muelle-caddy-probe).
const probeSuffix = "-probe"

// Names son los nombres de Docker de muelle-caddy: los de naming en
// producción, otros en los tests de integración (ingress.Options).
type Names struct {
	Container, PanelNetwork               string
	DataVolume, ConfigVolume, AdminVolume string
}

// Engine es lo que Runtime usa del cliente de Docker compartido.
type Engine interface {
	docker.Networks
	docker.Containers
	docker.Images
}

// Runtime es el contenedor muelle-caddy (contrato §3.4, spec §4.2): lo crea,
// lo recrea (nunca lo rearranca) y lo conecta a las redes. No es seguro
// llamar a Ensure ni a Connect desde varias goroutines a la vez: los
// serializa quien los usa (usecases.EnsureCaddy).
type Runtime struct {
	docker Engine
	names  Names
	log    *slog.Logger
}

var _ service.CaddyRuntime = (*Runtime)(nil)

func NewRuntime(d Engine, n Names, log *slog.Logger) *Runtime {
	return &Runtime{docker: d, names: n, log: log}
}

// spec es el contenedor que pide want (contrato §3.4). Nunca lleva la
// credencial: esa va solo al PullImage.
func (r *Runtime) spec(want service.CaddyWant) docker.ContainerSpec {
	return docker.ContainerSpec{
		Name:  r.names.Container,
		Image: want.Image,
		Cmd:   naming.CaddyCommand(),
		Env:   naming.CaddyEnv(),
		Ports: []docker.PortBinding{
			{HostPort: want.HTTPPort, ContainerPort: 80, Protocol: "tcp"},
			{HostPort: want.HTTPSPort, ContainerPort: 443, Protocol: "tcp"},
			{HostPort: want.HTTPSPort, ContainerPort: 443, Protocol: "udp"},
		},
		Mounts: []docker.Mount{
			{Volume: r.names.DataVolume, Target: naming.CaddyDataDir},
			{Volume: r.names.ConfigVolume, Target: naming.CaddyConfigDir},
			{Volume: r.names.AdminVolume, Target: naming.CaddyAdminDir},
		},
		Network:       r.names.PanelNetwork,
		RestartPolicy: "unless-stopped",
	}
}

// Ensure deja muelle-caddy como pide want (contrato §3.4), en este orden:
//  1. asegura la red panel;
//  2. si el contenedor existe y está como se pide (mismatch vacío), lo
//     conecta a las redes que le faltan (la panel y las de stage) y vuelve;
//  3. si no, lo recrea (D11 del plan de la Fase 3): baja la imagen si falta
//     (si no baja, el contenedor anterior queda intacto), lo detiene si
//     corría, lo borra, prueba los puertos (probePorts), crea uno nuevo, lo
//     conecta a las redes de stage y lo arranca. Con un puerto del host
//     ocupado, borra el que no arrancó.
//
// Nunca registra want, la credencial ni el RegistryAuth; sus errores nombran
// la imagen, no la credencial.
func (r *Runtime) Ensure(ctx context.Context, want service.CaddyWant) (bool, error) {
	if err := r.ensurePanelNetwork(ctx); err != nil {
		return false, err
	}
	spec := r.spec(want)
	c, err := r.docker.InspectContainer(ctx, r.names.Container)
	exists := err == nil
	if err != nil && !errors.Is(err, docker.ErrNotFound) {
		return false, err
	}
	reason := "no existe"
	if exists {
		reason = mismatch(c, spec)
		if reason == "" {
			return false, r.connect(ctx, c.Networks, append([]string{r.names.PanelNetwork}, want.Networks...))
		}
	}

	if err := r.ensureImage(ctx, want); err != nil {
		return false, err
	}
	r.log.Info("ingress: creando muelle-caddy", "contenedor", r.names.Container, "imagen", want.Image, "motivo", reason)
	if exists {
		if c.Running {
			if err := r.docker.StopContainer(ctx, r.names.Container, stopTimeout); err != nil {
				r.log.Warn("ingress: no se pudo detener muelle-caddy; se borra igual", "contenedor", r.names.Container, "error", err)
			}
		}
		if err := r.docker.RemoveContainer(ctx, r.names.Container, true); err != nil && !errors.Is(err, docker.ErrNotFound) {
			return false, fmt.Errorf("borrando muelle-caddy: %w", err)
		}
	}
	if err := r.probePorts(ctx, spec); err != nil {
		return false, err
	}
	if _, err := r.docker.CreateContainer(ctx, spec); err != nil {
		return false, fmt.Errorf("creando muelle-caddy con %s: %w", want.Image, err)
	}
	if err := r.connect(ctx, []string{r.names.PanelNetwork}, want.Networks); err != nil {
		return false, err
	}
	if err := r.docker.StartContainer(ctx, r.names.Container); err != nil {
		if errors.Is(err, docker.ErrPortInUse) {
			// Alguien tomó el puerto después de la prueba. Docker 29 lo
			// arrancaría sin puertos si alguien lo vuelve a arrancar: se
			// borra ya, y el job "proxy" lo crea de nuevo.
			if rmErr := r.docker.RemoveContainer(ctx, r.names.Container, true); rmErr != nil && !errors.Is(rmErr, docker.ErrNotFound) {
				r.log.Warn("ingress: no se pudo borrar muelle-caddy después del puerto ocupado", "contenedor", r.names.Container, "error", rmErr)
			}
			return false, fmt.Errorf("%w: %w", service.ErrPortInUse, err)
		}
		return false, fmt.Errorf("arrancando muelle-caddy: %w", err)
	}
	return true, nil
}

// Connect conecta muelle-caddy a las redes de networks que le faltan, sin
// recrearlo. Una red que no existe se registra y se salta, como en Ensure.
func (r *Runtime) Connect(ctx context.Context, networks []string) error {
	c, err := r.docker.InspectContainer(ctx, r.names.Container)
	if err != nil {
		return err
	}
	return r.connect(ctx, c.Networks, networks)
}

// Status es lo que corre de verdad (GET /proxy): la imagen con que se creó y
// si corre. found false si el contenedor no existe.
func (r *Runtime) Status(ctx context.Context) (image string, running, found bool, err error) {
	c, err := r.docker.InspectContainer(ctx, r.names.Container)
	if errors.Is(err, docker.ErrNotFound) {
		return "", false, false, nil
	}
	if err != nil {
		return "", false, false, err
	}
	return c.Image, c.Running, true, nil
}

// probePorts prueba que los puertos del host de spec estén libres, con un
// contenedor descartable (<contenedor>-probe) de la misma imagen, en la red
// bridge por defecto y sin redes de Swarm, que corre "caddy version" y se
// borra enseguida. Hace falta porque Docker (29) no suelta la conexión de
// Swarm de la red con que se creó un contenedor cuyo arranque falla por un
// puerto ocupado: queda una tarea "attachment" que no se borra, con una IP de
// la red panel, y la red ya no se puede borrar. Con un puerto ocupado, el job
// "proxy" reintentaría cada 5 minutos y se comería la red panel en un día.
// Devuelve ErrPortInUse si un puerto está ocupado; si la prueba falla por
// otra cosa (sin la red bridge, por ejemplo), lo registra y sigue: la prueba
// es una protección, no un requisito.
func (r *Runtime) probePorts(ctx context.Context, spec docker.ContainerSpec) error {
	name := r.names.Container + probeSuffix
	remove := func() {
		err := r.docker.RemoveContainer(context.WithoutCancel(ctx), name, true)
		if err != nil && !errors.Is(err, docker.ErrNotFound) {
			r.log.Warn("ingress: no se pudo borrar el contenedor de prueba de los puertos", "contenedor", name, "error", err)
		}
	}
	remove() // uno que quedó de una prueba cortada
	_, err := r.docker.CreateContainer(ctx, docker.ContainerSpec{
		Name: name, Image: spec.Image, Cmd: []string{"caddy", "version"}, Ports: spec.Ports,
	})
	if err != nil {
		if errors.Is(err, entity.ErrDockerUnavailable) {
			return err
		}
		r.log.Warn("ingress: no se pudieron probar los puertos de muelle-caddy; se crea igual", "error", err)
		return nil
	}
	defer remove()
	if err := r.docker.StartContainer(ctx, name); err != nil {
		switch {
		case errors.Is(err, docker.ErrPortInUse):
			return fmt.Errorf("%w: %w", service.ErrPortInUse, err)
		case errors.Is(err, entity.ErrDockerUnavailable):
			return err
		}
		r.log.Warn("ingress: no se pudieron probar los puertos de muelle-caddy; se crea igual", "error", err)
	}
	return nil
}

// ensurePanelNetwork crea la red panel (overlay attachable) si falta.
// Normalmente la crea install (Fase 5).
func (r *Runtime) ensurePanelNetwork(ctx context.Context) error {
	_, err := r.docker.InspectNetwork(ctx, r.names.PanelNetwork)
	if err == nil {
		return nil
	}
	if !errors.Is(err, docker.ErrNotFound) {
		return err
	}
	_, err = r.docker.CreateNetwork(ctx, docker.NetworkSpec{Name: r.names.PanelNetwork, Driver: "overlay", Attachable: true})
	if err != nil && !errors.Is(err, docker.ErrConflict) {
		return fmt.Errorf("creando la red %s: %w", r.names.PanelNetwork, err)
	}
	return nil
}

// ensureImage baja la imagen si no está en el nodo, con la credencial de want
// si hay (anónima si no). Un pull que falla es ErrCaddyImagePull, salvo
// Docker caído, que vuelve tal cual (Docker no responde, no la imagen).
func (r *Runtime) ensureImage(ctx context.Context, want service.CaddyWant) error {
	_, err := r.docker.InspectImage(ctx, want.Image)
	if err == nil {
		return nil
	}
	if !errors.Is(err, docker.ErrNotFound) {
		return err
	}
	var auth *docker.RegistryAuth
	if want.Auth != nil {
		auth = &docker.RegistryAuth{Host: want.Auth.Host, Username: want.Auth.Username, Password: want.Auth.Password}
	}
	r.log.Info("ingress: bajando la imagen de muelle-caddy", "imagen", want.Image, "con_credencial", auth != nil)
	if err := r.docker.PullImage(ctx, want.Image, auth); err != nil {
		if errors.Is(err, entity.ErrDockerUnavailable) {
			return err
		}
		return fmt.Errorf("%w %s: %w", service.ErrCaddyImagePull, want.Image, err)
	}
	return nil
}

// connect conecta muelle-caddy a cada red de want que no esté en have. Ya
// conectado (ErrConflict) no es un error. Una red que ya no existe (un stage
// que se borró entre la lectura y ahora) se registra y se salta (D11).
func (r *Runtime) connect(ctx context.Context, have, want []string) error {
	for _, n := range want {
		if slices.Contains(have, n) {
			continue
		}
		err := r.docker.ConnectNetwork(ctx, n, r.names.Container, nil)
		switch {
		case err == nil, errors.Is(err, docker.ErrConflict):
		case errors.Is(err, docker.ErrNotFound):
			r.log.Warn("ingress: la red no existe; muelle-caddy no se conecta", "red", n, "contenedor", r.names.Container)
		default:
			return fmt.Errorf("conectando muelle-caddy a %s: %w", n, err)
		}
	}
	return nil
}

// mismatch dice por qué el contenedor c no es el que pide spec ("" si es):
// otra imagen, otro comando, otros puertos u otros volúmenes, detenido, o
// corriendo sin publicar sus puertos (Docker 29, ver StartContainer).
// Los puertos y los volúmenes se comparan como conjuntos.
func mismatch(c docker.Container, spec docker.ContainerSpec) string {
	switch {
	case c.Image != spec.Image:
		return "otra imagen (" + c.Image + ")"
	case !slices.Equal(c.Cmd, spec.Cmd):
		return "otro comando"
	case !slices.Equal(portKeys(c.Ports), portKeys(spec.Ports)):
		return "otros puertos"
	case !slices.Equal(mountKeys(c.Mounts), mountKeys(spec.Mounts)):
		return "otros volúmenes"
	case !c.Running:
		return "no está corriendo (" + c.Status + ")"
	case len(c.Ports) > 0 && len(c.PublishedPorts) == 0:
		return "corre sin publicar sus puertos"
	}
	return ""
}

func portKeys(ports []docker.PortBinding) []string {
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		out = append(out, p.HostIP+"|"+strconv.Itoa(p.HostPort)+"|"+strconv.Itoa(p.ContainerPort)+"/"+cmp.Or(p.Protocol, "tcp"))
	}
	slices.Sort(out)
	return out
}

func mountKeys(mounts []docker.Mount) []string {
	out := make([]string, 0, len(mounts))
	for _, m := range mounts {
		out = append(out, m.Volume+"|"+m.Target+"|"+strconv.FormatBool(m.ReadOnly))
	}
	slices.Sort(out)
	return out
}
