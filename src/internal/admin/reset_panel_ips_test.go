package admin

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	adminrepo "github.com/CPU-commits/muelle/src/internal/admin/infra/out/repository/sqlite"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// storedCIDRs lee el valor guardado de panel_allowed_cidrs; ok false si no está.
func storedCIDRs(t *testing.T, db *sqlite.DB) (value string, ok bool) {
	t.Helper()
	v, _, err := adminrepo.NewSettingsRepository(db).Get(context.Background(), entity.SettingPanelAllowedCIDRs)
	if err != nil {
		return "", false
	}
	return string(v), true
}

func countAudit(t *testing.T, db *sqlite.DB) int {
	t.Helper()
	ctx := context.Background()
	var n int
	if err := db.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func resetRecorder(db *sqlite.DB) *sqlite.AuditRecorder {
	return sqlite.NewAuditRecorder(db, clock.System{}, nil)
}

// muelle settings reset-panel-ips deja la lista en [] (contrato §4.1 y §5.4).
// Una lista vacía, o que no está, no se escribe; un valor que no se entiende
// se reemplaza igual.
func TestResetPanelAllowedCIDRs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stored   *string
		changed  bool
		previous []string
	}{
		{name: "sin la clave"},
		{name: "vacía", stored: new("[]")},
		{name: "texto vacío", stored: new("")},
		{name: "null", stored: new("null")},
		{name: "con rangos", stored: new(`["190.5.6.0/24","2001:db8::/32"]`), changed: true,
			previous: []string{"190.5.6.0/24", "2001:db8::/32"}},
		{name: "basura", stored: new("basura"), changed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			ctx := context.Background()
			if tc.stored != nil {
				if err := adminrepo.NewSettingsRepository(e.db).Set(ctx, entity.SettingPanelAllowedCIDRs, []byte(*tc.stored), false); err != nil {
					t.Fatal(err)
				}
			}
			res, err := ResetPanelAllowedCIDRs(ctx, e.db, resetRecorder(e.db))
			if err != nil {
				t.Fatal(err)
			}
			if res.Changed != tc.changed || !slices.Equal(res.Previous, tc.previous) {
				t.Fatalf("resultado %+v, esperaba Changed=%v Previous=%q", res, tc.changed, tc.previous)
			}
			got, ok := storedCIDRs(t, e.db)
			switch {
			case tc.changed && (!ok || got != "[]"):
				t.Fatalf("guardado %q (%v), esperaba []", got, ok)
			case !tc.changed && tc.stored == nil && ok:
				t.Fatalf("sin cambios escribió %q", got)
			case !tc.changed && tc.stored != nil && got != *tc.stored:
				t.Fatalf("sin cambios cambió %q a %q", *tc.stored, got)
			}
		})
	}
}

// El cambio deja un settings.update del sistema (no hay sesión), con la lista
// vacía y via=cli; sin cambio no queda ningún evento.
func TestResetPanelAllowedCIDRsAudit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	rec := resetRecorder(e.db)

	before := countAudit(t, e.db)
	if res, err := ResetPanelAllowedCIDRs(ctx, e.db, rec); err != nil || res.Changed {
		t.Fatalf("sin lista: %+v %v", res, err)
	}
	if n := countAudit(t, e.db); n != before {
		t.Fatalf("sin cambio se agregaron %d eventos", n-before)
	}

	if err := adminrepo.NewSettingsRepository(e.db).Set(ctx, entity.SettingPanelAllowedCIDRs, []byte(`["190.5.6.0/24"]`), false); err != nil {
		t.Fatal(err)
	}
	if res, err := ResetPanelAllowedCIDRs(ctx, e.db, rec); err != nil || !res.Changed {
		t.Fatalf("con lista: %+v %v", res, err)
	}
	if n := countAudit(t, e.db); n != before+1 {
		t.Fatalf("se agregaron %d eventos, esperaba 1", n-before)
	}
	var actor, action, target, details string
	var actorID *int64
	if err := e.db.Conn(ctx).QueryRowContext(ctx,
		`SELECT actor_type, actor_id, action, target, details FROM audit_events ORDER BY id DESC LIMIT 1`).
		Scan(&actor, &actorID, &action, &target, &details); err != nil {
		t.Fatal(err)
	}
	if actor != "system" || actorID != nil || action != "settings.update" || target != "settings" ||
		details != `{"panel_allowed_cidrs":[],"via":"cli"}` {
		t.Fatalf("evento: actor=%s/%v action=%s target=%s details=%s", actor, actorID, action, target, details)
	}

	// Ya vacía: ningún evento más.
	if res, err := ResetPanelAllowedCIDRs(ctx, e.db, rec); err != nil || res.Changed {
		t.Fatalf("otra vez: %+v %v", res, err)
	}
	if n := countAudit(t, e.db); n != before+1 {
		t.Fatalf("la segunda vez agregó eventos: %d", n-before)
	}
}

// Solo cambia la lista: los demás ajustes quedan, y la API la ve vacía.
func TestResetPanelAllowedCIDRsKeepsOtherSettings(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	repo := adminrepo.NewSettingsRepository(e.db)
	if err := repo.Set(ctx, entity.SettingProxyHTTPSPort, []byte("8443"), false); err != nil {
		t.Fatal(err)
	}
	if err := repo.Set(ctx, entity.SettingPanelAllowedCIDRs, []byte(`["190.5.6.0/24"]`), false); err != nil {
		t.Fatal(err)
	}
	if _, err := ResetPanelAllowedCIDRs(ctx, e.db, resetRecorder(e.db)); err != nil {
		t.Fatal(err)
	}
	if v, _, err := repo.Get(ctx, entity.SettingProxyHTTPSPort); err != nil || string(v) != "8443" {
		t.Fatalf("proxy_https_port=%q %v", v, err)
	}

	w := e.c.Do("GET", "/api/v1/settings", nil, httptestutil.WithSession("sin"))
	httptestutil.Expect(t, w, http.StatusOK)
	got := httptestutil.Decode[map[string]any](t, w)
	if list, ok := got["panel_allowed_cidrs"].([]any); !ok || len(list) != 0 {
		t.Fatalf("panel_allowed_cidrs=%v", got["panel_allowed_cidrs"])
	}
	if got["proxy_https_port"] != float64(8443) {
		t.Fatalf("proxy_https_port=%v", got["proxy_https_port"])
	}
}
