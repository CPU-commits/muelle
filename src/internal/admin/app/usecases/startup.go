package usecases

import (
	"context"
	"errors"
	"slices"
	"strconv"

	"github.com/CPU-commits/muelle/src/internal/admin/app/dto"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/repository"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/cipher"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// ApplyStartupSettings guarda, en cada arranque, lo que viene de la
// configuración de muelle y no del panel (§3.3 paso 6).
type ApplyStartupSettings struct {
	settings repository.SettingsRepository
	cipher   cipher.Cipher
	audit    audit.Recorder
	tx       sharedrepo.Transactor
}

func NewApplyStartupSettings(settings repository.SettingsRepository, c cipher.Cipher, rec audit.Recorder, tx sharedrepo.Transactor) *ApplyStartupSettings {
	return &ApplyStartupSettings{settings: settings, cipher: c, audit: rec, tx: tx}
}

// Execute, en una transacción:
//   - panel_domain se escribe siempre con in.PanelDomain (el host de
//     MUELLE_PANEL_ORIGIN): no se edita en el panel, así nunca hay dos fuentes;
//   - in.Init se aplica solo en el primer arranque de la base, el que guarda
//     la marca entity.KeyInitApplied. install deja montado el secreto del
//     token y las variables MUELLE_INIT_*, así que sin la marca un reinicio
//     devolvería lo que el admin borró en el panel (un token filtrado, por
//     ejemplo). En ese arranque, cada valor que no es cero se guarda si la
//     clave no existe; el token va cifrado (settings:cloudflare_token). Si
//     guardó algo, deja un evento settings.init con las claves (sin valores),
//     con actor system.
//
// Los valores ya vienen validados (settings.Load, MUELLE_INIT_*).
func (u *ApplyStartupSettings) Execute(ctx context.Context, in dto.StartupSettings) error {
	return u.tx.InTx(ctx, func(ctx context.Context) error {
		if err := u.settings.Set(ctx, shared.SettingPanelDomain, []byte(in.PanelDomain), false); err != nil {
			return err
		}
		// La marca se guarda primero: si ya estaba, este no es el primer arranque.
		err := u.settings.Insert(ctx, entity.KeyInitApplied, []byte(shared.FormatBool(true)), false)
		switch {
		case errors.Is(err, sharedrepo.ErrConflict):
			return nil
		case err != nil:
			return err
		}
		i := in.Init
		plain := map[string]string{
			shared.SettingPublicIP:   i.PublicIP,
			shared.SettingPublicIPv6: i.PublicIPv6,
			shared.SettingACMEEmail:  i.ACMEEmail,
			shared.SettingCaddyImage: i.CaddyImage,
		}
		if i.ProxyHTTPPort != 0 {
			plain[shared.SettingProxyHTTPPort] = strconv.Itoa(i.ProxyHTTPPort)
		}
		if i.ProxyHTTPSPort != 0 {
			plain[shared.SettingProxyHTTPSPort] = strconv.Itoa(i.ProxyHTTPSPort)
		}
		var stored []string
		insert := func(key string, value []byte, encrypted bool) error {
			err := u.settings.Insert(ctx, key, value, encrypted)
			switch {
			case errors.Is(err, sharedrepo.ErrConflict):
				return nil
			case err != nil:
				return err
			}
			stored = append(stored, key)
			return nil
		}
		for k, v := range plain {
			if v == "" {
				continue
			}
			if err := insert(k, []byte(v), false); err != nil {
				return err
			}
		}
		if i.CloudflareToken != "" {
			ct := u.cipher.Encrypt([]byte(i.CloudflareToken), shared.SettingCloudflareTokenAAD)
			if err := insert(shared.SettingCloudflareToken, ct, true); err != nil {
				return err
			}
		}
		if len(stored) > 0 {
			slices.Sort(stored)
			u.audit.Record(ctx, "settings.init", "settings", map[string][]string{"keys": stored})
		}
		return nil
	})
}
