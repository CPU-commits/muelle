package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/package/cipher"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// RegistriesReader lee las credenciales de la tabla registries (de deploy,
// §4.1 regla 1) y descifra la contraseña (AAD registries:password:<id>, §12).
type RegistriesReader struct {
	db     *sqlite.DB
	cipher cipher.Cipher
}

var _ service.Registries = (*RegistriesReader)(nil)

func NewRegistriesReader(db *sqlite.DB, c cipher.Cipher) *RegistriesReader {
	return &RegistriesReader{db: db, cipher: c}
}

// RegistryPasswordAAD es la AAD de la contraseña de la fila id de registries.
func RegistryPasswordAAD(id int64) string { return fmt.Sprintf("registries:password:%d", id) }

func (r *RegistriesReader) Credential(ctx context.Context, host string) (*service.Credential, error) {
	var id int64
	var user string
	var password []byte
	err := r.db.Conn(ctx).QueryRowContext(ctx,
		`SELECT id, username, password FROM registries WHERE host = ?`, host).Scan(&id, &user, &password)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pt, err := r.cipher.Decrypt(password, RegistryPasswordAAD(id))
	if err != nil {
		return nil, fmt.Errorf("registries: descifrando la contraseña de %s: %w", host, err)
	}
	return &service.Credential{Host: host, Username: user, Password: string(pt)}, nil
}
