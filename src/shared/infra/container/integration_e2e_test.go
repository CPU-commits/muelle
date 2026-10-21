//go:build integration

// La prueba de punta a punta de las Fases 2, 3 y 5 juntas: muelle completo
// (container.New con los seis módulos, cron.Run y Run como en main, y la API
// por HTTP de verdad) contra el Swarm real de este nodo:
//
//	go test -tags integration -race -count=1 -timeout 25m -run Integration -v ./src/shared/infra/container/
//
// Todo lo que crea se llama it9<id>… o it9-<id>-… (id: 4 hex al azar): el
// proyecto (y con él sus stacks, servicios y redes), la red panel, muelle-caddy
// con sus volúmenes y su prueba de puertos, y el puente de la admin API. Se
// borra en t.Cleanup, también si la prueba falla; un -timeout que vence no
// corre los t.Cleanup, y lo barre el TestMain de la corrida siguiente
// (integration_main_test.go). Nunca usa los nombres fijos de producción
// (muelle_panel, muelle-caddy, muelle_caddy_*, muelle_muelle…) ni los puertos
// 80, 443 o 2019: solo 23001-23003 del host. muelle-caddy es la imagen
// oficial (caddy:2-alpine) con la CA interna (tls internal), y Cloudflare es
// un cftest: nada llega a la API real. Los digests salen de las imágenes del
// nodo (e2eDigests), así Swarm no baja nada. Usa imágenes que ya estén en el
// nodo (nginx:alpine, caddy:2-alpine y alpine/socat:latest); si falta alguna,
// la prueba se salta en vez de bajarla.
//
// Para limpiar a mano lo que haya quedado (solo toca lo que empieza con it9):
//
//	bash -c '
//	docker stack ls --format "{{.Name}}" | grep "^it9" | xargs -r -n1 docker stack rm
//	docker ps -a --format "{{.Names}}" | grep "^it9-" | xargs -r docker rm -f
//	for i in $(seq 60); do docker ps -a --format "{{.Label \"com.docker.stack.namespace\"}}" | grep -q "^it9" || break; sleep 2; done
//	docker network ls --format "{{.Name}}" | grep "^it9" | xargs -r docker network rm
//	docker volume ls --format "{{.Name}}" | grep "^it9" | xargs -r docker volume rm'
package container

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/distribution/reference"

	"github.com/CPU-commits/muelle/src/cmd/cron"
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/ingress"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare/cftest"
	"github.com/CPU-commits/muelle/src/settings"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

const (
	e2ePrefix     = "it9"
	e2eLabel      = "muelle.test"
	e2eNginx      = "nginx:alpine"
	e2eCaddyImage = "caddy:2-alpine"
	e2eSocatImage = "alpine/socat:latest"
	e2eHTTPPort   = 23001
	e2eHTTPSPort  = 23002
	e2eAdminPort  = 23003 // el puente de la admin API de muelle-caddy
	e2eTimeout    = 3 * time.Minute
	e2ePublicIP   = "203.0.113.9"
	e2eStage      = "staging"
	e2eApp        = "web"
)

// e2eDigests es el cliente real, salvo que DistributionDigest no sale a
// internet: devuelve el digest de RepoDigests de la imagen del nodo. Como
// docker stack deploy va con --resolve-image=never y la ref resuelta trae ese
// digest, Swarm no baja nada. Una ref con digest (el rollback) también pasa
// por acá: se busca sin el tag (<repo>@sha256:…) y devuelve ese mismo digest.
// Es lo mismo que localDigests en los tests de integración de deploy.
type e2eDigests struct{ *docker.Client }

func (l e2eDigests) DistributionDigest(ctx context.Context, ref string, _ *docker.RegistryAuth) (string, error) {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return "", fmt.Errorf("e2eDigests: %s: %w", ref, docker.ErrNotFound)
	}
	lookup := ref
	if d, ok := named.(reference.Digested); ok {
		lookup = reference.FamiliarName(named) + "@" + d.Digest().String()
	}
	img, err := l.InspectImage(ctx, lookup)
	if err != nil {
		return "", err
	}
	for _, rd := range img.RepoDigests {
		rn, err := reference.ParseNormalizedNamed(rd)
		if err != nil || rn.Name() != named.Name() {
			continue
		}
		if d, ok := rn.(reference.Digested); ok {
			if pinned, ok := named.(reference.Digested); ok && pinned.Digest() != d.Digest() {
				continue
			}
			return d.Digest().String(), nil
		}
	}
	return "", fmt.Errorf("e2eDigests: %s no tiene RepoDigests de %s: %w", ref, named.Name(), docker.ErrNotFound)
}

// e2eEnv es un muelle completo de la prueba. Los nombres exactos:
//
//	el proyecto, y opts.NetworkPrefix   it9<id>
//	la red del stage                    it9<id>_staging
//	la red panel (opts.PanelNetwork)    it9<id>_panel
//	muelle-caddy (opts.CaddyContainer)  it9-<id>-caddy (y it9-<id>-caddy-probe)
//	sus volúmenes                       it9-<id>-caddy-data, -config y -admin
//	el puente de la admin API           it9-<id>-admin
type e2eEnv struct {
	t       *testing.T
	docker  *docker.Client
	id      string
	project string
	opts    ingress.Options
	cf      *cftest.Server
	c       *Container
	srv     *httptest.Server
	origin  string // MUELLE_PANEL_ORIGIN: https://panel.it9<id>.test:23002
	session string // la cookie del admin
	// roots es opts.TLSRoots: empieza vacío, y la prueba le suma la raíz de
	// la CA interna de Caddy (trustCaddyCA) antes de crear el primer dominio,
	// así la verificación de certificados de ingress lee el de Caddy.
	roots *x509.CertPool
}

// newE2EEnv arma el entorno y registra su limpieza antes que nada (t.Cleanup
// es LIFO: el muelle se detiene y se cierra antes de borrar lo de Docker). Se
// salta sin un Swarm activo con este nodo de manager, o sin las imágenes.
func newE2EEnv(t *testing.T) *e2eEnv {
	t.Helper()
	cli, err := docker.New(docker.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cli.Close() })
	ctx := context.Background()
	info, err := cli.Info(ctx)
	if err != nil {
		t.Skipf("sin Docker: %v", err)
	}
	if info.SwarmState != docker.SwarmActive || !info.IsManager {
		t.Skipf("el nodo no es manager de un Swarm activo (%s)", info.SwarmState)
	}
	for _, img := range []string{e2eNginx, e2eCaddyImage, e2eSocatImage} {
		if _, err := cli.InspectImage(ctx, img); err != nil {
			t.Skipf("falta la imagen %s en el nodo (%v); bájala con docker pull", img, err)
		}
	}
	if img, _ := cli.InspectImage(ctx, e2eNginx); len(img.RepoDigests) == 0 {
		t.Skipf("la imagen %s no tiene RepoDigests (¿docker load?); bájala con docker pull", e2eNginx)
	}
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	id := hex.EncodeToString(b)
	e := &e2eEnv{t: t, docker: cli, id: id, project: e2ePrefix + id, cf: cftest.New(t), roots: x509.NewCertPool()}
	e.origin = "https://panel." + e.project + ".test:" + strconv.Itoa(e2eHTTPSPort)
	adminAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(e2eAdminPort))
	e.opts = ingress.Options{
		CaddyContainer:    e.name("caddy"),
		PanelNetwork:      e.project + "_panel",
		CaddyDataVolume:   e.name("caddy-data"),
		CaddyConfigVolume: e.name("caddy-config"),
		CaddyAdminVolume:  e.name("caddy-admin"),
		CaddyAdminDial: func(ctx context.Context) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", adminAddr)
		},
		CaddyTLSAddr:  net.JoinHostPort("127.0.0.1", strconv.Itoa(e2eHTTPSPort)),
		TLSRoots:      e.roots,
		InternalCA:    true,
		NetworkPrefix: e.project,
		CloudflareURL: e.cf.URL(),
		// La primera verificación de un dominio, al segundo (no a los 30 s).
		Timing: ingress.Timing{FirstCheck: time.Second},
	}
	t.Cleanup(e.cleanup)
	t.Logf("e2e: proyecto %s, puertos %d/%d (admin %d)", e.project, e2eHTTPPort, e2eHTTPSPort, e2eAdminPort)

	e.startAdminBridge()

	dir := t.TempDir()
	s := settings.Settings{
		DataDir:       filepath.Join(dir, "data"),
		MasterKeyFile: writeKey(t, dir, "key"),
		PanelOrigin:   e.origin,
		SecureCookies: true,
		Init: settings.Init{
			CaddyImage:     e2eCaddyImage,
			ProxyHTTPPort:  e2eHTTPPort,
			ProxyHTTPSPort: e2eHTTPSPort,
		},
	}
	log := slog.New(slog.NewTextHandler(t.Output(), &slog.HandlerOptions{Level: slog.LevelDebug}))
	c, err := New(ctx, s, log, WithDocker(e2eDigests{cli}), WithIngress(e.opts))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("cerrando el contenedor: %v", err)
		}
	})
	e.c = c
	e.srv = httptest.NewServer(c.Server)
	t.Cleanup(e.srv.Close)

	// Como main: cron.Run (el job "proxy" corre al arrancar y crea
	// muelle-caddy) y c.Run, hasta que la prueba termina.
	bgCtx, stop := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() { cron.Run(bgCtx, c.Jobs, log) })
	wg.Go(func() { c.Run(bgCtx) })
	t.Cleanup(func() {
		stop()
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(60 * time.Second):
			t.Error("cron.Run y Run no volvieron 60 s después de cancelarlos")
		}
	})

	token, created, err := c.Identity.EnsureSetupInvite(ctx)
	if err != nil || !created {
		t.Fatalf("EnsureSetupInvite: created=%v err=%v", created, err)
	}
	resp, body := e.send("POST", "/api/v1/auth/setup", map[string]string{"token": token, "username": "admin", "password": testPassword}, "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /auth/setup: %d %s", resp.StatusCode, body)
	}
	for _, ck := range resp.Cookies() {
		if ck.Name == utils.SessionCookie {
			e.session = ck.Value
		}
	}
	if e.session == "" {
		t.Fatalf("POST /auth/setup no dejó la cookie %s", utils.SessionCookie)
	}
	return e
}

// name es it9-<id>-<suffix>.
func (e *e2eEnv) name(suffix string) string { return e2ePrefix + "-" + e.id + "-" + suffix }

// owned dice si un nombre de Docker es de esta prueba.
func (e *e2eEnv) owned(name string) bool {
	return strings.HasPrefix(name, e.project+"_") || strings.HasPrefix(name, e2ePrefix+"-"+e.id+"-")
}

// startAdminBridge arranca it9-<id>-admin: un socat que monta el volumen de
// la admin API de muelle-caddy y publica su socket en 127.0.0.1:23003. Así
// muelle le habla a Caddy como en producción (admin.listen en el socket).
func (e *e2eEnv) startAdminBridge() {
	e.t.Helper()
	ctx := context.Background()
	name := e.name("admin")
	if _, err := e.docker.CreateContainer(ctx, docker.ContainerSpec{
		Name:   name,
		Image:  e2eSocatImage,
		Cmd:    []string{"TCP-LISTEN:" + strconv.Itoa(e2eAdminPort) + ",fork,reuseaddr", "UNIX-CONNECT:" + naming.CaddyAdminSocket},
		Ports:  []docker.PortBinding{{HostIP: "127.0.0.1", HostPort: e2eAdminPort, ContainerPort: e2eAdminPort}},
		Mounts: []docker.Mount{{Volume: e.opts.CaddyAdminVolume, Target: naming.CaddyAdminDir}},
		Labels: map[string]string{e2eLabel: e2ePrefix},
	}); err != nil {
		e.t.Fatalf("creando %s: %v", name, err)
	}
	if err := e.docker.StartContainer(ctx, name); err != nil {
		e.t.Fatalf("arrancando %s: %v", name, err)
	}
}

// cleanup borra todo lo de la prueba: los stacks del proyecto (esperando a
// que Swarm baje sus contenedores), los contenedores it9-<id>-…, las redes
// it9<id>_… (reintentando mientras estén en uso) y los volúmenes.
func (e *e2eEnv) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	byProject := map[string]string{naming.LabelProject: e.project}
	if svcs, err := e.docker.ListServices(ctx, byProject); err == nil {
		stacks := map[string]bool{}
		for _, s := range svcs {
			if st := s.Labels[naming.LabelStackNamespace]; strings.HasPrefix(st, e.project+"_") {
				stacks[st] = true
			}
		}
		for st := range stacks {
			if out, err := e.docker.RemoveStack(ctx, st); err != nil {
				e.t.Errorf("limpieza: docker stack rm %s: %v\n%s", st, err, out)
			}
		}
	} else {
		e.t.Errorf("limpieza: listando servicios: %v", err)
	}
	if !e2eRetry(ctx, 90*time.Second, func() bool {
		svcs, err := e.docker.ListServices(ctx, byProject)
		return err == nil && len(svcs) == 0 && len(e.stackContainers(ctx, e.project+"_")) == 0
	}) {
		e.t.Errorf("limpieza: quedaron servicios o contenedores del proyecto %s", e.project)
	}
	for _, n := range []string{e.opts.CaddyContainer, e.opts.CaddyContainer + "-probe", e.name("admin")} {
		if err := e.docker.RemoveContainer(ctx, n, true); err != nil && !errors.Is(err, docker.ErrNotFound) {
			e.t.Errorf("limpieza: contenedor %s: %v", n, err)
		}
	}
	if nets, err := e.docker.ListNetworks(ctx, nil); err == nil {
		for _, n := range nets {
			if !e.owned(n.Name) {
				continue
			}
			if !e2eRetry(ctx, 60*time.Second, func() bool {
				err := e.docker.RemoveNetwork(ctx, n.Name)
				return err == nil || errors.Is(err, docker.ErrNotFound)
			}) {
				e.t.Errorf("limpieza: no se pudo borrar la red %s", n.Name)
			}
		}
	} else {
		e.t.Errorf("limpieza: listando redes: %v", err)
	}
	if vols, err := e.docker.ListVolumes(ctx, nil); err == nil {
		for _, v := range vols {
			if !e.owned(v.Name) {
				continue
			}
			if err := e.docker.RemoveVolume(ctx, v.Name, true); err != nil && !errors.Is(err, docker.ErrNotFound) {
				e.t.Errorf("limpieza: volumen %s: %v", v.Name, err)
			}
		}
	} else {
		e.t.Errorf("limpieza: listando volúmenes: %v", err)
	}
}

// stackContainers son los ids de los contenedores (corriendo o no) cuyo
// stack empieza con prefix (docker.Engine no lista contenedores: va el CLI).
func (e *e2eEnv) stackContainers(ctx context.Context, prefix string) []string {
	out, err := exec.CommandContext(ctx, "docker", "ps", "-a", "--format", `{{.ID}} {{.Label "`+naming.LabelStackNamespace+`"}}`).Output()
	if err != nil {
		return []string{"(docker ps falló: " + err.Error() + ")"}
	}
	var ids []string
	for line := range strings.Lines(string(out)) {
		id, stack, _ := strings.Cut(strings.TrimSpace(line), " ")
		if strings.HasPrefix(stack, prefix) {
			ids = append(ids, id)
		}
	}
	return ids
}

// e2eRetry repite ok cada segundo hasta que da true o pasa d.
func e2eRetry(ctx context.Context, d time.Duration, ok func() bool) bool {
	deadline := time.Now().Add(d)
	for {
		if ok() {
			return true
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return false
		}
		time.Sleep(time.Second)
	}
}

// eventually repite check cada 500 ms hasta que devuelve true o pasa timeout.
func (e *e2eEnv) eventually(timeout time.Duration, what string, check func() (bool, error)) {
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

// --- La API por HTTP ---------------------------------------------------------

// send hace la petición al servidor de verdad (httptest.NewServer de
// c.Server): con bearer, un token de API sin Origin; sin bearer, la cookie de
// la sesión (si hay) y Origin = MUELLE_PANEL_ORIGIN, como el panel. Devuelve
// la respuesta, ya leída, y su cuerpo.
func (e *e2eEnv) send(method, path string, body any, bearer string) (*http.Response, []byte) {
	e.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := jsonv2.Marshal(body, jsonv2.Deterministic(true))
		if err != nil {
			e.t.Fatalf("serializando el body de %s %s: %v", method, path, err)
		}
		reader = bytes.NewReader(raw)
	}
	req := e.request(method, path, reader, bearer)
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatalf("%s %s: leyendo la respuesta: %v", method, path, err)
	}
	return resp, b
}

func (e *e2eEnv) request(method, path string, body io.Reader, bearer string) *http.Request {
	e.t.Helper()
	req, err := http.NewRequest(method, e.srv.URL+path, body)
	if err != nil {
		e.t.Fatal(err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
		return req
	}
	req.Header.Set("Origin", e.origin)
	if e.session != "" {
		req.AddCookie(&http.Cookie{Name: utils.SessionCookie, Value: e.session})
	}
	return req
}

// call hace la petición, exige want y devuelve el cuerpo.
func (e *e2eEnv) call(method, path string, body any, bearer string, want int) []byte {
	e.t.Helper()
	resp, b := e.send(method, path, body, bearer)
	if resp.StatusCode != want {
		e.t.Fatalf("%s %s: %d, esperaba %d: %s", method, path, resp.StatusCode, want, b)
	}
	return b
}

// object es call con una respuesta JSON objeto.
func (e *e2eEnv) object(method, path string, body any, bearer string, want int) map[string]any {
	e.t.Helper()
	return e2eDecode[map[string]any](e.t, e.call(method, path, body, bearer, want))
}

func e2eDecode[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if err := jsonv2.Unmarshal(b, &v); err != nil {
		t.Fatalf("JSON inválido: %v: %s", err, b)
	}
	return v
}

// e2eEvent es un evento SSE.
type e2eEvent struct{ Name, ID, Data string }

// events abre el stream y lee sus eventos hasta que el servidor lo cierra
// (el status final de un deploy, el end de unos logs sin follow), con un
// plazo de timeout. Los comentarios (": ping") no cuentan.
func (e *e2eEnv) events(path, bearer string, timeout time.Duration) []e2eEvent {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req := e.request("GET", path, nil, bearer).WithContext(ctx)
	resp, err := e.srv.Client().Do(req)
	if err != nil {
		e.t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		b, _ := io.ReadAll(resp.Body)
		e.t.Fatalf("GET %s: %d %s: %s", path, resp.StatusCode, resp.Header.Get("Content-Type"), b)
	}
	var out []e2eEvent
	var ev e2eEvent
	has := false
	br := bufio.NewReader(resp.Body)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if ctx.Err() != nil {
				e.t.Fatalf("GET %s: el stream no cerró en %s (eventos: %+v)", path, timeout, out)
			}
			if has {
				out = append(out, ev)
			}
			return out
		}
		line = strings.TrimSuffix(line, "\n")
		switch {
		case line == "":
			if has {
				out = append(out, ev)
			}
			ev, has = e2eEvent{}, false
		case strings.HasPrefix(line, ":"):
		default:
			k, v, _ := strings.Cut(line, ":")
			v = strings.TrimPrefix(v, " ")
			switch k {
			case "event":
				ev.Name = v
			case "id":
				ev.ID = v
			case "data":
				ev.Data = v
			}
			has = true
		}
	}
}

// deployLog lee el log del deploy por polling (contrato §2.5) hasta que
// termina, y devuelve su estado final y el log entero.
func (e *e2eEnv) deployLog(app string, id int64, bearer string) (string, string) {
	e.t.Helper()
	var log strings.Builder
	offset := 0
	deadline := time.Now().Add(e2eTimeout)
	for {
		page := e2eDecode[struct {
			Chunk      string `json:"chunk"`
			NextOffset int    `json:"next_offset"`
			Status     string `json:"status"`
		}](e.t, e.call("GET", fmt.Sprintf("%s/deploys/%d/log?offset=%d", app, id, offset), nil, bearer, http.StatusOK))
		log.WriteString(page.Chunk)
		offset = page.NextOffset
		switch page.Status {
		case "succeeded", "failed", "superseded":
			if page.Chunk == "" {
				return page.Status, log.String()
			}
			continue
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("el deploy %d no terminó en %s (%s):\n%s", id, e2eTimeout, page.Status, log.String())
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// --- Caddy -------------------------------------------------------------------

// viaCaddy hace un GET por HTTPS a muelle-caddy (127.0.0.1:23002) con
// ServerName y Host = host. No verifica el certificado: es de la CA interna.
func (e *e2eEnv) viaCaddy(host string) (int, string, error) {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(e2eHTTPSPort))
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
	resp, err := c.Get("https://" + host + "/")
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, string(b), err
}

// caddyAdmin hace un GET a la admin API de Caddy por el puente
// (127.0.0.1:23003) y devuelve el cuerpo; un status que no es 200 es un error.
func (e *e2eEnv) caddyAdmin(path string) ([]byte, error) {
	c := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext:       func(ctx context.Context, _, _ string) (net.Conn, error) { return e.opts.CaddyAdminDial(ctx) },
			DisableKeepAlives: true,
		},
	}
	resp, err := c.Get("http://caddy" + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %d %s", path, resp.StatusCode, b)
	}
	return b, nil
}

// caddyRoutes es la lista de rutas que tiene Caddy ahora, en JSON.
func (e *e2eEnv) caddyRoutes() (string, error) {
	b, err := e.caddyAdmin("/config/apps/http/servers/muelle/routes")
	return string(b), err
}

// trustCaddyCA suma a e.roots (opts.TLSRoots) la raíz de la CA interna de
// Caddy (GET /pki/ca/local). Va antes de crear el primer dominio: después,
// la verificación de certificados de ingress lee el pool.
func (e *e2eEnv) trustCaddyCA() {
	e.t.Helper()
	var ca struct {
		Root string `json:"root_certificate"`
	}
	e.eventually(30*time.Second, "GET /pki/ca/local", func() (bool, error) {
		b, err := e.caddyAdmin("/pki/ca/local")
		if err != nil {
			return false, err
		}
		return true, jsonv2.Unmarshal(b, &ca, jsonv2.RejectUnknownMembers(false))
	})
	if !e.roots.AppendCertsFromPEM([]byte(ca.Root)) {
		e.t.Fatalf("la raíz de la CA interna no es PEM: %q", ca.Root)
	}
}

// --- La prueba ---------------------------------------------------------------

// El flujo completo de una app por la API, con muelle entero (container.New)
// contra el Swarm real, muelle-caddy de verdad y un Cloudflare falso:
//
//  1. proyecto, stage (su red y la conexión de muelle-caddy), una app de
//     formulario con nginx y una variable secreta;
//  2. el deploy por la API con un token de CI, seguido por SSE hasta
//     succeeded, con el secreto enmascarado en el log;
//  3. GET …/status;
//  4. los logs de la app por SSE, con tail y con since (enmascarados);
//  5. un dominio con Cloudflare (falso): muelle-caddy enruta ese Host a la app;
//  6. un redeploy y el rollback;
//  7. borrar el dominio, la app y el stage: no quedan servicios, contenedores,
//     red ni rutas de Caddy.
func TestIntegrationEndToEnd(t *testing.T) {
	e := newE2EEnv(t)
	ctx := context.Background()
	const zoneID = "it9zone"
	cfToken := "cf-token-" + e.id + "-1234567890abcdef"
	e.cf.AddToken(cfToken)
	e.cf.AddZone(zoneID, e.project+".test")
	// El secreto: un valor de la app que, además, es el nombre del token de
	// CI. Así aparece en la primera línea del log de cada deploy (el
	// actor_name) y tiene que salir enmascarado; la app lo imprime al
	// arrancar, y sus logs también lo enmascaran.
	secret := "it9-secreto-" + e.id + "-Zq7Lw"

	// muelle-caddy lo crea el job "proxy" al arrancar (cron.Run), con los
	// ajustes de MUELLE_INIT_* (la imagen y los puertos).
	e.eventually(90*time.Second, "GET /proxy synced", func() (bool, error) {
		got := e.object("GET", "/api/v1/proxy", nil, "", http.StatusOK)
		if got["status"] != "synced" {
			return false, fmt.Errorf("%v", got)
		}
		return true, nil
	})
	proxy := e.object("GET", "/api/v1/proxy", nil, "", http.StatusOK)
	if c, _ := proxy["caddy"].(map[string]any); c["image"] != e2eCaddyImage || c["running"] != true ||
		c["http_port"] != float64(e2eHTTPPort) || c["https_port"] != float64(e2eHTTPSPort) || proxy["synced_at"] == nil {
		t.Fatalf("GET /proxy: %v", proxy)
	}
	e.trustCaddyCA()

	// 1. Proyecto, stage y app.
	P := "/api/v1/projects/" + e.project
	S := P + "/stages/" + e2eStage
	A := S + "/apps/" + e2eApp
	e.object("POST", "/api/v1/projects", map[string]string{"slug": e.project, "name": "it9 " + e.id}, "", http.StatusCreated)
	e.object("POST", P+"/stages", map[string]string{"slug": e2eStage}, "", http.StatusCreated)
	network := naming.Network(e.project, e2eStage)
	n, err := e.docker.InspectNetwork(ctx, network)
	if err != nil || n.Driver != "overlay" || !n.Attachable || n.Labels[naming.LabelProject] != e.project {
		t.Fatalf("la red del stage: %+v %v", n, err)
	}
	if c, err := e.docker.InspectContainer(ctx, e.opts.CaddyContainer); err != nil || !slices.Contains(c.Networks, network) {
		t.Fatalf("muelle-caddy no quedó en la red del stage: %+v %v", c, err)
	}
	e.object("POST", S+"/apps", map[string]any{"slug": e2eApp, "kind": "service", "spec": map[string]any{
		"image": e2eNginx, "port": 80, "replicas": 1,
		"command":     []string{"sh", "-c", `echo "it9 secreto: $IT9_SECRET"; exec nginx -g 'daemon off;'`},
		"healthcheck": map[string]any{"type": "http", "path": "/", "interval_s": 1, "timeout_s": 2, "retries": 3},
	}}, "", http.StatusCreated)
	e.call("PUT", A+"/env", map[string]any{"vars": map[string]string{"IT9_SECRET": secret}, "secret": true}, "", http.StatusNoContent)
	tok, _ := e.object("POST", "/api/v1/tokens", map[string]any{"name": secret, "project": e.project, "stage": ""}, "", http.StatusCreated)["token"].(string)
	if !strings.HasPrefix(tok, "mk_") {
		t.Fatalf("token: %q", tok)
	}

	// 2. El deploy, seguido por SSE.
	queued := e.object("POST", A+"/deploys", nil, tok, http.StatusAccepted)
	id1 := int64(queued["id"].(float64))
	if queued["status"] != "queued" || queued["status_url"] != fmt.Sprintf("%s/deploys/%d", A, id1) {
		t.Fatalf("POST …/deploys: %v", queued)
	}
	evs := e.events(fmt.Sprintf("%s/deploys/%d/log/stream", A, id1), tok, e2eTimeout)
	var lines []string
	for _, ev := range evs {
		if ev.Name == "log" {
			lines = append(lines, ev.Data)
		}
	}
	streamed := strings.Join(lines, "\n")
	if last := evs[len(evs)-1]; last.Name != "status" || last.Data != "succeeded" {
		t.Fatalf("el deploy %d terminó con %+v:\n%s", id1, last, streamed)
	}
	if strings.Contains(streamed, secret) || !strings.Contains(lines[0], "(api · ***)") {
		t.Fatalf("el secreto no salió enmascarado en el log:\n%s", streamed)
	}
	if !strings.Contains(streamed, "rutas de Caddy actualizadas") || strings.Contains(streamed, "advertencia") {
		t.Fatalf("el paso 7 no llegó a Caddy:\n%s", streamed)
	}
	if _, polled := e.deployLog(A, id1, tok); polled != streamed+"\n" {
		t.Fatalf("el stream y el polling no traen lo mismo:\n%s\n---\n%s", streamed, polled)
	}
	d1 := e.object("GET", fmt.Sprintf("%s/deploys/%d", A, id1), nil, tok, http.StatusOK)
	images1, _ := d1["images"].(map[string]any)
	if ref, _ := images1[e2eApp].(string); d1["trigger"] != "api" || d1["actor_type"] != "token" || !strings.HasPrefix(ref, e2eNginx+"@sha256:") {
		t.Fatalf("el deploy %d: %v", id1, d1)
	}
	// La ref pedida, sin digest, y routes_warning false: el paso 7 llegó al
	// Caddy de verdad (contrato §2.1).
	if requested1, _ := d1["requested_images"].(map[string]any); requested1[e2eApp] != e2eNginx || d1["routes_warning"] != false {
		t.Fatalf("el deploy %d: %v", id1, d1)
	}
	// catalog ve el deploy (deploy escribe last_deployed_at).
	if app := e.object("GET", A, nil, tok, http.StatusOK); app["last_deployed_at"] == nil || app["pending_changes"] != false {
		t.Fatalf("GET %s después del deploy: %v", A, app)
	}

	// 3. El estado en Swarm.
	var svc map[string]any
	e.eventually(60*time.Second, "GET …/status healthy", func() (bool, error) {
		got := e2eDecode[struct {
			Services []map[string]any `json:"services"`
		}](t, e.call("GET", A+"/status", nil, tok, http.StatusOK))
		if len(got.Services) != 1 {
			return false, fmt.Errorf("servicios: %v", got.Services)
		}
		svc = got.Services[0]
		tasks, _ := svc["tasks"].([]any)
		if len(tasks) == 0 {
			return false, fmt.Errorf("sin tareas: %v", svc)
		}
		task, _ := tasks[0].(map[string]any)
		if task["state"] != "running" || task["health"] != "healthy" {
			return false, fmt.Errorf("tarea: %v", task)
		}
		return true, nil
	})
	if svc["name"] != e2eApp || svc["image"] != images1[e2eApp] || svc["replicas_desired"] != 1.0 || svc["replicas_running"] != 1.0 {
		t.Fatalf("GET …/status: %v", svc)
	}

	// 4. Los logs de la app, con tail y con since.
	type logLine struct {
		Time   string `json:"time"`
		Task   string `json:"task"`
		Stream string `json:"stream"`
		Line   string `json:"line"`
	}
	appLogs := func(query string) []logLine {
		t.Helper()
		evs := e.events(A+"/logs?"+query, tok, 60*time.Second)
		if len(evs) == 0 || evs[len(evs)-1] != (e2eEvent{Name: "end", Data: "eof"}) {
			t.Fatalf("GET …/logs?%s sin end eof al final: %+v", query, evs)
		}
		var out []logLine
		for _, ev := range evs[:len(evs)-1] {
			if ev.Name != "log" {
				t.Fatalf("GET …/logs?%s: evento %+v", query, ev)
			}
			if strings.Contains(ev.Data, secret) {
				t.Fatalf("GET …/logs?%s trae el secreto: %s", query, ev.Data)
			}
			out = append(out, e2eDecode[logLine](t, []byte(ev.Data)))
		}
		return out
	}
	masked := func(l logLine) bool { return l.Line == "it9 secreto: ***" }
	var first logLine
	e.eventually(30*time.Second, "la línea del secreto en los logs", func() (bool, error) {
		got := appLogs("service=" + e2eApp + "&tail=1000")
		i := slices.IndexFunc(got, masked)
		if i < 0 {
			return false, fmt.Errorf("%+v", got)
		}
		first = got[i]
		return true, nil
	})
	if first.Task != e2eApp+".1" || first.Stream != "stdout" || !strings.HasSuffix(first.Time, "Z") {
		t.Fatalf("la línea del secreto: %+v", first)
	}
	since := appLogs("service=" + e2eApp + "&since=" + url.QueryEscape(first.Time))
	if !slices.Contains(since, first) {
		t.Fatalf("since=%s no trajo de nuevo la línea %+v: %+v", first.Time, first, since)
	}

	// 5. Un dominio, con el token de Cloudflare y la IP pública en Ajustes.
	st := e.object("PATCH", "/api/v1/settings", map[string]any{"cloudflare_token": cfToken, "public_ip": e2ePublicIP}, "", http.StatusOK)
	if st["cloudflare_token_set"] != true || st["cloudflare_zones"] != 1.0 || strings.Contains(fmt.Sprint(st), cfToken) {
		t.Fatalf("PATCH /settings: %v", st)
	}
	host := "web." + e.project + ".test"
	dom := e.object("POST", A+"/domains", map[string]any{"hostname": host, "target_service": e2eApp, "target_port": 80}, "", http.StatusCreated)
	if dom["tls_mode"] != "cloudflare" || dom["cf_zone_id"] != zoneID || dom["cf_record_id"] == nil || dom["status"] != "pending" {
		t.Fatalf("POST …/domains: %v", dom)
	}
	recordFor := func(name string) []string {
		var out []string
		for _, r := range e.cf.Records(zoneID) {
			if r.Name == name {
				out = append(out, r.Type+" "+r.Content)
			}
		}
		return out
	}
	if got := recordFor(host); !slices.Equal(got, []string{"A " + e2ePublicIP}) {
		t.Fatalf("los registros de %s en Cloudflare: %v", host, got)
	}
	nginxOK := func() (bool, error) {
		code, body, err := e.viaCaddy(host)
		if err != nil {
			return false, err
		}
		if code != http.StatusOK || !strings.Contains(body, "Welcome to nginx!") {
			return false, fmt.Errorf("%d %.200q", code, body)
		}
		return true, nil
	}
	e.eventually(60*time.Second, "muelle-caddy enruta "+host+" a la app", nginxOK)
	if got := e.object("GET", "/api/v1/proxy", nil, "", http.StatusOK); got["status"] != "synced" {
		t.Fatalf("GET /proxy con el dominio: %v", got)
	}
	// La verificación de ingress (al segundo de crearlo) lee el certificado
	// de la CA interna. La hoja dura 12 h, menos que los 14 días de active:
	// sigue pending, con el emisor y el vencimiento.
	e.eventually(60*time.Second, "la verificación del certificado de "+host, func() (bool, error) {
		list := e2eDecode[[]map[string]any](t, e.call("GET", A+"/domains", nil, "", http.StatusOK))
		if len(list) != 1 {
			return false, fmt.Errorf("dominios: %v", list)
		}
		issuer, _ := list[0]["cert_issuer"].(string)
		if !strings.HasPrefix(issuer, "Caddy Local Authority") || list[0]["cert_expires_at"] == nil || list[0]["status"] != "pending" {
			return false, fmt.Errorf("%v", list[0])
		}
		return true, nil
	})

	// 6. Un redeploy, y el rollback al deploy anterior.
	id2 := int64(e.object("POST", A+"/deploys", map[string]any{}, tok, http.StatusAccepted)["id"].(float64))
	if status, log := e.deployLog(A, id2, tok); status != "succeeded" || strings.Contains(log, secret) {
		t.Fatalf("el redeploy %d: %s\n%s", id2, status, log)
	}
	rb := e.object("POST", A+"/rollback", nil, tok, http.StatusAccepted)
	id3 := int64(rb["id"].(float64))
	if status, log := e.deployLog(A, id3, tok); status != "succeeded" || strings.Contains(log, secret) {
		t.Fatalf("el rollback %d: %s\n%s", id3, status, log)
	}
	d3 := e.object("GET", fmt.Sprintf("%s/deploys/%d", A, id3), nil, tok, http.StatusOK)
	if images3, _ := d3["images"].(map[string]any); d3["trigger"] != "rollback" || images3[e2eApp] != images1[e2eApp] {
		t.Fatalf("el rollback: %v", d3)
	}
	// El rollback pide la ref con digest: el panel ve que fija el servicio.
	if requested3, _ := d3["requested_images"].(map[string]any); requested3[e2eApp] != images1[e2eApp] || d3["routes_warning"] != false {
		t.Fatalf("el rollback: %v", d3)
	}
	if s, err := e.docker.InspectService(ctx, naming.Service(e.project, e2eStage, e2eApp, e2eApp)); err != nil ||
		s.Labels[naming.LabelDeploy] != strconv.FormatInt(id3, 10) {
		t.Fatalf("Swarm después del rollback: %+v %v", s, err)
	}
	e.eventually(60*time.Second, "muelle-caddy enruta "+host+" después del rollback", nginxOK)

	// 7. Borrar el dominio, la app y el stage. Con el dominio, la app no se
	// borra (catalog cuenta los dominios de ingress).
	resp, b := e.send("DELETE", A, nil, "")
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(b), `"not_empty"`) {
		t.Fatalf("DELETE de la app con un dominio: %d %s", resp.StatusCode, b)
	}
	e.call("DELETE", A+"/domains/"+host, nil, "", http.StatusNoContent)
	if got := recordFor(host); len(got) != 0 {
		t.Fatalf("quedaron registros de %s en Cloudflare: %v", host, got)
	}
	e.eventually(30*time.Second, "Caddy sin la ruta de "+host, func() (bool, error) {
		routes, err := e.caddyRoutes()
		if err != nil {
			return false, err
		}
		if strings.Contains(routes, host) {
			return false, fmt.Errorf("sigue la ruta: %s", routes)
		}
		return true, nil
	})
	if ok, _ := nginxOK(); ok {
		t.Fatalf("muelle-caddy sigue enrutando %s a la app", host)
	}
	e.call("DELETE", A, nil, "", http.StatusNoContent)
	stack := naming.Stack(e.project, e2eStage, e2eApp)
	e.eventually(90*time.Second, "Swarm sin los servicios ni los contenedores de "+stack, func() (bool, error) {
		svcs, err := e.docker.ListServices(ctx, map[string]string{naming.LabelProject: e.project})
		if err != nil {
			return false, err
		}
		if left := e.stackContainers(ctx, stack); len(svcs) != 0 || len(left) != 0 {
			return false, fmt.Errorf("servicios %d, contenedores %v", len(svcs), left)
		}
		return true, nil
	})
	e.call("DELETE", S, nil, "", http.StatusNoContent)
	e.eventually(60*time.Second, "sin la red "+network, func() (bool, error) {
		_, err := e.docker.InspectNetwork(ctx, network)
		if errors.Is(err, docker.ErrNotFound) {
			return true, nil
		}
		return false, fmt.Errorf("la red sigue: %v", err)
	})
	if c, err := e.docker.InspectContainer(ctx, e.opts.CaddyContainer); err != nil || slices.Contains(c.Networks, network) || !c.Running {
		t.Fatalf("muelle-caddy después de borrar el stage: %+v %v", c, err)
	}
	if routes, err := e.caddyRoutes(); err != nil || strings.Contains(routes, e.project+"_") {
		t.Fatalf("las rutas de Caddy después de borrar todo: %s %v", routes, err)
	}

	// La auditoría tiene todo, en orden, y nunca el token de Cloudflare ni
	// el valor de la variable.
	audit := e.call("GET", "/api/v1/audit?limit=200", nil, "", http.StatusOK)
	var actions []string
	events := e2eDecode[[]struct {
		Action  string `json:"action"`
		Details any    `json:"details"`
	}](t, audit)
	for i := len(events) - 1; i >= 0; i-- { // la API los da del más nuevo al más viejo: acá, en orden
		actions = append(actions, events[i].Action)
		if a := events[i].Action; a == "env.replace" && strings.Contains(fmt.Sprint(events[i].Details), secret) {
			t.Fatalf("la auditoría de %s trae el valor: %v", a, events[i].Details)
		}
	}
	want := []string{"project.create", "stage.create", "app.create", "env.replace", "token.create", "deploy.create",
		"settings.update", "domain.create", "deploy.create", "deploy.create", "domain.delete", "app.delete", "stage.delete"}
	if i := 0; !slices.ContainsFunc(actions, func(a string) bool {
		if i < len(want) && a == want[i] {
			i++
		}
		return i == len(want)
	}) {
		t.Fatalf("la auditoría no tiene, en orden, %v: %v", want, actions)
	}
	if bytes.Contains(audit, []byte(cfToken)) {
		t.Fatalf("la auditoría trae el token de Cloudflare: %s", audit)
	}
}
