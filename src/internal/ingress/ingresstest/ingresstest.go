// Package ingresstest siembra la base para los tests de los paquetes de
// ingress: proyectos, stages y apps (de catalog), los ajustes del proxy (de
// admin) y las credenciales de registries (de deploy), con el formato que usa
// cada dueño. Cualquier error corta el test.
package ingresstest

import (
	"context"
	"fmt"
	"testing"

	"github.com/CPU-commits/muelle/src/package/cipher"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

func exec(t testing.TB, db *sqlite.DB, q string, args ...any) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.Conn(ctx).ExecContext(ctx, q, args...); err != nil {
		t.Fatalf("ingresstest: %s: %v", q, err)
	}
}

// Stage crea el proyecto y el stage si faltan (sin apps).
func Stage(t testing.TB, db *sqlite.DB, project, stage string) {
	t.Helper()
	exec(t, db, `INSERT INTO projects (slug, name, created_at) VALUES (?, ?, 0) ON CONFLICT (slug) DO NOTHING`, project, project)
	exec(t, db, `INSERT INTO stages (project_id, slug, created_at) SELECT id, ?, 0 FROM projects WHERE slug = ?
		ON CONFLICT (project_id, slug) DO NOTHING`, stage, project)
}

// App crea la app (y su proyecto y su stage si faltan) y devuelve su id.
// kind es "service" (con spec '{}') o "compose" (con compose_yaml 'services: {}').
func App(t testing.TB, db *sqlite.DB, project, stage, app, kind string) int64 {
	t.Helper()
	Stage(t, db, project, stage)
	var spec, compose any
	switch kind {
	case "service":
		spec = "{}"
	case "compose":
		compose = "services: {}"
	default:
		t.Fatalf("ingresstest: tipo de app %q", kind)
	}
	ctx := context.Background()
	var id int64
	err := db.Conn(ctx).QueryRowContext(ctx, `INSERT INTO apps (stage_id, slug, kind, spec, compose_yaml, config_changed_at, created_at)
		SELECT s.id, ?, ?, ?, ?, 0, 0 FROM stages s JOIN projects p ON p.id = s.project_id WHERE p.slug = ? AND s.slug = ?
		RETURNING id`, app, kind, spec, compose, project, stage).Scan(&id)
	if err != nil {
		t.Fatalf("ingresstest: app %s/%s/%s: %v", project, stage, app, err)
	}
	return id
}

// SetProxySettings guarda p como lo guarda admin: cada clave de Encode en
// texto, y el token cifrado con settings:cloudflare_token (o lo borra si p no
// trae uno).
func SetProxySettings(t testing.TB, db *sqlite.DB, c cipher.Cipher, p entity.ProxySettings) {
	t.Helper()
	const upsert = `INSERT INTO settings (key, value, encrypted) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, encrypted = excluded.encrypted`
	for k, v := range p.Encode() {
		exec(t, db, upsert, k, v, 0)
	}
	if p.CloudflareToken == "" {
		exec(t, db, `DELETE FROM settings WHERE key = ?`, entity.SettingCloudflareToken)
		return
	}
	exec(t, db, upsert, entity.SettingCloudflareToken, c.Encrypt([]byte(p.CloudflareToken), entity.SettingCloudflareTokenAAD), 1)
}

// Registry guarda la credencial de un registry como deploy: la contraseña
// cifrada con registries:password:<id>.
func Registry(t testing.TB, db *sqlite.DB, c cipher.Cipher, host, user, pass string) {
	t.Helper()
	ctx := context.Background()
	err := db.InTx(ctx, func(ctx context.Context) error {
		var id int64
		if err := db.Conn(ctx).QueryRowContext(ctx, `INSERT INTO registries (host, username, password, created_at, updated_at)
			VALUES (?, ?, x'', 0, 0) RETURNING id`, host, user).Scan(&id); err != nil {
			return err
		}
		ct := c.Encrypt([]byte(pass), fmt.Sprintf("registries:password:%d", id))
		_, err := db.Conn(ctx).ExecContext(ctx, `UPDATE registries SET password = ? WHERE id = ?`, ct, id)
		return err
	})
	if err != nil {
		t.Fatalf("ingresstest: registry %s: %v", host, err)
	}
}
