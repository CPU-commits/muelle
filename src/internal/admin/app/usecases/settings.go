// Package usecases tiene los casos de uso del módulo admin.
package usecases

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strconv"

	"github.com/CPU-commits/muelle/src/internal/admin/app/dto"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/cipher"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// get lee una clave; ok false si no está.
func get(ctx context.Context, repo repository.SettingsRepository, key string) (value []byte, encrypted, ok bool, err error) {
	v, enc, err := repo.Get(ctx, key)
	switch {
	case errors.Is(err, sharedrepo.ErrNotFound):
		return nil, false, false, nil
	case err != nil:
		return nil, false, false, err
	}
	return v, enc, true, nil
}

// loadSettings lee los ajustes; lo que no se guardó tiene su valor por
// defecto. El token se descifra con su AAD (settings:cloudflare_token).
func loadSettings(ctx context.Context, repo repository.SettingsRepository, c cipher.Cipher) (entity.Settings, error) {
	var s entity.Settings
	v, _, ok, err := get(ctx, repo, shared.SettingRequire2FA)
	if err != nil {
		return s, err
	}
	s.Require2FA = ok && shared.ParseBool(string(v))

	raw := map[string]string{}
	for _, k := range shared.ProxySettingKeys {
		v, _, ok, err := get(ctx, repo, k)
		if err != nil {
			return s, err
		}
		if ok {
			raw[k] = string(v)
		}
	}
	var token string
	v, enc, ok, err := get(ctx, repo, shared.SettingCloudflareToken)
	if err != nil {
		return s, err
	}
	if ok {
		if !enc {
			return s, fmt.Errorf("settings: %s no está cifrado", shared.SettingCloudflareToken)
		}
		pt, err := c.Decrypt(v, shared.SettingCloudflareTokenAAD)
		if err != nil {
			return s, fmt.Errorf("settings: descifrando %s: %w", shared.SettingCloudflareToken, err)
		}
		token = string(pt)
	}
	if s.Proxy, err = shared.DecodeProxySettings(raw, token); err != nil {
		return s, err
	}
	v, _, ok, err = get(ctx, repo, shared.SettingCloudflareZones)
	if err != nil {
		return s, err
	}
	if ok {
		if n, err := strconv.Atoi(string(v)); err == nil && n >= 0 {
			s.CloudflareZones = &n
		}
	}
	return s, nil
}

func output(s entity.Settings, clientIP string) dto.SettingsOutput {
	p := s.Proxy
	cidrs := p.PanelAllowedCIDRs
	if cidrs == nil {
		cidrs = []string{}
	}
	return dto.SettingsOutput{
		Require2FA:         s.Require2FA,
		CloudflareTokenSet: p.CloudflareToken != "",
		CloudflareZones:    s.CloudflareZones,
		PublicIP:           p.PublicIP,
		PublicIPv6:         p.PublicIPv6,
		ACMEEmail:          p.ACMEEmail,
		PanelDomain:        p.PanelDomain,
		PanelAllowedCIDRs:  cidrs,
		ProxyHTTPPort:      p.HTTPPort,
		ProxyHTTPSPort:     p.HTTPSPort,
		ClientIP:           clientIP,
	}
}

type GetSettings struct {
	settings repository.SettingsRepository
	cipher   cipher.Cipher
}

func NewGetSettings(settings repository.SettingsRepository, c cipher.Cipher) *GetSettings {
	return &GetSettings{settings: settings, cipher: c}
}

func (u *GetSettings) Execute(ctx context.Context, in dto.GetSettingsInput) (dto.SettingsOutput, error) {
	s, err := loadSettings(ctx, u.settings, u.cipher)
	if err != nil {
		return dto.SettingsOutput{}, err
	}
	return output(s, in.ClientIP), nil
}

type PatchSettings struct {
	settings repository.SettingsRepository
	totp     service.TOTPStatus
	cipher   cipher.Cipher
	hook     service.ProxyHook
	origin   *GetPanelOrigin
	audit    audit.Recorder
	tx       sharedrepo.Transactor
}

// NewPatchSettings: hook puede ser nil (los ajustes del proxy solo se
// guardan). origin, si no es nil, es el origen del panel en caché que se
// invalida cuando cambia algún ajuste del proxy.
func NewPatchSettings(settings repository.SettingsRepository, totp service.TOTPStatus, c cipher.Cipher, hook service.ProxyHook, origin *GetPanelOrigin, rec audit.Recorder, tx sharedrepo.Transactor) *PatchSettings {
	return &PatchSettings{settings: settings, totp: totp, cipher: c, hook: hook, origin: origin, audit: rec, tx: tx}
}

// tokenRe es un token de API de Cloudflare: ASCII visible, sin espacios.
var tokenRe = regexp.MustCompile(`^[\x21-\x7e]{20,512}$`)

// applyProxy valida los campos presentes y los aplica sobre p. Los errores
// nombran el campo simple del cuerpo (nunca panel_allowed_cidrs[1]).
func applyProxy(p shared.ProxySettings, in dto.PatchSettingsInput) (shared.ProxySettings, error) {
	if in.PublicIP != nil {
		if *in.PublicIP != "" {
			if err := validate.IPv4("public_ip", *in.PublicIP); err != nil {
				return p, err
			}
		}
		p.PublicIP = *in.PublicIP
	}
	if in.PublicIPv6 != nil {
		if *in.PublicIPv6 != "" {
			if err := validate.IPv6("public_ipv6", *in.PublicIPv6); err != nil {
				return p, err
			}
		}
		p.PublicIPv6 = *in.PublicIPv6
	}
	if in.ACMEEmail != nil {
		if *in.ACMEEmail != "" {
			if err := validate.Email("acme_email", *in.ACMEEmail); err != nil {
				return p, err
			}
		}
		p.ACMEEmail = *in.ACMEEmail
	}
	if in.ProxyHTTPPort != nil {
		if err := validate.Port("proxy_http_port", *in.ProxyHTTPPort); err != nil {
			return p, err
		}
		p.HTTPPort = *in.ProxyHTTPPort
	}
	if in.ProxyHTTPSPort != nil {
		if err := validate.Port("proxy_https_port", *in.ProxyHTTPSPort); err != nil {
			return p, err
		}
		p.HTTPSPort = *in.ProxyHTTPSPort
	}
	if p.HTTPPort == p.HTTPSPort {
		return p, validate.Errorf("proxy_https_port", "no puede ser el mismo que proxy_http_port (%d)", p.HTTPPort)
	}
	if in.PanelAllowedCIDRs != nil {
		list, err := validate.CIDRs("panel_allowed_cidrs", *in.PanelAllowedCIDRs)
		if err != nil {
			return p, err
		}
		p.PanelAllowedCIDRs = list
	}
	if in.CloudflareToken != nil {
		if t := *in.CloudflareToken; t != "" && !tokenRe.MatchString(t) {
			return p, validate.Errorf("cloudflare_token", "no parece un token de API de Cloudflare (de 20 a 512 caracteres visibles, sin espacios)")
		}
		p.CloudflareToken = *in.CloudflareToken
	}
	return p, nil
}

// locksOut dice si la lista deja fuera a la IP del cliente. Una lista vacía
// es cualquier IP; una IP que no se entiende queda fuera.
func locksOut(cidrs []string, clientIP string) bool {
	if len(cidrs) == 0 {
		return false
	}
	ip, err := netip.ParseAddr(clientIP)
	if err != nil {
		return true
	}
	ip = ip.Unmap()
	for _, c := range cidrs {
		if p, err := netip.ParsePrefix(c); err == nil && p.Contains(ip) {
			return false
		}
	}
	return true
}

// Execute aplica los campos presentes y devuelve los ajustes resultantes.
//   - Activar require_2fa exige que quien lo pide tenga su propio TOTP
//     (§10.1): sin esa guarda quedaría encerrado en la pantalla de configuración.
//   - Una lista de panel_allowed_cidrs que deja fuera la IP de quien la
//     guarda devuelve service.ErrWouldLockOut, salvo con ConfirmLockout.
//   - Un token de Cloudflare nuevo se verifica con el hook antes de guardar
//     nada (cuántas zonas ve); "" lo borra.
//   - Todo se guarda en una transacción, y después, si cambió algún ajuste
//     del proxy, se invalida el origen del panel en caché (el Origin del
//     puerto nuevo pasa enseguida, y el del viejo ya no) y se avisa al hook
//     (ingress los aplica) antes de responder.
func (u *PatchSettings) Execute(ctx context.Context, in dto.PatchSettingsInput) (dto.SettingsOutput, error) {
	p, ok := shared.PrincipalFrom(ctx)
	if !ok {
		return dto.SettingsOutput{}, shared.ErrUnauthenticated
	}
	// Los ajustes son de la sesión de un usuario, nunca de un token de API
	// (aunque el patch venga vacío).
	if p.Token != nil {
		return dto.SettingsOutput{}, shared.ErrForbidden
	}
	old, err := loadSettings(ctx, u.settings, u.cipher)
	if err != nil {
		return dto.SettingsOutput{}, err
	}
	next, err := applyProxy(old.Proxy, in)
	if err != nil {
		return dto.SettingsOutput{}, err
	}
	if in.PanelAllowedCIDRs != nil && !in.ConfirmLockout && locksOut(next.PanelAllowedCIDRs, in.ClientIP) {
		return dto.SettingsOutput{}, service.ErrWouldLockOut
	}
	changed := next.ChangedKeys(old.Proxy)
	tokenChanged := next.CloudflareToken != old.Proxy.CloudflareToken
	var zones *int
	if tokenChanged && next.CloudflareToken != "" && u.hook != nil {
		// Fuera de la transacción: es una llamada a Cloudflare.
		if zones, err = u.hook.CheckCloudflareToken(ctx, next.CloudflareToken); err != nil {
			return dto.SettingsOutput{}, err
		}
	}

	details := map[string]any{}
	err = u.tx.InTx(ctx, func(ctx context.Context) error {
		if in.Require2FA != nil {
			if *in.Require2FA {
				has, err := u.totp.HasTOTP(ctx, p.UserID)
				if err != nil {
					return err
				}
				if !has {
					return service.ErrTOTPRequiredFirst
				}
			}
			if err := u.settings.Set(ctx, shared.SettingRequire2FA, []byte(shared.FormatBool(*in.Require2FA)), false); err != nil {
				return err
			}
			details[shared.SettingRequire2FA] = *in.Require2FA
		}
		values := next.Encode()
		for _, k := range changed {
			if k == shared.SettingCloudflareToken {
				continue
			}
			if err := u.settings.Set(ctx, k, []byte(values[k]), false); err != nil {
				return err
			}
			details[k] = auditValue(next, k)
		}
		if tokenChanged {
			if err := u.saveToken(ctx, next.CloudflareToken, zones); err != nil {
				return err
			}
			details[shared.SettingCloudflareToken] = "set"
			if next.CloudflareToken == "" {
				details[shared.SettingCloudflareToken] = "removed"
			}
		}
		if len(details) > 0 {
			u.audit.Record(ctx, "settings.update", "settings", details)
		}
		return nil
	})
	if err != nil {
		return dto.SettingsOutput{}, err
	}
	if len(changed) > 0 && u.origin != nil {
		u.origin.Invalidate()
	}
	if len(changed) > 0 && u.hook != nil {
		u.hook.ProxySettingsChanged(ctx, old.Proxy, next)
	}
	return NewGetSettings(u.settings, u.cipher).Execute(ctx, dto.GetSettingsInput{ClientIP: in.ClientIP})
}

// saveToken guarda el token cifrado y sus zonas, o borra los dos con "".
func (u *PatchSettings) saveToken(ctx context.Context, token string, zones *int) error {
	if token == "" {
		if err := u.settings.Delete(ctx, shared.SettingCloudflareToken); err != nil {
			return err
		}
		return u.settings.Delete(ctx, shared.SettingCloudflareZones)
	}
	ct := u.cipher.Encrypt([]byte(token), shared.SettingCloudflareTokenAAD)
	if err := u.settings.Set(ctx, shared.SettingCloudflareToken, ct, true); err != nil {
		return err
	}
	if zones == nil {
		return u.settings.Delete(ctx, shared.SettingCloudflareZones)
	}
	return u.settings.Set(ctx, shared.SettingCloudflareZones, []byte(strconv.Itoa(*zones)), false)
}

// auditValue es el valor de una clave para los details de la auditoría, con
// su tipo (los puertos como número, la lista como lista).
func auditValue(p shared.ProxySettings, key string) any {
	switch key {
	case shared.SettingProxyHTTPPort:
		return p.HTTPPort
	case shared.SettingProxyHTTPSPort:
		return p.HTTPSPort
	case shared.SettingPanelAllowedCIDRs:
		return p.PanelAllowedCIDRs
	}
	return p.Encode()[key]
}
