// Package service define los errores y los puertos de servicios del módulo admin.
package service

import (
	"context"
	"errors"

	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

var (
	ErrWrongMasterKey = errors.New("clave maestra incorrecta")
	// ErrTOTPRequiredFirst: quien activa require_2fa sin tener su propio TOTP quedaría encerrado (§10.1).
	ErrTOTPRequiredFirst = errors.New("configura tu propio 2FA antes de exigirlo a todos")
	// ErrWouldLockOut: la lista de panel_allowed_cidrs deja fuera la IP de
	// quien la guarda, y Caddy lo bloquearía (§8). Se guarda solo con
	// confirm_lockout: true.
	ErrWouldLockOut = errors.New("la lista de IPs permitidas deja fuera tu IP actual y perderías el acceso al panel; mándala con confirm_lockout: true para guardarla igual")
)

// ProxyHook es lo que admin le avisa al proxy cuando cambian sus ajustes. Lo
// implementa ingress.Module; el contenedor lo conecta. Puede ser nil: entonces
// los ajustes solo se guardan.
type ProxyHook interface {
	// CheckCloudflareToken verifica un token nuevo antes de guardarlo y
	// devuelve cuántas zonas ve (nil: no se sabe). Un token rechazado es un
	// *validate.Error en cloudflare_token (422); si Cloudflare no responde, un
	// error que envuelve entity.ErrCloudflareUnavailable (502). Si falla, no
	// se guarda nada del PATCH.
	CheckCloudflareToken(ctx context.Context, token string) (zones *int, err error)
	// ProxySettingsChanged recibe los ajustes de antes y los nuevos, ya
	// guardados, cuando algo cambió. Corre antes de que el PATCH responda;
	// lo que cortaría la respuesta (recrear muelle-caddy) lo deja para
	// después de volver (contrato §3.5).
	ProxySettingsChanged(ctx context.Context, old, next entity.ProxySettings)
}

// TOTPStatus le pregunta a identity (vía SQL, sin importarlo) si un usuario tiene 2FA.
type TOTPStatus interface {
	HasTOTP(ctx context.Context, userID int64) (bool, error)
}

// BackupStore guarda las copias de la base. Los nombres son solo el nombre
// del archivo y ordenan cronológicamente (muelle-AAAA-MM-DD.db).
type BackupStore interface {
	// Create escribe una copia consistente de la base con ese nombre. Si falla no deja un archivo a medias.
	Create(ctx context.Context, name string) error
	// List devuelve los nombres de las copias, de la más vieja a la más nueva.
	List(ctx context.Context) ([]string, error)
	Remove(ctx context.Context, name string) error
}
