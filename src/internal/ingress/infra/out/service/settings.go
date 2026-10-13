package service

import (
	"context"
	"fmt"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/package/cipher"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// ProxySettingsReader lee de la tabla settings (de admin, §4.1 regla 1) los
// ajustes del proxy y el DNS, con el formato de entity.ProxySettings.Encode,
// y descifra el token de Cloudflare (AAD settings:cloudflare_token).
type ProxySettingsReader struct {
	db     *sqlite.DB
	cipher cipher.Cipher
}

var _ service.Settings = (*ProxySettingsReader)(nil)

func NewProxySettingsReader(db *sqlite.DB, c cipher.Cipher) *ProxySettingsReader {
	return &ProxySettingsReader{db: db, cipher: c}
}

func (r *ProxySettingsReader) ProxySettings(ctx context.Context) (entity.ProxySettings, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT key, value, encrypted FROM settings`)
	if err != nil {
		return entity.ProxySettings{}, err
	}
	defer rows.Close()
	wanted := map[string]bool{entity.SettingCloudflareToken: true}
	for _, k := range entity.ProxySettingKeys {
		wanted[k] = true
	}
	raw := map[string]string{}
	var token string
	for rows.Next() {
		var key string
		var value []byte
		var encrypted bool
		if err := rows.Scan(&key, &value, &encrypted); err != nil {
			return entity.ProxySettings{}, err
		}
		if !wanted[key] {
			continue
		}
		if key == entity.SettingCloudflareToken {
			if !encrypted {
				return entity.ProxySettings{}, fmt.Errorf("settings: %s no está cifrado", key)
			}
			pt, err := r.cipher.Decrypt(value, entity.SettingCloudflareTokenAAD)
			if err != nil {
				return entity.ProxySettings{}, fmt.Errorf("settings: descifrando %s: %w", key, err)
			}
			token = string(pt)
			continue
		}
		raw[key] = string(value)
	}
	if err := rows.Err(); err != nil {
		return entity.ProxySettings{}, err
	}
	return entity.DecodeProxySettings(raw, token)
}
