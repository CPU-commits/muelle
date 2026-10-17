package service

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

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

// panelDNSKeys son las claves de PanelDNS, en el orden de panelDNSFields.
var panelDNSKeys = [3]string{entity.SettingPanelCFZoneID, entity.SettingPanelCFRecordID, entity.SettingPanelCFRecordIDAAAA}

func panelDNSFields(d *service.PanelDNS) [3]*string {
	return [3]*string{&d.ZoneID, &d.RecordID, &d.RecordIDAAAA}
}

// PanelDNS lee las claves panel_cf_* (contrato §3.6), que escribe solo
// ingress, en una sola consulta.
func (r *ProxySettingsReader) PanelDNS(ctx context.Context) (service.PanelDNS, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT key, value, encrypted FROM settings WHERE key IN (?, ?, ?)`,
		panelDNSKeys[0], panelDNSKeys[1], panelDNSKeys[2])
	if err != nil {
		return service.PanelDNS{}, err
	}
	defer rows.Close()
	var out service.PanelDNS
	dst := panelDNSFields(&out)
	for rows.Next() {
		var key string
		var value []byte
		var encrypted bool
		if err := rows.Scan(&key, &value, &encrypted); err != nil {
			return service.PanelDNS{}, err
		}
		if encrypted {
			return service.PanelDNS{}, fmt.Errorf("settings: %s no debería estar cifrado", key)
		}
		for i, k := range panelDNSKeys {
			if k == key {
				*dst[i] = string(value)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return service.PanelDNS{}, err
	}
	return out, nil
}

// SavePanelDNS escribe las claves no vacías (sin cifrar, en texto) y borra
// las vacías, todo en una transacción.
func (r *ProxySettingsReader) SavePanelDNS(ctx context.Context, p service.PanelDNS) error {
	src := panelDNSFields(&p)
	return r.db.InTx(ctx, func(ctx context.Context) error {
		for i, key := range panelDNSKeys {
			var err error
			if v := *src[i]; v != "" {
				_, err = r.db.Conn(ctx).ExecContext(ctx, `INSERT INTO settings (key, value, encrypted) VALUES (?, ?, 0)
					ON CONFLICT (key) DO UPDATE SET value = excluded.value, encrypted = 0`, key, v)
			} else {
				_, err = r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// SaveZones guarda cloudflare_zones = zones solo si el token guardado sigue
// siendo token (contrato §3.6): lee, descifra y compara en la misma
// transacción en que escribe, así un PATCH /settings que cambió o borró el
// token mientras ingress listaba las zonas no queda con un número viejo.
func (r *ProxySettingsReader) SaveZones(ctx context.Context, token string, zones int) (bool, error) {
	if token == "" {
		return false, nil
	}
	saved := false
	err := r.db.InTx(ctx, func(ctx context.Context) error {
		var value []byte
		var encrypted bool
		err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT value, encrypted FROM settings WHERE key = ?`,
			entity.SettingCloudflareToken).Scan(&value, &encrypted)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if !encrypted {
			return fmt.Errorf("settings: %s no está cifrado", entity.SettingCloudflareToken)
		}
		pt, err := r.cipher.Decrypt(value, entity.SettingCloudflareTokenAAD)
		if err != nil {
			return fmt.Errorf("settings: descifrando %s: %w", entity.SettingCloudflareToken, err)
		}
		if subtle.ConstantTimeCompare(pt, []byte(token)) != 1 {
			return nil
		}
		if _, err := r.db.Conn(ctx).ExecContext(ctx, `INSERT INTO settings (key, value, encrypted) VALUES (?, ?, 0)
			ON CONFLICT (key) DO UPDATE SET value = excluded.value, encrypted = 0`,
			entity.SettingCloudflareZones, strconv.Itoa(zones)); err != nil {
			return err
		}
		saved = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return saved, nil
}

// RollbackPorts vuelve los puertos del proxy a to solo si los guardados
// siguen siendo from (contrato §3.5): lee y escribe en la misma transacción,
// así un PATCH /settings que los cambió mientras muelle-caddy se recreaba
// gana. Una clave sin guardar vale lo de por defecto (80 y 443), como en
// ProxySettings.
func (r *ProxySettingsReader) RollbackPorts(ctx context.Context, from, to service.ProxyPorts) (bool, error) {
	saved := false
	err := r.db.InTx(ctx, func(ctx context.Context) error {
		cur := service.ProxyPorts{HTTPPort: entity.DefaultProxyHTTPPort, HTTPSPort: entity.DefaultProxyHTTPSPort}
		for key, dst := range map[string]*int{entity.SettingProxyHTTPPort: &cur.HTTPPort, entity.SettingProxyHTTPSPort: &cur.HTTPSPort} {
			var value string
			err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
			switch {
			case errors.Is(err, sql.ErrNoRows):
				continue
			case err != nil:
				return err
			}
			if value == "" {
				continue
			}
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("settings: %s = %q no es un puerto", key, value)
			}
			*dst = n
		}
		if cur != from {
			return nil
		}
		for key, v := range map[string]int{entity.SettingProxyHTTPPort: to.HTTPPort, entity.SettingProxyHTTPSPort: to.HTTPSPort} {
			if _, err := r.db.Conn(ctx).ExecContext(ctx, `INSERT INTO settings (key, value, encrypted) VALUES (?, ?, 0)
				ON CONFLICT (key) DO UPDATE SET value = excluded.value, encrypted = 0`, key, strconv.Itoa(v)); err != nil {
				return err
			}
		}
		saved = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return saved, nil
}
