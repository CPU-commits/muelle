package usecases

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/distribution/reference"
)

// maxCauseRunes es lo máximo de un error de Docker que va al mensaje de
// GET /proxy.
const maxCauseRunes = 300

// EnsureCaddy deja muelle-caddy como piden los ajustes (contrato §3.4): la
// imagen, los puertos, la red panel y las de todos los stages. Lo llaman el
// job "proxy" y la recreación agendada (Recreator), de a uno.
type EnsureCaddy struct {
	mu         sync.Mutex
	runtime    service.CaddyRuntime
	settings   service.Settings
	catalog    service.Catalog
	registries service.Registries
	state      *ProxyState
	defImage   string
	log        *slog.Logger
}

// NewEnsureCaddy recibe la imagen de Caddy de esta versión
// (naming.CaddyImageFor), la que va si caddy_image está vacío.
func NewEnsureCaddy(runtime service.CaddyRuntime, settings service.Settings, catalog service.Catalog,
	registries service.Registries, state *ProxyState, defImage string, log *slog.Logger) *EnsureCaddy {
	return &EnsureCaddy{runtime: runtime, settings: settings, catalog: catalog, registries: registries,
		state: state, defImage: defImage, log: log}
}

// Execute, de a uno: sin el proxy habilitado devuelve (false, nil) sin tocar
// Docker. Si no, arma lo que pide (la imagen, la credencial del registry de
// su host si hay, los puertos y las redes de todos los stages) y llama a
// runtime.Ensure. El resultado va al estado de GET /proxy: ContainerOK, o
// ContainerFailed con un mensaje en español (el puerto ocupado, la imagen que
// no baja y Docker caído tienen el suyo). Un error leyendo la base no cambia
// el estado. Nunca registra want ni la credencial.
//
// Si lo creó, vuelve a leer los stages y lo conecta a las redes de los que
// no estaban en la primera lectura (connectNewStages).
func (u *EnsureCaddy) Execute(ctx context.Context) (recreated bool, err error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	p, err := u.settings.ProxySettings(ctx)
	if err != nil {
		return false, fmt.Errorf("ingress: leyendo los ajustes del proxy: %w", err)
	}
	if !p.Enabled(u.defImage) {
		return false, nil
	}
	image := p.CaddyImageOr(u.defImage)
	host := imageHost(image)
	var auth *service.Credential
	if host != "" {
		if auth, err = u.registries.Credential(ctx, host); err != nil {
			return false, fmt.Errorf("ingress: leyendo la credencial del registry %s: %w", host, err)
		}
	}
	stages, err := u.catalog.Stages(ctx)
	if err != nil {
		return false, fmt.Errorf("ingress: leyendo los stages: %w", err)
	}
	nets := make([]string, 0, len(stages))
	for _, s := range stages {
		nets = append(nets, naming.Network(s.Project, s.Stage))
	}
	want := service.CaddyWant{Image: image, HTTPPort: p.HTTPPort, HTTPSPort: p.HTTPSPort, Networks: nets, Auth: auth}

	recreated, err = u.runtime.Ensure(ctx, want)
	if err != nil {
		password := ""
		if auth != nil {
			password = auth.Password
		}
		msg := u.message(ctx, err, p, image, host, auth != nil, password)
		u.state.ContainerFailed(msg)
		u.log.Warn("ingress: muelle-caddy no quedó como piden los ajustes", "error", hideSecret(err.Error(), password))
		return recreated, fmt.Errorf("ingress: asegurando muelle-caddy: %w", secretError{err, password})
	}
	u.state.ContainerOK()
	if recreated {
		u.log.Info("ingress: muelle-caddy creado", "imagen", image, "http", p.HTTPPort, "https", p.HTTPSPort)
		u.connectNewStages(ctx, nets)
	}
	return recreated, nil
}

// connectNewStages conecta el muelle-caddy recién creado a las redes de los
// stages que no están en nets (las de la lectura de antes de crearlo).
//
// La ventana va desde la primera lectura de Stages hasta CreateContainer, e
// incluye la bajada de la imagen y el StopContainer (hasta 10 s). Un
// EnsureStageNetwork que corre ahí, con el contenedor viejo ya borrado o a
// punto de serlo, no conecta al nuevo, y este se crea sin esa red: las rutas
// del stage darían 502 hasta el próximo job "proxy". Solo afecta a los stages
// que faltan en la primera lectura (los nuevos): los demás ya están en nets.
// Esta lectura es posterior a la creación, así que cubre el paso 4 de un
// deploy (§7), cuyo stage ya está en la base. catalog guarda el stage recién
// después de asegurar su red; si llega tarde a esta lectura, conecta la red
// el paso 4 del primer deploy del stage (antes no hay rutas que servir) o el
// job "proxy".
//
// Un error solo se registra: el contenedor ya quedó, y el job "proxy" conecta
// las redes que falten.
func (u *EnsureCaddy) connectNewStages(ctx context.Context, nets []string) {
	stages, err := u.catalog.Stages(ctx)
	if err != nil {
		u.log.Warn("ingress: releyendo los stages después de crear muelle-caddy", "error", err)
		return
	}
	var missing []string
	for _, s := range stages {
		if n := naming.Network(s.Project, s.Stage); !slices.Contains(nets, n) {
			missing = append(missing, n)
		}
	}
	if len(missing) == 0 {
		return
	}
	if err := u.runtime.Connect(ctx, missing); err != nil {
		u.log.Warn("ingress: conectando muelle-caddy a las redes de los stages nuevos", "error", err)
		return
	}
	u.log.Info("ingress: muelle-caddy conectado a las redes de stages nuevos", "redes", missing)
}

// message es el error del contenedor para GET /proxy (contrato §3.3).
func (u *EnsureCaddy) message(ctx context.Context, err error, p shared.ProxySettings, image, host string, hasCredential bool, password string) string {
	switch {
	case errors.Is(err, shared.ErrDockerUnavailable):
		return "Docker no responde: muelle no pudo revisar muelle-caddy; se reintenta solo"
	case errors.Is(err, service.ErrPortInUse):
		return fmt.Sprintf("los puertos %d o %d del host ya están en uso, y muelle-caddy no pudo arrancar: libéralos, o cambia los puertos del proxy en Ajustes; muelle lo reintenta cada 5 minutos",
			p.HTTPPort, p.HTTPSPort)
	case errors.Is(err, service.ErrCaddyImagePull):
		var cause string
		switch {
		case errors.Is(err, docker.ErrDenied):
			cause = "el registry no dio acceso"
		case errors.Is(err, docker.ErrNotFound):
			cause = "el registry no la encontró"
		default:
			cause = strings.TrimPrefix(err.Error(), service.ErrCaddyImagePull.Error()+" "+image+": ")
			cause = truncateRunes(hideSecret(cause, password), maxCauseRunes)
		}
		msg := fmt.Sprintf("muelle no pudo bajar la imagen %s de Caddy (%s); %s", image, cause, u.previous(ctx))
		if host != "" && (errors.Is(err, docker.ErrDenied) || errors.Is(err, docker.ErrNotFound)) {
			if hasCredential {
				msg += ". El registry " + host + " no dio acceso con la credencial guardada: actualízala en Ajustes → Registries (el token puede haber vencido)"
			} else {
				msg += ". Si la imagen es privada, agrega el registry " + host + " en Ajustes → Registries, con un usuario y un token que puedan leerla"
			}
		}
		return msg
	}
	return "muelle no pudo dejar muelle-caddy como piden los ajustes: " + truncateRunes(hideSecret(err.Error(), password), maxCauseRunes)
}

// previous dice qué pasa con el contenedor anterior cuando la imagen nueva
// no bajó: sigue sirviendo si corre (D11).
func (u *EnsureCaddy) previous(ctx context.Context) string {
	if _, running, _, err := u.runtime.Status(ctx); err == nil && running {
		return "muelle-caddy sigue con la anterior"
	}
	return "muelle-caddy no está corriendo"
}

// imageHost es el host del registry de la imagen, como lo guarda deploy en
// registries (contrato §2.10); "" si la imagen no parsea (va anónima).
func imageHost(image string) string {
	named, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return ""
	}
	return reference.Domain(named)
}

// secretError es err con un secreto oculto en su texto (hideSecret);
// errors.Is y errors.As siguen viendo err.
type secretError struct {
	err    error
	secret string
}

func (e secretError) Error() string { return hideSecret(e.err.Error(), e.secret) }
func (e secretError) Unwrap() error { return e.err }

// hideSecret reemplaza secret por "***" en s; con secret "" no hace nada.
// Docker no repite la credencial en sus errores, pero el mensaje va a GET
// /proxy y al log: por las dudas.
func hideSecret(s, secret string) string {
	if secret == "" {
		return s
	}
	return strings.ReplaceAll(s, secret, "***")
}

// truncateRunes corta s a n runas, con "…" si lo cortó.
func truncateRunes(s string, n int) string {
	s = strings.ToValidUTF8(s, "�")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
