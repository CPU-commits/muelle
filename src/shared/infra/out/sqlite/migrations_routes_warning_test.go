package sqlite

import (
	"context"
	"testing"
)

// routesWarningLine es la línea que escribe el paso 7 de un deploy cuando no
// pudo actualizar Caddy (contrato §2.11), con su hora.
const routesWarningLine = "2026-10-10T14:20:40.001Z advertencia: no se pudo actualizar Caddy; " +
	"el motivo está en el estado del proxy (GET /api/v1/proxy)\n"

// 0005 suma deploys.routes_warning (contrato §2.1) y la llena en los deploys
// que ya había: 1 en un succeeded cuyo log trae la advertencia del paso 7,
// con su hora adelante; 0 en el resto.
func TestDeploysRoutesWarningMigration(t *testing.T) {
	d := openAt(t, 4)
	app := seedApp(t, d)
	const ok = "2026-10-10T14:20:39.000Z api: 1/1 réplicas corriendo\n"
	deploys := []struct {
		status, log string
		want        int
	}{
		{"succeeded", ok + routesWarningLine + "2026-10-10T14:20:41.002Z succeeded en 52 s\n", 1},
		{"succeeded", ok + "2026-10-10T14:20:40.001Z rutas de Caddy actualizadas\n", 0},
		// El texto sin la hora adelante no es la línea de muelle (una salida
		// del CLI que lo cita).
		{"succeeded", "2026-10-10T14:20:39.000Z  x: advertencia: no se pudo actualizar Caddy; el motivo\n", 0},
		// En un failed, el texto puede ser de una tarea de la app que falló.
		{"failed", ok + routesWarningLine, 0},
		{"queued", "", 0},
	}
	for i, dep := range deploys {
		mustExec(t, d, insertDeploy, app, dep.status, "{}", "api", "token", 4, "ci", 0)
		mustExec(t, d, `UPDATE deploys SET log = ? WHERE id = ?`, dep.log, i+1)
	}

	if err := d.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	for i, dep := range deploys {
		var got int
		if err := d.Conn(ctx).QueryRowContext(ctx, `SELECT routes_warning FROM deploys WHERE id = ?`, i+1).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != dep.want {
			t.Errorf("deploy %d (%s): routes_warning = %d, esperaba %d", i+1, dep.status, got, dep.want)
		}
	}
}

// En una base nueva, un deploy nace con routes_warning 0, que solo puede
// ser 0 o 1 y nunca NULL.
func TestDeploysRoutesWarningColumn(t *testing.T) {
	d := newTestDB(t)
	app := seedApp(t, d)
	mustExec(t, d, insertDeploy, app, "running", "{}", "api", "token", 4, "ci", 0)
	ctx := context.Background()
	var got int
	if err := d.Conn(ctx).QueryRowContext(ctx, `SELECT routes_warning FROM deploys WHERE id = 1`).Scan(&got); err != nil || got != 0 {
		t.Fatalf("por defecto: %d %v", got, err)
	}
	mustExec(t, d, `UPDATE deploys SET routes_warning = 1 WHERE id = 1`)
	mustFail(t, d, "routes_warning fuera de 0 y 1", `UPDATE deploys SET routes_warning = 2 WHERE id = 1`)
	mustFail(t, d, "routes_warning NULL", `UPDATE deploys SET routes_warning = NULL WHERE id = 1`)
}
