package repository

import (
	"context"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
)

// OriginCerts es la tabla origin_ca_certs (migración 0006): los
// certificados de Cloudflare Origin CA que emitió muelle (contrato §3.7). La
// clave va cifrada con la clave maestra (AAD entity.OriginKeyAAD(id)) y solo
// ActiveWithKeys la descifra; los demás métodos la dejan en "". Los errores
// de "no existe" son repository.ErrNotFound.
type OriginCerts interface {
	// Insert guarda c como el active de su hostname, con la clave cifrada, y
	// retira el active anterior de ese hostname (si hay) en la misma
	// transacción, con retired_at = c.CreatedAt y la clave borrada. Le
	// asigna c.ID.
	Insert(ctx context.Context, c *entity.OriginCert) error
	// InsertRetired guarda c ya retirado en at, sin la clave: la prueba del
	// permiso cuyo DELETE falló (§3.3 del diseño de Origin CA), que se revoca
	// después. Le asigna c.ID.
	InsertRetired(ctx context.Context, c *entity.OriginCert, at time.Time) error
	// Active son los active por hostname, sin la clave.
	Active(ctx context.Context) (map[string]entity.OriginCert, error)
	// ActiveWithKeys son los active por hostname con la clave descifrada:
	// solo para la configuración de Caddy. Un error de descifrado nombra el
	// hostname, nunca la clave.
	ActiveWithKeys(ctx context.Context) (map[string]entity.OriginCert, error)
	// ActiveFor es el active de hostname sin la clave; nil si no hay.
	ActiveFor(ctx context.Context, hostname string) (*entity.OriginCert, error)
	// Retire retira el active id: retired_at = at y la clave borrada.
	// repository.ErrNotFound si no hay un active con ese id.
	Retire(ctx context.Context, id int64, at time.Time) error
	// Retired son todos los retirados, por id.
	Retired(ctx context.Context) ([]entity.OriginCert, error)
	// RetiredDue son los retirados, por id, que vencieron (expires_at < now:
	// se borran sin revocar) o que ya se pueden revocar (§6 del diseño):
	// retirados antes de lastGoodBuild, hace al menos grace, y sin un
	// reintento por delante (next_revoke_at <= now, o sin uno).
	RetiredDue(ctx context.Context, now, lastGoodBuild time.Time, grace time.Duration) ([]entity.OriginCert, error)
	// RevokeFailed registra una revocación que falló: revoke_failures y
	// next_revoke_at.
	RevokeFailed(ctx context.Context, id int64, failures int, next time.Time) error
	// Delete borra la fila (revocado, o vencido).
	Delete(ctx context.Context, id int64) error
}

// OriginHosts es la tabla origin_ca_hosts (migración 0006): lo último que
// vio muelle en Cloudflare de cada hostname y la última emisión que falló,
// con su reintento. Cada método toca solo sus columnas, así la pasada del
// reconciliador y un PATCH de un dominio no se pisan.
type OriginHosts interface {
	// All son todas las filas por hostname.
	All(ctx context.Context) (map[string]entity.OriginHost, error)
	// Get es la fila de hostname; sin fila, una vacía (solo con Hostname).
	Get(ctx context.Context, hostname string) (entity.OriginHost, error)
	// SetObserved guarda lo observado en at (crea la fila si falta).
	SetObserved(ctx context.Context, hostname string, o entity.Observation, at time.Time) error
	// ForgetObserved vuelve observed a NULL (la API dice issuing hasta la
	// próxima lectura) y deja la falla. Sin fila no hace nada.
	ForgetObserved(ctx context.Context, hostname string, at time.Time) error
	// RecordFailure guarda la falla número failures, con su código, su causa
	// y el reintento (crea la fila si falta).
	RecordFailure(ctx context.Context, hostname string, failures int, code entity.OriginErrorCode, cause string, next, at time.Time) error
	// ClearFailure borra la falla y su reintento (una emisión que salió bien).
	ClearFailure(ctx context.Context, hostname string, at time.Time) error
	// ResetBackoff pone next_attempt_at en at en las filas con una falla, de
	// hostname o de todas con "", y deja la falla (§9.2 del diseño).
	ResetBackoff(ctx context.Context, hostname string, at time.Time) error
	// Delete borra la fila de hostname (si hay).
	Delete(ctx context.Context, hostname string) error
	// DeleteAll borra todas las filas (al apagar la opción).
	DeleteAll(ctx context.Context) error
}
