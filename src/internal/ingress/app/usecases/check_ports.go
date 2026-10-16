package usecases

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// checkPortsTimeout es el plazo de CheckProxyPorts, sobre el ctx de la petición.
const checkPortsTimeout = 30 * time.Second

// CheckProxyPorts prueba, antes de que admin guarde otros puertos del proxy
// (PATCH /settings, contrato §4.1), que muelle-caddy los pueda tomar en el
// host. Si uno está ocupado, el PATCH no guarda nada: así un puerto ocupado
// no deja el panel sin Caddy.
type CheckProxyPorts struct {
	runtime  service.CaddyRuntime
	defImage string
	log      *slog.Logger
}

// NewCheckProxyPorts recibe la imagen de Caddy de esta versión
// (naming.CaddyImageFor), que decide si el proxy está habilitado.
func NewCheckProxyPorts(runtime service.CaddyRuntime, defImage string, log *slog.Logger) *CheckProxyPorts {
	return &CheckProxyPorts{runtime: runtime, defImage: defImage, log: log}
}

// Execute, con un plazo de 30 s sobre ctx:
//   - sin el proxy habilitado con next (desarrollo), o si los puertos no
//     cambian, no hace nada;
//   - prueba los puertos que cambian, de a un campo (proxy_http_port en TCP,
//     proxy_https_port en TCP y UDP), con la imagen del muelle-caddy que sirve
//     (o la de next, si no hay uno);
//   - no prueba los que ya publica el muelle-caddy que sirve: los suelta al
//     recrearse.
//
// Un puerto ocupado es un *validate.Error en su campo (422), y Docker caído,
// su error (503). Si la prueba no se puede hacer por otra cosa, lo registra y
// devuelve nil: la protección de fondo es la vuelta atrás (EnsureCaddy.Recreate).
func (u *CheckProxyPorts) Execute(ctx context.Context, old, next shared.ProxySettings) error {
	if !next.Enabled(u.defImage) || (old.HTTPPort == next.HTTPPort && old.HTTPSPort == next.HTTPSPort) {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, checkPortsTimeout)
	defer cancel()
	s, serving, err := u.runtime.Serving(ctx)
	switch {
	case errors.Is(err, shared.ErrDockerUnavailable):
		return fmt.Errorf("ingress: probando los puertos del proxy: %w", err)
	case err != nil:
		u.log.Warn("ingress: no se pudo revisar muelle-caddy; los puertos nuevos no se prueban", "error", err)
		return nil
	}
	image := next.CaddyImageOr(u.defImage)
	var held []service.HostPort
	if serving {
		image = s.Image
		held = []service.HostPort{
			{Port: s.HTTPPort, Protocol: "tcp"}, {Port: s.HTTPSPort, Protocol: "tcp"}, {Port: s.HTTPSPort, Protocol: "udp"},
		}
	}
	fields := []struct {
		key        string
		old, port  int
		protocols  []string
		protoLabel string
	}{
		{shared.SettingProxyHTTPPort, old.HTTPPort, next.HTTPPort, []string{"tcp"}, ""},
		{shared.SettingProxyHTTPSPort, old.HTTPSPort, next.HTTPSPort, []string{"tcp", "udp"}, " (en TCP o en UDP)"},
	}
	for _, f := range fields {
		if f.port == f.old {
			continue
		}
		var ports []service.HostPort
		for _, proto := range f.protocols {
			if hp := (service.HostPort{Port: f.port, Protocol: proto}); !slices.Contains(held, hp) {
				ports = append(ports, hp)
			}
		}
		if len(ports) == 0 {
			continue
		}
		err := u.runtime.ProbePorts(ctx, image, ports)
		switch {
		case errors.Is(err, service.ErrPortInUse):
			return validate.Errorf(f.key, "el puerto %d del host ya está en uso%s: libéralo o elige otro", f.port, f.protoLabel)
		case err != nil:
			return fmt.Errorf("ingress: probando el puerto %d del proxy: %w", f.port, err)
		}
	}
	return nil
}
