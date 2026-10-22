package install

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

var (
	ErrAlreadyInstalled = errors.New("muelle ya está instalado")
	ErrPreviousData     = errors.New("hay datos de una instalación anterior")
	ErrPortsBusy        = errors.New("los puertos del proxy están ocupados")
)

const (
	defaultSetupTimeout = 2 * time.Minute
	defaultPoll         = time.Second
	defaultNetworkWait  = 5 * time.Second

	// probeTimeout acota el borrado del contenedor de prueba, que se hace
	// siempre, también si install se corta (con un ctx sin cancelación).
	probeTimeout = 30 * time.Second
	// undoTimeout es el plazo de todo el deshacer del paso 8, y undoWait lo
	// que espera cada paso a que Docker suelte lo que borra (el servicio, un
	// volumen en uso, la red con tareas).
	undoTimeout = 60 * time.Second
	undoWait    = 30 * time.Second
	retryEvery  = 500 * time.Millisecond
)

// Config son las dependencias de Run; los tests acortan los plazos y cambian la red.
type Config struct {
	Version        string // main.version
	Docker         docker.Engine
	Stdout, Stderr io.Writer
	DetectIP       func(ctx context.Context) (string, error) // nil: detectPublicIP con traceURL
	SetupTimeout   time.Duration                             // 0: 2 min
	Poll           time.Duration                             // 0: 1 s
	NetworkWait    time.Duration                             // 0: 5 s (las subredes de una red recién creada)
	// DockerConfig es el config.json de Docker de donde salen las
	// credenciales de las imágenes si no vinieron por flags. Vacío es
	// $DOCKER_CONFIG/config.json o ~/.docker/config.json: en la imagen,
	// /root/.docker/config.json, el que se monta con -v.
	DockerConfig string
}

func (c Config) withDefaults() Config {
	c.SetupTimeout = cmp.Or(c.SetupTimeout, defaultSetupTimeout)
	c.Poll = cmp.Or(c.Poll, defaultPoll)
	c.NetworkWait = cmp.Or(c.NetworkWait, defaultNetworkWait)
	if c.DetectIP == nil {
		client := &http.Client{Timeout: traceTimeout}
		c.DetectIP = func(ctx context.Context) (string, error) { return detectPublicIP(ctx, client, traceURL) }
	}
	if c.DockerConfig == "" {
		c.DockerConfig = defaultDockerConfig()
	}
	if c.Stdout == nil {
		c.Stdout = io.Discard
	}
	if c.Stderr == nil {
		c.Stderr = io.Discard
	}
	return c
}

// installer es una corrida de install: lo que se averigua en cada paso.
type installer struct {
	o     Options
	c     Config
	d     docker.Engine
	names naming.InstallNames

	image, caddyImage string
	creds             map[string]*docker.RegistryAuth // por host
	info              docker.Info
	prev              previous
	publicIP          string
	nodeID            string
}

// previous es lo que quedó de una instalación anterior (paso 3).
type previous struct {
	masterKey, tokenSecret, registrySecret bool
	dataVolume, caddyAdminVolume           bool
	caddyContainer                         bool
	network                                bool // names.PanelNetwork existe (y sirve)
}

// Run instala muelle con los pasos del contrato §5.4, en ese orden: nada
// cambia antes de la prueba de puertos. Devuelve nil si imprimió el link de
// setup (o muelle ya tenía usuarios). Los errores ya traen el mensaje para
// quien corre install.
func Run(ctx context.Context, o Options, c Config) error {
	o.NamePrefix = cmp.Or(o.NamePrefix, defaultPrefix)
	o.HTTPPort = cmp.Or(o.HTTPPort, 80)
	o.HTTPSPort = cmp.Or(o.HTTPSPort, 443)
	// Paso 1: ParseArgs ya validó; esto cubre a quien arma Options a mano.
	if err := o.validate(); err != nil {
		return err
	}
	if (o.RegistryUser == "") != (o.RegistryPassword == "") {
		return errors.New("--registry-user y --registry-password-stdin van juntos")
	}
	in := &installer{o: o, c: c.withDefaults(), d: c.Docker, names: naming.InstallNamesFor(o.NamePrefix)}
	return in.run(ctx)
}

func (in *installer) run(ctx context.Context) error {
	// La imagen y las credenciales no tocan Docker: un binario sin versión
	// publicada, o un config.json que no sirve, abortan antes de todo.
	in.image = cmp.Or(in.o.Image, naming.MuelleImageFor(in.c.Version))
	if in.image == "" {
		return fmt.Errorf("este binario de muelle (%s) no es una versión publicada y no tiene una imagen por defecto: pasa la imagen con --image", in.c.Version)
	}
	in.caddyImage = cmp.Or(in.o.CaddyImage, naming.CaddyImageFor(in.c.Version))
	creds, err := resolveCredentials(in.o, in.c.DockerConfig, in.image, in.caddyImage)
	if err != nil {
		return err
	}
	in.creds = creds

	if err := in.checkDocker(ctx); err != nil { // paso 2
		return err
	}
	if err := in.checkPrevious(ctx); err != nil { // paso 3
		return err
	}
	in.detectIP(ctx)                             // paso 4
	if err := in.ensureImages(ctx); err != nil { // paso 5
		return err
	}
	if err := in.probePorts(ctx); err != nil { // paso 6
		return err
	}
	if in.o.CloudflareToken == "" && in.o.HTTPPort != 80 && !in.prev.dataVolume {
		in.warnf("sin --cloudflare-token, Let's Encrypt valida por HTTP-01 en el puerto 80 público: con --http-port %d, solo funciona si el 80 de tu router o firewall llega al %d de este servidor", in.o.HTTPPort, in.o.HTTPPort)
	}
	if err := in.initSwarm(ctx); err != nil { // paso 7
		return err
	}
	key, err := in.create(ctx) // paso 8
	if err != nil {
		return err
	}
	return in.finish(ctx, key) // paso 9
}

func (in *installer) logf(format string, args ...any) {
	fmt.Fprintf(in.c.Stderr, format+"\n", args...)
}

func (in *installer) warnf(format string, args ...any) {
	fmt.Fprintf(in.c.Stderr, "aviso: "+format+"\n", args...)
}

// credFor es la credencial del host de ref, o nil (anónima).
func (in *installer) credFor(ref string) *docker.RegistryAuth {
	h, err := registryHost(ref)
	if err != nil {
		return nil
	}
	return in.creds[h]
}

// --- Paso 2: Docker y el Swarm --------------------------------------------------------

func (in *installer) checkDocker(ctx context.Context) error {
	info, err := in.d.Info(ctx)
	if err != nil {
		return fmt.Errorf("Docker no responde: install necesita su socket, montado con docker run -v /var/run/docker.sock:/var/run/docker.sock (%w)", err)
	}
	in.info = info
	switch info.SwarmState {
	case docker.SwarmActive:
		if !info.IsManager {
			return errors.New("este nodo es un worker del Swarm: corre install en un nodo manager")
		}
		in.logf("Docker %s, Swarm activo (nodo %s)", info.ServerVersion, info.NodeID)
		if in.o.AdvertiseAddr != "" {
			in.warnf("--advertise-addr se ignora: el Swarm ya está activo")
		}
	case docker.SwarmInactive:
		in.logf("Docker %s, sin Swarm: se inicia después de probar los puertos", info.ServerVersion)
	case docker.SwarmPending:
		return errors.New("el nodo se está uniendo a un Swarm (pending): espera a que termine, o sal con docker swarm leave, y repite")
	case docker.SwarmLocked:
		return errors.New("el Swarm está bloqueado (autolock): desbloquéalo con docker swarm unlock y repite")
	case docker.SwarmError:
		return errors.New("el Swarm del nodo está en error: revisa docker info")
	default:
		return fmt.Errorf("el Swarm del nodo está en un estado desconocido (%q): revisa docker info", info.SwarmState)
	}
	return nil
}

func (in *installer) swarmActive() bool { return in.info.SwarmState == docker.SwarmActive }

// --- Paso 3: una instalación anterior y la red ----------------------------------------

// exists convierte el error de un Inspect en "existe": ErrNotFound es que no.
func exists(err error) (bool, error) {
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, docker.ErrNotFound):
		return false, nil
	default:
		return false, err
	}
}

func (in *installer) checkPrevious(ctx context.Context) error {
	n, o := in.names, in.o
	var err error
	// Con el Swarm inactivo no hay servicios ni secretos (el daemon respondería 503).
	if in.swarmActive() {
		svc, err := exists(inspectService(ctx, in.d, n.Service))
		if err != nil {
			return fmt.Errorf("buscando el servicio %s: %w", n.Service, err)
		}
		if svc {
			return fmt.Errorf("%w (el servicio %s existe): para actualizarlo, docker service update --with-registry-auth --image %s:<versión> %s",
				ErrAlreadyInstalled, n.Service, naming.MuelleImage, n.Service)
		}
		secrets := []struct {
			name string
			look bool
			dst  *bool
		}{
			{n.MasterKeySecret, true, &in.prev.masterKey},
			{n.InitCloudflareTokenSecret, o.CloudflareToken != "" || o.Reinstall, &in.prev.tokenSecret},
			{n.InitRegistrySecret, in.credFor(in.image) != nil || o.Reinstall, &in.prev.registrySecret},
		}
		for _, s := range secrets {
			if !s.look {
				continue
			}
			if *s.dst, err = exists(inspectSecret(ctx, in.d, s.name)); err != nil {
				return fmt.Errorf("buscando el secreto %s: %w", s.name, err)
			}
		}
	}
	vols, err := in.d.ListVolumes(ctx, nil)
	if err != nil {
		return fmt.Errorf("listando los volúmenes: %w", err)
	}
	for _, v := range vols {
		in.prev.dataVolume = in.prev.dataVolume || v.Name == n.DataVolume
		in.prev.caddyAdminVolume = in.prev.caddyAdminVolume || v.Name == n.CaddyAdminVolume
	}
	if o.Reinstall {
		if in.prev.caddyContainer, err = exists(inspectContainer(ctx, in.d, n.CaddyContainer)); err != nil {
			return fmt.Errorf("buscando el contenedor %s: %w", n.CaddyContainer, err)
		}
	}
	if err := in.checkNetwork(ctx); err != nil {
		return err
	}

	switch {
	case !o.Reinstall && (in.prev.masterKey || in.prev.dataVolume):
		return fmt.Errorf("%w (%s): una clave nueva no descifraría esa base. Si borraste muelle con docker stack rm %s, repite con --reinstall para reusar la clave y la base; "+
			"para empezar de cero (se pierden los datos), bórralos con docker secret rm %s y docker volume rm %s",
			ErrPreviousData, in.previousWhat(), n.Stack, n.MasterKeySecret, n.DataVolume)
	case o.Reinstall && !in.prev.masterKey:
		if in.prev.dataVolume {
			return fmt.Errorf("--reinstall necesita el secreto %s de la instalación anterior, y no existe: sin esa clave la base de %s no se puede abrir. "+
				"Para empezar de cero (se pierden los datos), borra el volumen con docker volume rm %s e instala sin --reinstall", n.MasterKeySecret, n.DataVolume, n.DataVolume)
		}
		return fmt.Errorf("--reinstall necesita el secreto %s de la instalación anterior, y no existe: no hay nada que reinstalar, instala sin --reinstall", n.MasterKeySecret)
	}
	if in.prev.dataVolume {
		in.warnf("la base ya existe (volumen %s): los ajustes de install no se aplican sobre una base que ya arrancó; cámbialos en el panel", n.DataVolume)
	}
	return nil
}

func (in *installer) previousWhat() string {
	var what []string
	if in.prev.masterKey {
		what = append(what, "el secreto "+in.names.MasterKeySecret)
	}
	if in.prev.dataVolume {
		what = append(what, "el volumen "+in.names.DataVolume)
	}
	return strings.Join(what, " y ")
}

// checkNetwork mira names.PanelNetwork (InspectNetwork funciona sin Swarm):
// si existe y no es overlay, attachable y de scope swarm, aborta acá, antes
// de bajar la imagen, probar los puertos o iniciar el Swarm. Sus subredes se
// revisan en el paso 8.
func (in *installer) checkNetwork(ctx context.Context) error {
	name := in.names.PanelNetwork
	net, err := in.d.InspectNetwork(ctx, name)
	if errors.Is(err, docker.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("buscando la red %s: %w", name, err)
	}
	if net.Driver != "overlay" || !net.Attachable || net.Scope != "swarm" {
		return fmt.Errorf("la red %s ya existe y no sirve: tiene que ser overlay, attachable y de scope swarm (es %s, attachable %v, scope %s). Bórrala con docker network rm %s y repite",
			name, cmp.Or(net.Driver, "?"), net.Attachable, cmp.Or(net.Scope, "?"), name)
	}
	in.prev.network = true
	return nil
}

func inspectService(ctx context.Context, d docker.Engine, name string) error {
	_, err := d.InspectService(ctx, name)
	return err
}

func inspectSecret(ctx context.Context, d docker.Engine, name string) error {
	_, err := d.InspectSecret(ctx, name)
	return err
}

func inspectContainer(ctx context.Context, d docker.Engine, name string) error {
	_, err := d.InspectContainer(ctx, name)
	return err
}

// --- Paso 4: la IP pública ------------------------------------------------------------

func (in *installer) detectIP(ctx context.Context) {
	if in.o.PublicIP != "" {
		in.publicIP = in.o.PublicIP
		in.logf("IP pública: %s", in.publicIP)
		return
	}
	ip, err := in.c.DetectIP(ctx)
	if err != nil {
		in.warnf("no pude detectar la IP pública (%v): muelle arranca sin ella; ponla después en Ajustes, o repite con --public-ip", err)
		return
	}
	in.publicIP = ip
	in.logf("IP pública: %s (detectada)", ip)
}

// --- Paso 5: las imágenes -------------------------------------------------------------

// ensureImages deja en el nodo la imagen de muelle y la de Caddy (así ingress
// la encuentra en el primer arranque), cada una con la credencial de su host.
func (in *installer) ensureImages(ctx context.Context) error {
	refs := []string{in.image}
	if in.caddyImage != "" && in.caddyImage != in.image {
		refs = append(refs, in.caddyImage)
	}
	for _, ref := range refs {
		ok, err := exists(inspectImage(ctx, in.d, ref))
		if err != nil {
			return fmt.Errorf("buscando la imagen %s: %w", ref, err)
		}
		if ok {
			in.logf("imagen %s: ya está en el nodo", ref)
			continue
		}
		cred := in.credFor(ref)
		if cred != nil {
			in.logf("bajando %s (con la credencial de %s para %s)…", ref, cred.Username, cred.Host)
		} else {
			in.logf("bajando %s…", ref)
		}
		if err := in.d.PullImage(ctx, ref, cred); err != nil {
			return pullError(ref, cred, err)
		}
	}
	return nil
}

func inspectImage(ctx context.Context, d docker.Engine, ref string) error {
	_, err := d.InspectImage(ctx, ref)
	return err
}

// pullError explica qué hacer si no se pudo bajar una imagen. Nunca cita la
// contraseña: el error de Docker no la trae, y acá solo va el usuario.
func pullError(ref string, cred *docker.RegistryAuth, err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	msg := strings.ToLower(err.Error())
	denied := errors.Is(err, docker.ErrDenied) || strings.Contains(msg, "denied") || strings.Contains(msg, "unauthorized")
	switch {
	case cred == nil:
		return fmt.Errorf("no pude bajar %s (%w): revisa que la versión exista y que el servidor llegue al registry; %s", ref, err, pullHint)
	case denied:
		return fmt.Errorf("%s rechazó la credencial de %s para %s (%w): revisa que el token pueda leer la imagen; %s", cred.Host, cred.Username, ref, err, pullHint)
	default:
		return fmt.Errorf("no pude bajar %s: %w", ref, err)
	}
}

// --- Paso 6: la prueba de puertos -------------------------------------------------------

// probeSpec es el contenedor temporal que publica los mismos puertos que
// muelle-caddy (contrato §5.4): la imagen de muelle con help, que termina enseguida.
func probeSpec(name, image string, httpPort, httpsPort int) docker.ContainerSpec {
	return docker.ContainerSpec{
		Name:  name,
		Image: image,
		Cmd:   []string{"help"},
		Ports: []docker.PortBinding{
			{HostPort: httpPort, ContainerPort: 80, Protocol: "tcp"},
			{HostPort: httpsPort, ContainerPort: 443, Protocol: "tcp"},
			{HostPort: httpsPort, ContainerPort: 443, Protocol: "udp"}, // HTTP/3
		},
	}
}

func (in *installer) probePorts(ctx context.Context) (err error) {
	n, o := in.names, in.o
	if o.Reinstall && in.prev.caddyContainer {
		in.logf("no pruebo los puertos: %s ya existe y los tiene tomados (ingress lo recrea si cambian)", n.CaddyContainer)
		return nil
	}
	// Se borra siempre: también el de una corrida cortada, y este si install se corta.
	remove := func() error {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), probeTimeout)
		defer cancel()
		if err := in.d.RemoveContainer(rctx, n.ProbeContainer, true); err != nil && !errors.Is(err, docker.ErrNotFound) {
			return err
		}
		return nil
	}
	if err := remove(); err != nil {
		return fmt.Errorf("borrando el contenedor de prueba que quedó (%s): %w", n.ProbeContainer, err)
	}
	defer func() {
		if rerr := remove(); rerr != nil {
			in.warnf("no pude borrar el contenedor de prueba (%v): bórralo con docker rm -f %s", rerr, n.ProbeContainer)
		}
	}()
	if _, err := in.d.CreateContainer(ctx, probeSpec(n.ProbeContainer, in.image, o.HTTPPort, o.HTTPSPort)); err != nil {
		return fmt.Errorf("creando el contenedor de prueba de puertos: %w", err)
	}
	err = in.d.StartContainer(ctx, n.ProbeContainer)
	if errors.Is(err, docker.ErrPortInUse) {
		return in.portsBusy(ctx, err)
	}
	if err != nil {
		return fmt.Errorf("arrancando el contenedor de prueba de puertos: %w", err)
	}
	in.logf("puertos %d y %d libres", o.HTTPPort, o.HTTPSPort)
	return nil
}

func (in *installer) portsBusy(ctx context.Context, dockerErr error) error {
	o, n := in.o, in.names
	msg := fmt.Sprintf("el %d/tcp, el %d/tcp o el %d/udp ya lo usa otro proceso o contenedor (%v): elige otros con --http-port y --https-port "+
		"(por ejemplo --http-port 8080 --https-port 8443, como en la migración desde Dokploy)", o.HTTPPort, o.HTTPSPort, o.HTTPSPort, dockerErr)
	if ok, _ := exists(inspectContainer(ctx, in.d, n.CaddyContainer)); ok {
		msg += fmt.Sprintf(". Ojo: existe el contenedor %s, de una instalación anterior (docker stack rm no lo borra), que tiene tomados los puertos del proxy: si no lo usas, bórralo con docker rm -f %s y repite",
			n.CaddyContainer, n.CaddyContainer)
	}
	return fmt.Errorf("%w: %s", ErrPortsBusy, msg)
}

// --- Paso 7: el Swarm ----------------------------------------------------------------

func (in *installer) initSwarm(ctx context.Context) error {
	if in.swarmActive() {
		in.nodeID = in.info.NodeID
		return nil
	}
	id, err := in.d.InitSwarm(ctx, in.o.AdvertiseAddr)
	if err != nil {
		if in.o.AdvertiseAddr == "" {
			return fmt.Errorf("iniciando el Swarm: %w. Si la máquina tiene varias IPs, repite con --advertise-addr <ip> (la IP privada por la que se hablarían los nodos)", err)
		}
		return fmt.Errorf("iniciando el Swarm con --advertise-addr %s: %w", in.o.AdvertiseAddr, err)
	}
	in.nodeID = id
	in.logf("Swarm iniciado (nodo %s)", id)
	return nil
}

// --- Paso 8: la red, los secretos y el stack -------------------------------------------

// create crea la red, los secretos y el stack. Ante cualquier error (o un
// Ctrl-C) desde la primera creación hasta que DeployStack funciona, deshace
// lo que creó esta corrida, en orden inverso y con un ctx propio: nunca queda
// un secreto con una clave que nadie vio (contrato §5.4 paso 8). Devuelve la
// clave maestra generada ("" con --reinstall).
func (in *installer) create(ctx context.Context) (key string, err error) {
	n, o := in.names, in.o
	var undo []func(context.Context)
	deployed := false
	defer func() {
		if err == nil || deployed || len(undo) == 0 {
			return
		}
		in.logf("deshaciendo lo que creó esta corrida…")
		uctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), undoTimeout)
		defer cancel()
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i](uctx)
		}
	}()

	if !in.prev.network {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if _, err := in.d.CreateNetwork(ctx, docker.NetworkSpec{Name: n.PanelNetwork, Driver: "overlay", Attachable: true}); err != nil {
			return "", fmt.Errorf("creando la red %s: %w", n.PanelNetwork, err)
		}
		undo = append(undo, in.undoNetwork)
		in.logf("red %s creada", n.PanelNetwork)
	} else {
		in.logf("red %s: ya existe, se usa", n.PanelNetwork)
	}
	if err := in.checkSubnets(ctx); err != nil {
		return "", err
	}

	if !o.Reinstall {
		k := crypto.GenerateKey()
		if err := in.createSecret(ctx, n.MasterKeySecret, false, []byte(k), &undo); err != nil {
			return "", err
		}
		key = k
	}
	if o.CloudflareToken != "" {
		if err := in.createSecret(ctx, n.InitCloudflareTokenSecret, in.prev.tokenSecret, []byte(o.CloudflareToken), &undo); err != nil {
			return "", err
		}
	}
	registry := in.credFor(in.image)
	if registry != nil {
		data, err := json.Marshal(initRegistry{Host: registry.Host, Username: registry.Username, Password: registry.Password})
		if err != nil {
			return "", err
		}
		if err := in.createSecret(ctx, n.InitRegistrySecret, in.prev.registrySecret, data, &undo); err != nil {
			return "", err
		}
	}

	compose, err := composeStack(stackInput{
		Names: n, Image: in.image, PublicIP: in.publicIP, NodeID: in.nodeID,
		Token: o.CloudflareToken != "", Registry: registry != nil, Opts: o,
	})
	if err != nil {
		return "", err
	}
	deploy := docker.StackDeploy{Name: n.Stack, Compose: compose}
	if registry != nil {
		deploy.Registries = []docker.RegistryAuth{*registry}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// Antes de DeployStack: el CLI puede crear el servicio y fallar después.
	undo = append(undo, in.undoStack)
	out, err := in.d.DeployStack(ctx, deploy)
	if err != nil {
		if out = strings.TrimSpace(out); out != "" {
			return "", fmt.Errorf("docker stack deploy %s: %w\n%s", n.Stack, err, out)
		}
		return "", fmt.Errorf("docker stack deploy %s: %w", n.Stack, err)
	}
	deployed = true
	in.logf("stack %s desplegado", n.Stack)
	return key, nil
}

// initRegistry es el JSON del secreto muelle_init_registry (contrato §5.2),
// que muelle lee de MUELLE_INIT_REGISTRY_FILE.
type initRegistry struct {
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// createSecret crea un secreto y agrega su borrado a undo. Si quedó uno de
// una corrida cortada (leftover), primero lo borra.
func (in *installer) createSecret(ctx context.Context, name string, leftover bool, data []byte, undo *[]func(context.Context)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if leftover {
		if err := in.d.RemoveSecret(ctx, name); err != nil && !errors.Is(err, docker.ErrNotFound) {
			return fmt.Errorf("borrando el secreto %s que quedó de una corrida anterior: %w", name, err)
		}
	}
	if _, err := in.d.CreateSecret(ctx, name, data, nil); err != nil {
		return fmt.Errorf("creando el secreto %s: %w", name, err)
	}
	*undo = append(*undo, func(ctx context.Context) { in.undoSecret(ctx, name) })
	in.logf("secreto %s creado", name)
	return nil
}

// checkSubnets espera las subredes de la red (una recién creada puede venir
// sin ellas unos milisegundos) y revisa que sean privadas (RFC 1918 o ULA):
// muelle confía en X-Real-IP solo desde una red privada.
func (in *installer) checkSubnets(ctx context.Context) error {
	name := in.names.PanelNetwork
	deadline := time.Now().Add(in.c.NetworkWait)
	for {
		net, err := in.d.InspectNetwork(ctx, name)
		if err != nil {
			return fmt.Errorf("revisando la red %s: %w", name, err)
		}
		if len(net.Subnets) > 0 {
			for _, s := range net.Subnets {
				p, err := netip.ParsePrefix(s)
				if err != nil || !p.Addr().IsPrivate() {
					return fmt.Errorf("la red %s tiene la subred %s, que no es privada (RFC 1918 o ULA): muelle confía en X-Real-IP solo desde una red privada. "+
						"Borra la red con docker network rm %s y revisa el default-addr-pool del Swarm", name, s, name)
				}
			}
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("la red %s no tiene subred todavía (Swarm no le asignó una en %s): revisa docker network inspect %s y repite", name, in.c.NetworkWait, name)
		}
		if err := sleep(ctx, min(in.c.Poll, 100*time.Millisecond)); err != nil {
			return err
		}
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// retry repite fn mientras devuelva ErrConflict (Docker todavía no suelta
// lo que se borra), hasta undoWait o el plazo de ctx.
func retry(ctx context.Context, fn func() error) error {
	deadline := time.Now().Add(undoWait)
	for {
		err := fn()
		if !errors.Is(err, docker.ErrConflict) || time.Now().After(deadline) {
			return err
		}
		if serr := sleep(ctx, retryEvery); serr != nil {
			return err
		}
	}
}

func (in *installer) leftover(what string, err error, cmd string) {
	in.warnf("no pude borrar %s (%v): bórralo a mano con %s", what, err, cmd)
}

func (in *installer) undoNetwork(ctx context.Context) {
	name := in.names.PanelNetwork
	err := retry(ctx, func() error { return in.d.RemoveNetwork(ctx, name) })
	if err != nil && !errors.Is(err, docker.ErrNotFound) {
		in.leftover("la red "+name, err, "docker network rm "+name)
	}
}

func (in *installer) undoSecret(ctx context.Context, name string) {
	if err := in.d.RemoveSecret(ctx, name); err != nil && !errors.Is(err, docker.ErrNotFound) {
		in.leftover("el secreto "+name, err, "docker secret rm "+name)
	}
}

// undoStack corre primero al deshacer: si el CLI alcanzó a crear el servicio,
// docker stack rm y espera a que ya no exista (los secretos no se pueden
// borrar mientras un servicio los usa). Después borra los volúmenes que esta
// corrida no encontró en el paso 3: si la tarea alcanzó a arrancar, el daemon
// los creó y muelle dejó una base cifrada con una clave que nadie vio.
func (in *installer) undoStack(ctx context.Context) {
	n := in.names
	svc, err := exists(inspectService(ctx, in.d, n.Service))
	if err != nil {
		in.leftover("el stack "+n.Stack, err, "docker stack rm "+n.Stack)
		return
	}
	if svc {
		if _, err := in.d.RemoveStack(ctx, n.Stack); err != nil {
			in.leftover("el stack "+n.Stack, err, "docker stack rm "+n.Stack)
			return
		}
		deadline := time.Now().Add(undoWait)
		for {
			still, err := exists(inspectService(ctx, in.d, n.Service))
			if err == nil && !still {
				break
			}
			if time.Now().After(deadline) || sleep(ctx, retryEvery) != nil {
				in.leftover("el servicio "+n.Service, cmp.Or(err, errors.New("sigue existiendo")), "docker stack rm "+n.Stack)
				return
			}
		}
	}
	for _, v := range []struct {
		name    string
		existed bool
	}{{n.DataVolume, in.prev.dataVolume}, {n.CaddyAdminVolume, in.prev.caddyAdminVolume}} {
		if v.existed {
			continue // de antes de esta corrida (con --reinstall, la base): nunca se borra
		}
		err := retry(ctx, func() error { return in.d.RemoveVolume(ctx, v.name, false) })
		if err != nil && !errors.Is(err, docker.ErrNotFound) {
			in.leftover("el volumen "+v.name, err, "docker volume rm "+v.name)
		}
	}
}

// --- Paso 9: la clave y el link -----------------------------------------------------

func (in *installer) finish(ctx context.Context, key string) error {
	out := in.c.Stdout
	if key != "" {
		fmt.Fprintf(out, "Clave maestra de muelle. Guárdala en un lugar seguro: no se vuelve a mostrar, y sin ella no se pueden descifrar los secretos ni los backups:\n\n    %s\n\n", key)
	}
	in.logf("esperando a que muelle arranque (hasta %s)…", in.c.SetupTimeout)
	res, err := waitSetup(ctx, in.d, in.names.Service, in.c.SetupTimeout, in.c.Poll, in.c.Stderr)
	if err != nil {
		return err
	}
	if res.HasUsers {
		// El origen que dice muelle sale de sus ajustes: con una base que ya
		// tenía usuarios (--reinstall), --https-port no se aplicó.
		fmt.Fprintf(out, "muelle ya tiene usuarios: entra al panel en %s\n", cmp.Or(res.Panel, panelOrigin(in.o)))
		return nil
	}
	fmt.Fprintf(out, "Abre este link para crear el primer usuario (es de un solo uso y vence en 24 h):\n\n    %s\n", res.URL)
	return nil
}
