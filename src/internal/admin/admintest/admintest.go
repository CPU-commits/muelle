// Package admintest tiene ayudas para los tests del módulo admin.
package admintest

import (
	"context"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// InsertUser crea un usuario con SQL directo (sin pasar por identity) y
// devuelve su id. Con withTOTP el usuario tiene un secreto TOTP configurado.
func InsertUser(t testing.TB, db *sqlite.DB, username string, withTOTP bool) int64 {
	t.Helper()
	var secret any
	if withTOTP {
		secret = []byte("secreto-totp")
	}
	ctx := context.Background()
	res, err := db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO users (username, password_hash, totp_secret, created_at) VALUES (?, 'x', ?, ?)`,
		username, secret, sqlite.Millis(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
