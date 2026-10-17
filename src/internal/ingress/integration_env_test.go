//go:build integration

// El entorno de los tests de integración de ingress, contra el Docker real
// (un Swarm activo en el que este nodo es manager):
//
//	go test -tags integration -count=1 -run Integration -v ./src/internal/ingress/...
//
// Todo lo que crean se llama it3<id>… o it3-<id>-… (id: 4 hex al azar por
// test; it6 en los de la vuelta atrás de los puertos del proxy, itGroup) y
// se borra en t.Cleanup, también si el test falla. Nunca usan los nombres
// fijos de producción (muelle_panel, muelle-caddy, muelle_caddy_*,
// muelle_muelle…) ni los puertos 80, 443 o 2019: solo los del host entre
// 21000 y 21999 (it6: entre 23000 y 23999), por ranuras. Usan imágenes que ya estén en el nodo
// (caddy:2-alpine y alpine/socat:latest); si faltan, el test se salta en vez
// de bajarlas. Ninguno llega a la API real de Cloudflare: opts.CloudflareURL
// es siempre un cftest. No usan t.Parallel.
package ingress

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	reposqlite "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/repository/sqlite"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare/cftest"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

const (
	itCaddyImage  = "caddy:2-alpine"
	itSocatImage  = "alpine/socat:latest"
	itTestLabel   = "muelle.test"
	itRemoveRetry = 30 * time.Second
)

// itGroup es el prefijo de los nombres y los puertos del host de un grupo de
// tests: cada grupo corre sin chocar con los de otras ramas.
type itGroup struct {
	prefix      string
	first, last int
}

var (
	// itPhase3 son los tests de la Fase 3.
	itPhase3 = itGroup{prefix: "it3", first: 21000, last: 21999}
	// itPorts6 son los de la vuelta atrás de los puertos del proxy.
	itPorts6 = itGroup{prefix: "it6", first: 23000, last: 23999}
)

// slot son los puertos del host de una ranura n: first+100n+1 (http), +2
// (https), +3 (el puente de la admin API), +4/+5 (http/https de repuesto,
// para la recreación) y +6 (ocupado a propósito). Cada test usa su ranura.
type slot struct{ HTTP, HTTPS, Admin, HTTP2, HTTPS2, Busy int }

func slotPorts(g itGroup, n int) slot {
	b := g.first + 100*n
	return slot{HTTP: b + 1, HTTPS: b + 2, Admin: b + 3, HTTP2: b + 4, HTTPS2: b + 5, Busy: b + 6}
}

// itEnv es el entorno de un test de integración. Los nombres exactos:
//
//	el proyecto, y opts.NetworkPrefix   it3<id>
//	las redes de stage                  it3<id>_<stage>
//	la red panel (opts.PanelNetwork)    it3<id>_panel
//	muelle-caddy (opts.CaddyContainer)  it3-<id>-caddy
//	su prueba de los puertos            it3-<id>-caddy-probe
//	sus volúmenes                       it3-<id>-caddy-data, -config y -admin
//	el puente de la admin API           it3-<id>-admin
//	los backends y la sonda             it3-<id>-<app>
type itEnv struct {
	t       *testing.T
	docker  *docker.Client
	group   itGroup
	id      string // 4 hex
	project string // "it3"+id (el prefijo del grupo)
	ports   slot
	// opts son los nombres de arriba, el dial al puente, CaddyTLSAddr
	// 127.0.0.1:<https>, InternalCA, NetworkPrefix it3<id> y la URL de cf.
	opts   Options
	cf     *cftest.Server // el de opts.CloudflareURL: ningún test llega a la API real
	db     *sqlite.DB
	cipher *crypto.Cipher
	clock  clock.Clock

	mu         sync.Mutex
	containers []string // los que creó el test, para la limpieza
}

// newITEnv arma el entorno de la ranura slotN de los tests de la Fase 3
// (it3) y registra su limpieza (antes que cualquier run). Se salta sin un
// Swarm activo con este nodo de manager, o sin las imágenes en el nodo.
func newITEnv(t *testing.T, slotN int) *itEnv {
	t.Helper()
	return newITEnvIn(t, itPhase3, slotN)
}

// newITEnvIn es newITEnv con los nombres y los puertos del grupo g.
func newITEnvIn(t *testing.T, g itGroup, slotN int) *itEnv {
	t.Helper()
	c, err := docker.New(docker.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	ctx := context.Background()
	info, err := c.Info(ctx)
	if err != nil {
		t.Skipf("sin Docker: %v", err)
	}
	if info.SwarmState != docker.SwarmActive || !info.IsManager {
		t.Skipf("el nodo no es manager de un Swarm activo (%s)", info.SwarmState)
	}
	for _, img := range []string{itCaddyImage, itSocatImage} {
		if _, err := c.InspectImage(ctx, img); err != nil {
			t.Skipf("falta la imagen %s en el nodo (%v); bájala con docker pull", img, err)
		}
	}
	ports := slotPorts(g, slotN)
	if ports.HTTP < g.first || ports.Busy > g.last {
		t.Fatalf("la ranura %d se sale de los puertos %d-%d", slotN, g.first, g.last)
	}
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	id := hex.EncodeToString(b)
	key, err := crypto.New(bytes.Repeat([]byte{3}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e := &itEnv{
		t: t, docker: c, group: g, id: id, project: g.prefix + id, ports: ports,
		cf: cftest.New(t), db: sqlitetest.New(t), cipher: key, clock: clock.System{},
	}
	adminAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(ports.Admin))
	e.opts = Options{
		CaddyContainer:    e.name("caddy"),
		PanelNetwork:      e.project + "_panel",
		CaddyDataVolume:   e.name("caddy-data"),
		CaddyConfigVolume: e.name("caddy-config"),
		CaddyAdminVolume:  e.name("caddy-admin"),
		CaddyAdminDial: func(ctx context.Context) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", adminAddr)
		},
		CaddyTLSAddr:  net.JoinHostPort("127.0.0.1", strconv.Itoa(ports.HTTPS)),
		InternalCA:    true,
		NetworkPrefix: e.project,
		CloudflareURL: e.cf.URL(),
	}
	t.Cleanup(e.cleanup)
	t.Logf("integración: proyecto %s, puertos %+v", e.project, ports)
	return e
}

// name es it3-<id>-<suffix>.
func (e *itEnv) name(suffix string) string { return e.group.prefix + "-" + e.id + "-" + suffix }

// owned dice si un nombre es de este test.
func (e *itEnv) owned(name string) bool {
	return strings.HasPrefix(name, e.project+"_") || strings.HasPrefix(name, e.group.prefix+"-"+e.id+"-")
}

func (e *itEnv) logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(e.t.Output(), &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// module arma ingress con la base, el Docker real y opts.
func (e *itEnv) module() *Module {
	log := e.logger()
	return New(Deps{DB: e.db, Cipher: e.cipher, Clock: e.clock, Audit: sqlite.NewAuditRecorder(e.db, e.clock, log), Log: log,
		Docker: e.docker, Options: e.opts})
}

// cleanup borra todo lo del test: los contenedores, las redes con uno de los
// dos prefijos (reintentando mientras estén en uso) y los volúmenes.
func (e *itEnv) cleanup() {
	ctx := context.Background()
	e.mu.Lock()
	// muelle-caddy y su contenedor de prueba de los puertos (caddy.Runtime),
	// más los que creó el test.
	names := append([]string{e.opts.CaddyContainer, e.opts.CaddyContainer + "-probe"}, e.containers...)
	e.mu.Unlock()
	for _, n := range names {
		if err := e.docker.RemoveContainer(ctx, n, true); err != nil && !errors.Is(err, docker.ErrNotFound) {
			e.t.Errorf("limpieza: contenedor %s: %v", n, err)
		}
	}
	nets, err := e.docker.ListNetworks(ctx, nil)
	if err != nil {
		e.t.Errorf("limpieza: listando redes: %v", err)
	}
	for _, n := range nets {
		if !e.owned(n.Name) {
			continue
		}
		deadline := time.Now().Add(itRemoveRetry)
		for {
			err := e.docker.RemoveNetwork(ctx, n.Name)
			if err == nil || errors.Is(err, docker.ErrNotFound) {
				break
			}
			if !errors.Is(err, docker.ErrConflict) || time.Now().After(deadline) {
				e.t.Errorf("limpieza: red %s: %v", n.Name, err)
				break
			}
			time.Sleep(time.Second)
		}
	}
	vols, err := e.docker.ListVolumes(ctx, nil)
	if err != nil {
		e.t.Errorf("limpieza: listando volúmenes: %v", err)
	}
	for _, v := range vols {
		if !e.owned(v.Name) {
			continue
		}
		if err := e.docker.RemoveVolume(ctx, v.Name, true); err != nil && !errors.Is(err, docker.ErrNotFound) {
			e.t.Errorf("limpieza: volumen %s: %v", v.Name, err)
		}
	}
}

// createAndStart crea el contenedor (anotado para la limpieza antes de
// crearlo), lo conecta a las redes extra y lo arranca.
func (e *itEnv) createAndStart(spec docker.ContainerSpec, extraNetworks ...string) {
	e.t.Helper()
	if !e.owned(spec.Name) {
		e.t.Fatalf("el contenedor %s no tiene el prefijo del test", spec.Name)
	}
	e.mu.Lock()
	e.containers = append(e.containers, spec.Name)
	e.mu.Unlock()
	spec.Labels = map[string]string{itTestLabel: e.group.prefix}
	for _, n := range append([]string{spec.Network}, extraNetworks...) {
		if n != "" {
			e.waitNetwork(n)
		}
	}
	ctx := context.Background()
	if _, err := e.docker.CreateContainer(ctx, spec); err != nil {
		e.t.Fatalf("creando %s: %v", spec.Name, err)
	}
	for _, n := range extraNetworks {
		if err := e.retryAttach(func() error { return e.docker.ConnectNetwork(ctx, n, spec.Name, nil) }); err != nil {
			e.t.Fatalf("conectando %s a %s: %v", spec.Name, n, err)
		}
	}
	if err := e.retryAttach(func() error { return e.docker.StartContainer(ctx, spec.Name) }); err != nil {
		e.t.Fatalf("arrancando %s: %v", spec.Name, err)
	}
}

// itAttachRetry es cuánto se reintenta una conexión a una overlay que Swarm
// no completó a tiempo (cada intento puede tardar ~20 s en fallar).
const itAttachRetry = 90 * time.Second

// isSwarmAttachTimeout dice si err es la espera de Swarm al conectar un
// contenedor a una overlay attachable: el manager no asignó la conexión a
// tiempo y el daemon responde "attaching to network failed … context
// deadline exceeded". Pasa con el Swarm cargado, sobre todo cuando otros
// tests crean y borran overlays a la vez (en el log del daemon: "could not
// find network allocator state"). No es un error de muelle: en producción
// el job "proxy" conecta muelle-caddy a las redes que le falten.
func isSwarmAttachTimeout(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "attaching to network failed") && strings.Contains(msg, "context deadline exceeded")
}

// retryAttach corre f y la repite mientras falle con isSwarmAttachTimeout,
// hasta itAttachRetry. Devuelve el último error: cualquier otro, al primer
// intento. f tiene que poder repetirse (arrancar un contenedor creado,
// conectarlo o EnsureStageNetwork, que toma "ya conectado" como éxito).
func (e *itEnv) retryAttach(f func() error) error {
	e.t.Helper()
	deadline := time.Now().Add(itAttachRetry)
	for {
		err := f()
		if !isSwarmAttachTimeout(err) || time.Now().After(deadline) {
			return err
		}
		e.t.Logf("Swarm no completó la conexión a la red a tiempo; se reintenta: %v", err)
		time.Sleep(time.Second)
	}
}

// waitNetwork espera a que Swarm complete la red (que tenga subred): conectar
// un contenedor a una overlay recién creada puede quedar esperando la tarea
// de la conexión y fallar con "attaching to network failed … context
// deadline exceeded" (lo mismo que espera el test de redes del paquete docker).
func (e *itEnv) waitNetwork(name string) {
	e.t.Helper()
	e.eventually(30*time.Second, "Swarm completa la red "+name, func() (bool, error) {
		n, err := e.docker.InspectNetwork(context.Background(), name)
		return err == nil && len(n.Subnets) > 0, err
	})
}

// startAdminBridge arranca it3-<id>-admin: un socat que monta el volumen de
// la admin API y publica su socket en 127.0.0.1:<admin> (D4). Así muelle le
// manda a Caddy la misma configuración que en producción (admin.listen en el
// socket).
func (e *itEnv) startAdminBridge() {
	e.t.Helper()
	e.createAndStart(docker.ContainerSpec{
		Name:   e.name("admin"),
		Image:  itSocatImage,
		Cmd:    []string{"TCP-LISTEN:" + strconv.Itoa(e.ports.Admin) + ",fork,reuseaddr", "UNIX-CONNECT:" + naming.CaddyAdminSocket},
		Ports:  []docker.PortBinding{{HostIP: "127.0.0.1", HostPort: e.ports.Admin, ContainerPort: e.ports.Admin}},
		Mounts: []docker.Mount{{Volume: e.opts.CaddyAdminVolume, Target: naming.CaddyAdminDir}},
	})
}

// startBackend arranca it3-<id>-<app> en la red del stage, con el alias del
// servicio Swarm de la app (naming.Service), que responde "hola <app>" en :8080.
func (e *itEnv) startBackend(stage, app string) {
	e.t.Helper()
	e.startBackendCmd(stage, app, []string{"caddy", "respond", "--listen", ":8080", "--body", "hola " + app})
}

// startBackendCmd es startBackend con otro comando (la sonda de aislamiento de T3).
func (e *itEnv) startBackendCmd(stage, app string, cmd []string) {
	e.t.Helper()
	e.createAndStart(docker.ContainerSpec{
		Name:           e.name(app),
		Image:          itCaddyImage,
		Cmd:            cmd,
		Network:        naming.Network(e.project, stage),
		NetworkAliases: []string{naming.Service(e.project, stage, app, app)},
	})
}

// ensurePanelNetwork crea opts.PanelNetwork (overlay attachable) si falta.
func (e *itEnv) ensurePanelNetwork() {
	e.t.Helper()
	ctx := context.Background()
	_, err := e.docker.InspectNetwork(ctx, e.opts.PanelNetwork)
	if err == nil {
		return
	}
	if !errors.Is(err, docker.ErrNotFound) {
		e.t.Fatal(err)
	}
	if _, err := e.docker.CreateNetwork(ctx, docker.NetworkSpec{
		Name: e.opts.PanelNetwork, Driver: "overlay", Attachable: true, Labels: map[string]string{itTestLabel: e.group.prefix},
	}); err != nil && !errors.Is(err, docker.ErrConflict) {
		e.t.Fatal(err)
	}
}

// startCaddyManually (solo T2, antes de caddy.Runtime): crea la red panel si
// falta y un muelle-caddy de prueba como lo crea ingress (naming.CaddyCommand,
// naming.CaddyEnv y los tres volúmenes), con los puertos de la ranura en
// 127.0.0.1, en la red panel y en las de los stages.
func (e *itEnv) startCaddyManually(stages ...string) {
	e.t.Helper()
	e.ensurePanelNetwork()
	var nets []string
	for _, s := range stages {
		nets = append(nets, naming.Network(e.project, s))
	}
	e.createAndStart(docker.ContainerSpec{
		Name:  e.opts.CaddyContainer,
		Image: itCaddyImage,
		Cmd:   naming.CaddyCommand(),
		Env:   naming.CaddyEnv(),
		Ports: []docker.PortBinding{
			{HostIP: "127.0.0.1", HostPort: e.ports.HTTP, ContainerPort: 80},
			{HostIP: "127.0.0.1", HostPort: e.ports.HTTPS, ContainerPort: 443},
			{HostIP: "127.0.0.1", HostPort: e.ports.HTTPS, ContainerPort: 443, Protocol: "udp"},
		},
		Mounts: []docker.Mount{
			{Volume: e.opts.CaddyDataVolume, Target: naming.CaddyDataDir},
			{Volume: e.opts.CaddyConfigVolume, Target: naming.CaddyConfigDir},
			{Volume: e.opts.CaddyAdminVolume, Target: naming.CaddyAdminDir},
		},
		Network: e.opts.PanelNetwork,
	}, nets...)
}

// run corre m.Run en segundo plano. Su limpieza (cancelar y esperar a que Run
// vuelva, hasta 30 s) se registra DESPUÉS de la de newITEnv: t.Cleanup es
// LIFO, así Run ya terminó cuando se borran contenedores y redes, y no puede
// crear un it3-<id>-caddy después de la limpieza.
func (e *itEnv) run(m *Module) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Run(ctx)
	}()
	e.t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			e.t.Error("Run no volvió 30 s después de cancelarlo")
		}
	})
}

// settings guarda p como los ajustes, con panel.it3<id>.test, caddy_image
// caddy:2-alpine y los puertos de la ranura donde p no trae otros (80 y 443
// nunca: se cambian por los de la ranura).
func (e *itEnv) settings(p shared.ProxySettings) {
	e.t.Helper()
	if p.PanelDomain == "" {
		p.PanelDomain = e.panelHost()
	}
	if p.CaddyImage == "" {
		p.CaddyImage = itCaddyImage
	}
	if p.HTTPPort == 0 || p.HTTPPort == shared.DefaultProxyHTTPPort {
		p.HTTPPort = e.ports.HTTP
	}
	if p.HTTPSPort == 0 || p.HTTPSPort == shared.DefaultProxyHTTPSPort {
		p.HTTPSPort = e.ports.HTTPS
	}
	if p.PanelAllowedCIDRs == nil {
		p.PanelAllowedCIDRs = []string{}
	}
	ingresstest.SetProxySettings(e.t, e.db, e.cipher, p)
}

func (e *itEnv) panelHost() string { return "panel." + e.project + ".test" }

// host es <sub>.it3<id>.test.
func (e *itEnv) host(sub string) string { return sub + "." + e.project + ".test" }

// addDomain guarda un dominio de la app hacia service:port.
func (e *itEnv) addDomain(appID int64, hostname, service string, port int) entity.Domain {
	e.t.Helper()
	d, err := entity.NewDomain(appID, hostname, service, port, false, e.clock.Now())
	if err != nil {
		e.t.Fatal(err)
	}
	if err := reposqlite.NewDomains(e.db).Create(context.Background(), &d); err != nil {
		e.t.Fatal(err)
	}
	return d
}

// get hace un GET por HTTPS a 127.0.0.1:<https> con ServerName y Host = host.
// No verifica el certificado (la CA interna de Caddy): el TLS se prueba en T4.
func (e *itEnv) get(host, path string) (status int, body string, err error) {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(e.ports.HTTPS))
	c := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", addr)
			},
			TLSClientConfig:   &tls.Config{ServerName: host, InsecureSkipVerify: true},
			DisableKeepAlives: true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := c.Get("https://" + host + path)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, strings.TrimSpace(string(b)), err
}

// caddy inspecciona el muelle-caddy del test (opts.CaddyContainer).
func (e *itEnv) caddy() (docker.Container, error) {
	return e.docker.InspectContainer(context.Background(), e.opts.CaddyContainer)
}

// proxyStatus es el estado de GET /proxy en memoria, como un check de
// eventually que espera want.
func (e *itEnv) proxyStatus(m *Module, want string) func() (bool, error) {
	return func() (bool, error) {
		status, msg, _ := m.state.Snapshot(true)
		if string(status) != want {
			return false, fmt.Errorf("estado %s: %q", status, msg)
		}
		return true, nil
	}
}

// La espera transitoria de Swarm al conectar un contenedor a una overlay
// (arrancándolo o en caliente) se reintenta; cualquier otro error, no. No
// necesita Docker.
func TestIntegrationEnvSwarmAttachTimeout(t *testing.T) {
	const attach = "attaching to network failed, make sure your network options are correct and check manager logs: context deadline exceeded"
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{fmt.Errorf("docker: arrancando el contenedor it3-x-web: Error response from daemon: failed to set up container networking: %s", attach), true},
		{fmt.Errorf("docker: conectando it3-x-caddy a it3x_s2: Error response from daemon: %s", attach), true},
		{nil, false},
		{errors.New("docker: arrancando el contenedor it3-x-web: el puerto del host ya está en uso: address already in use"), false},
		{errors.New("docker: conectando it3-x-caddy a it3x_s2: no existe"), false},
		{context.DeadlineExceeded, false},
	} {
		if got := isSwarmAttachTimeout(tc.err); got != tc.want {
			t.Errorf("isSwarmAttachTimeout(%v) = %v, esperaba %v", tc.err, got, tc.want)
		}
	}
}

// eventually repite check cada 500 ms hasta que devuelve true o pasa timeout.
func (e *itEnv) eventually(timeout time.Duration, what string, check func() (bool, error)) {
	e.t.Helper()
	deadline := time.Now().Add(timeout)
	var last error
	for {
		ok, err := check()
		if ok {
			return
		}
		last = err
		if time.Now().After(deadline) {
			e.t.Fatalf("%s: no pasó en %s (último: %v)", what, timeout, last)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// expectGet es un check de eventually: GET host+path da status (y body, si no es "").
func (e *itEnv) expectGet(host, path string, status int, body string) func() (bool, error) {
	return func() (bool, error) {
		code, got, err := e.get(host, path)
		if err != nil {
			return false, err
		}
		if code != status || (body != "" && got != body) {
			return false, fmt.Errorf("GET %s%s: %d %q", host, path, code, got)
		}
		return true, nil
	}
}
