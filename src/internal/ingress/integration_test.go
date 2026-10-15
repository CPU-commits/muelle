//go:build integration

package ingress

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// Caddy real (caddy:2-alpine, con la CA interna) acepta la configuración que
// arma muelle, la recibe por el socket de la admin API y enruta: por hostname
// a la app por la red del stage, y en el panel por client_ip (spec §15).
func TestIntegrationCaddyAcceptsConfig(t *testing.T) {
	e := newITEnv(t, 1)
	m := e.module()
	ctx := context.Background()

	// 1. El puente, la red del stage (por el módulo: si conecta algo, es
	// it3-<id>-caddy, nunca muelle-caddy), el backend y Caddy.
	e.startAdminBridge()
	ingresstest.Stage(t, e.db, e.project, "s1")
	if err := m.EnsureStageNetwork(ctx, e.project, "s1"); err != nil {
		t.Fatal(err)
	}
	e.startBackend("s1", "web")
	e.startCaddyManually("s1")

	// 2. El dominio web.it3<id>.test → web:8080.
	appID := ingresstest.App(t, e.db, e.project, "s1", "web", "service")
	web := e.host("web")
	e.addDomain(appID, web, "web", 8080)
	e.settings(shared.DefaultProxySettings())

	// 3. Caddy está arrancando: el primer POST /load puede no llegar.
	e.eventually(30*time.Second, "SyncRoutes", func() (bool, error) {
		err := m.SyncRoutes(ctx)
		return err == nil, err
	})
	if status, msg, _ := m.state.Snapshot(true); status != "synced" {
		t.Fatalf("estado después del Load: %s %q", status, msg)
	}

	// 4. Por hostname, a la app por la red del stage.
	e.eventually(30*time.Second, "GET "+web, e.expectGet(web, "/", http.StatusOK, "hola web"))

	// 5. Con IPs del panel, la del cliente (la del bridge de Docker) no está:
	// 403. Prueba el matcher client_ip en Caddy real.
	p := shared.DefaultProxySettings()
	p.PanelAllowedCIDRs = []string{"203.0.113.0/24"}
	e.settings(p)
	if err := m.SyncRoutes(ctx); err != nil {
		t.Fatal(err)
	}
	e.eventually(15*time.Second, "panel con IPs", e.expectGet(e.panelHost(), "/", http.StatusForbidden, ""))

	// 6. Sin la lista, la ruta existe y va a muelle_muelle:8080, que acá no
	// existe: 502. La app sigue respondiendo.
	e.settings(shared.DefaultProxySettings())
	if err := m.SyncRoutes(ctx); err != nil {
		t.Fatal(err)
	}
	e.eventually(15*time.Second, "panel sin IPs", e.expectGet(e.panelHost(), "/", http.StatusBadGateway, ""))
	e.eventually(15*time.Second, "GET "+web+" otra vez", e.expectGet(web, "/", http.StatusOK, "hola web"))
}
