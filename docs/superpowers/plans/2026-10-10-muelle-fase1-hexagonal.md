# muelle — Fase 1 (Base) con arquitectura hexagonal: Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Un binario `muelle serve` con la API REST de la Fase 1 (usuarios, setup, invitaciones, 2FA, sesiones, tokens de API con alcance, CRUD de proyectos, stages y apps, variables cifradas, settings, auditoría y backup), organizado con la arquitectura hexagonal de Extractor.

**Architecture:** Spec §4. Hay cuatro módulos (`identity`, `catalog`, `secrets` y `admin`) sobre una base compartida (`settings`, `package/`, `shared/` y `cmd/http`). Los módulos no se importan entre sí. Lo que uno necesita de otro lo pide con una interfaz en su `domain/service`, y su adaptador SQL lee directo las tablas del otro. Cada módulo tiene un `module.go` que arma sus piezas, y el contenedor solo une módulos.

**Tech Stack:**
- Go 1.27, con `net/http` y `encoding/json/v2` de la librería estándar
- `modernc.org/sqlite` v1.60.1 (sin CGO)
- `golang.org/x/crypto` v0.58.0 (argon2id)
- `github.com/pquerna/otp` v1.5.0
- `github.com/distribution/reference` (validar imágenes)

**Spec:** [`docs/superpowers/specs/2026-10-09-muelle-design.md`](../specs/2026-10-09-muelle-design.md). Las referencias `§N` apuntan a sus secciones.

**Código de referencia:** el plan anterior, [`2026-10-09-muelle-fase1-base.md`](2026-10-09-muelle-fase1-base.md). Su SQL, su lógica y sus tests son correctos y ya pasaron revisión. Aquí se reorganizan en la estructura nueva. Cuando este plan dice "portar de la Tarea N (ref)", se toma el código de esa tarea, se mueve a los archivos que se indican y se aplican los cambios listados. **No se copia la estructura vieja** (`internal/store`, `internal/auth`, `internal/api`).

---

## Etapas y paralelismo

| Etapa | Tareas | Cómo corre |
|---|---|---|
| 1. Base compartida | B1–B6 | Secuencial, en la rama `fase-1-base` |
| 2. Módulos | M-identity, M-catalog, M-secrets, M-admin | **En paralelo**, cada una en su worktree y rama (`fase-1-<módulo>`) creada desde `fase-1-base` al cerrar la Etapa 1 |
| 3. Integración | I1–I4 | Secuencial, en `fase-1-base`, después de mergear las cuatro ramas de módulo |

**Por qué las cuatro tareas de módulo no chocan:** cada una solo crea archivos dentro de `src/internal/<módulo>/`, con dos excepciones:
- `go.mod` y `go.sum`, si agrega dependencias;
- M-catalog, que además borra `internal/appspec/`.

Ninguna toca `shared/`, `package/`, `settings/`, `cmd/` ni las migraciones. Si una tarea de módulo necesita cambiar algo compartido, se detiene y lo reporta (status `NEEDS_CONTEXT`); no lo cambia.

## Convenciones (todas las tareas)

- **Repo:** `/home/felipe/Escritorio/dev/muelle`. Todos los comandos se ejecutan desde la raíz del repo o de su worktree.
- **Go:** 1.27. Si la máquina todavía tiene 1.26, `GOTOOLCHAIN=auto` descarga el toolchain solo. Sin CGO.
- **Idioma:** comentarios, mensajes de error y commits en español; identificadores en inglés.
- **Errores:**
  - Los comunes están en `shared/domain/repository` (`ErrNotFound`, `ErrConflict`, `ErrNotEmpty`) y `shared/domain/entity` (`ErrForbidden`, `ErrUnauthenticated`).
  - Las reglas de validación devuelven `*validate.Error`.
  - Cada módulo define los suyos en `domain/service/errors.go` (o `domain/entity`) y los registra en el `Kit` desde `infra/in/http/mapper_errors.go`.
  - Formato HTTP: `{"error": {"code", "message"}}`.
- **Tiempo:** SQLite guarda milisegundos Unix (`sqlite.Millis` / `sqlite.Time`). El reloj es `package/clock`, y los tests usan `clock.NewManual`.
- **Tests:**
  - van en el mismo paquete que el código;
  - SQLite real con `sqlitetest.New(t)`, sin mocks de la base;
  - los fakes solo se permiten para puertos hacia otro módulo o hacia HTTP (`httptestutil.FakeAuth`);
  - se corren con `go test -race`.
- **TDD:** test → verificar que falla → implementar → verificar que pasa → commit.
- **Commits:** cada commit termina con `-m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"`. Escribe los acentos reales en los mensajes (`ó`), no escapes.
- **Verificación antes de cada commit:** `gofmt -l .` no imprime nada, `go vet ./...` pasa y `go test -race ./...` pasa.
- **Alcance:** no modifiques archivos que la tarea no lista.

## Mapa de archivos

```
main.go                                   (I2) subcomandos serve y genkey
src/
├── settings/settings.go                  (B1) MUELLE_*  ← internal/config
├── package/
│   ├── cipher/cipher.go                  (B2) interfaz Cipher + ErrDecrypt
│   ├── clock/clock.go                    (B2) Clock, System, Manual
│   └── audit/audit.go                    (B2) interfaz Recorder
├── shared/
│   ├── domain/
│   │   ├── entity/principal.go           (B2) Principal, TokenScope, Actor, Allows, ctx
│   │   ├── entity/errors.go              (B2) ErrForbidden, ErrUnauthenticated
│   │   ├── repository/errors.go          (B2) ErrNotFound, ErrConflict, ErrNotEmpty
│   │   ├── repository/transactor.go      (B2) Transactor
│   │   └── validate/validate.go          (B1) ← internal/validate (+ arreglos)
│   └── infra/
│       ├── container/container.go        (I1)
│       └── out/
│           ├── crypto/crypto.go          (B1) ← internal/crypto (+ arreglos)
│           └── sqlite/
│               ├── sqlite.go             (B3) Open, Migrate, InTx, Conn, helpers
│               ├── migrations/0001_init.sql (B3)
│               ├── audit.go              (B3) AuditRecorder (implementa audit.Recorder)
│               └── sqlitetest/sqlitetest.go (B3)
├── cmd/
│   ├── http/
│   │   ├── server.go                     (B4) Server: Origin, headers, errores comunes
│   │   ├── utils/{kit,respond,decode,request}.go (B4) Kit, Access, Authenticator, JSON
│   │   └── httptestutil/httptestutil.go  (B5) cliente de prueba + FakeAuth
│   └── cron/cron.go                      (B6) Job y Run
└── internal/
    ├── identity/   (M-identity)
    ├── catalog/    (M-catalog)  incluye domain/vo/appspec.go ← internal/appspec
    ├── secrets/    (M-secrets)
    └── admin/      (M-admin)
```

Cada módulo usa esta estructura (se crean solo las carpetas que hagan falta):

```
src/internal/<m>/
├── module.go                     Deps, New, RegisterRoutes(k), Jobs() …
├── domain/{entity,vo,repository,service}/
├── app/{dto,usecases}/
└── infra/
    ├── in/{http,form,cron}/
    └── out/{repository/sqlite,service}/
```

---

# Etapa 1 — Base compartida (secuencial)

### Task B1: Go 1.27 y mudanza de lo ya hecho (settings, validate, crypto)

Mueve con `git mv` el código de las Tareas 2-4 a su nuevo lugar, sube el módulo a Go 1.27 y aplica los arreglos de las revisiones. `internal/appspec` **no se mueve aquí** (lo hace M-catalog); en esta tarea solo se actualiza su import de `validate`.

**Files:**
- Modify: `go.mod` (`go 1.27`)
- Move: `internal/config/` → `src/settings/` (paquete `settings`, tipo `Settings`, función `Load`)
- Move: `internal/validate/` → `src/shared/domain/validate/`
- Move: `internal/crypto/` → `src/shared/infra/out/crypto/`
- Modify: `internal/appspec/appspec.go` (solo el import de validate)
- Create: `src/package/cipher/cipher.go` (antes que crypto, porque crypto lo usa; mismo contenido que en B2)

- [ ] **Step 1: Go 1.27**

```bash
go mod edit -go=1.27
go version
```

Expected: `go.mod` dice `go 1.27`. `go version` muestra 1.27.x, o 1.26.x con un toolchain 1.27 descargado. Si muestra 1.26.x, cualquier `go test` posterior descarga 1.27 automáticamente.

- [ ] **Step 2: Mover los paquetes**

```bash
mkdir -p src/settings src/shared/domain src/shared/infra/out
git mv internal/config/config.go src/settings/settings.go
git mv internal/config/config_test.go src/settings/settings_test.go
git mv internal/validate src/shared/domain/validate
git mv internal/crypto src/shared/infra/out/crypto
```

En `src/settings/*.go`:
- `package config` pasa a `package settings`;
- el tipo `Config` pasa a `Settings`;
- el comentario del paquete pasa a decir "Package settings lee la configuración…".

En `internal/appspec/appspec.go`, el import pasa a `github.com/CPU-commits/muelle/src/shared/domain/validate`.

- [ ] **Step 3: Crear `src/package/cipher/cipher.go`** con el código de B2 Step 1, y hacer que crypto lo use:
  - `crypto.ErrDecrypt` se elimina y se usa `cipher.ErrDecrypt`;
  - se agrega `var _ cipher.Cipher = (*Cipher)(nil)` en `crypto.go`.

- [ ] **Step 4: Arreglos de `settings`** (revisión de la Tarea 2):

1. **IP literales no canónicas** (`validateOrigin`):
   - En la rama IPv6, reemplazar `net.ParseIP` por `netip.ParseAddr`. Rechazar zona (`a.Zone() != ""`) e IPv4 mapeada (`a.Is4In6()`), y usar `host = "[" + a.String() + "]"` como forma canónica.
   - Después del bucle de caracteres, si la última etiqueta del host es numérica o empieza con `0x`, exigir que `netip.ParseAddr(host)` funcione y sea `Is4()`. Si no, devolver el error "una IP v4 debe ir en decimal con puntos, sin ceros a la izquierda (ej. 127.0.0.1)".
   - Código probado por el revisor:
     ```go
     a, err := netip.ParseAddr(host)
     if err != nil || a.Zone() != "" || a.Is4In6() {
         return fmt.Errorf("la dirección IPv6 no es válida o no está soportada")
     }
     host = "[" + a.String() + "]"
     // …después del bucle de caracteres:
     labels := strings.Split(strings.TrimSuffix(host, "."), ".")
     if last := labels[len(labels)-1]; strings.HasPrefix(last, "0x") || strings.Trim(last, "0123456789") == "" {
         if a, err := netip.ParseAddr(host); err != nil || !a.Is4() {
             return fmt.Errorf("una IP v4 debe ir en decimal con puntos, sin ceros a la izquierda (ej. 127.0.0.1)")
         }
     }
     ```
2. En el test del origen inválido, cambiar `strings.Contains(err.Error(), "MUELLE_PANEL_ORIGIN")` por `strings.HasPrefix(err.Error(), "MUELLE_PANEL_ORIGIN:")`.
3. Usar `errors.New` en vez de `fmt.Errorf` en los mensajes sin verbos.
4. Agregar a los tests de rechazo:
   - `https://[0:0:0:0:0:0:0:1]` (sugiere `https://[::1]`)
   - `https://[2001:0db8::1]` (sugiere `https://[2001:db8::1]`)
   - `https://[::ffff:127.0.0.1]`
   - `http://127.1:8080`, `http://0x7f.0.0.1`, `http://0177.0.0.1`, `http://2130706433`, `http://256.0.0.1`

   Y a los de aceptación: `https://[2001:db8::1]`.

- [ ] **Step 5: Arreglos de `validate`** (revisión de la Tarea 4):

1. **Slugs de proyecto reservados:** `ProjectSlug` rechaza `muelle` y `docker` con el mensaje `"%q está reservado"`. Agregar un test.
2. **Prefijo `COMPOSE_` reservado:** `EnvKey` lo rechaza (spec §5.2 regla 3). Agregar un test.
3. **`EnvKey` recibe el campo:** la firma pasa a `EnvKey(field, key string) error`, para que el error de PATCH diga `set` o `unset`. Actualizar los tests.
4. **`Name` sin caracteres invisibles:** rechaza runas de control y de formato (`unicode.IsControl(r) || unicode.Is(unicode.Cf, r)`). Tests con `"\x00"`, `"​"`, `"a\nb"` y `"‮abc"`.
5. **Tests que faltan:**
   - `VolumeName` (válidos e inválidos);
   - contraseña con runas: `strings.Repeat("ñ", 11)` falla y `strings.Repeat("ñ", 12)` pasa;
   - bordes: contraseña de 256 bytes pasa y de 257 falla, EnvKey de 128 pasa y de 129 falla, Username de 32 pasa y de 33 falla;
   - la lista de slugs malos también contra `AppSlug` y `StageSlug`;
   - un test que arma el servicio Swarm más largo válido (`proyecto_stage_app_servicio` con largos máximos) y verifica que tiene ≤ 63 caracteres.

- [ ] **Step 6: Arreglos de `crypto`** (revisión de la Tarea 3):

1. **AAD vacía:** `Encrypt` hace `panic("crypto: AAD vacía")`, porque es un error de programación, y `Decrypt` devuelve `cipher.ErrDecrypt`. Test de los dos casos (el panic con `recover`).
2. **Doc del paquete:** documentar que va una AAD única por fila y columna, con formato `<tabla>:<dueño>:<clave>`, y que solo la última parte puede contener `:`.
3. **Comentario de `LoadKeyFile`:** corregirlo. Se toleran saltos de línea en cualquier lado y espacios solo en los extremos.
4. **Tests que faltan:**
   - base64 inválido en `LoadKeyFile`;
   - descifrar con otra clave devuelve `ErrDecrypt`;
   - ida y vuelta de un plaintext vacío;
   - revisar el error de `os.WriteFile` en `TestLoadKeyFile`.

- [ ] **Step 7: Verificar y commit**

```bash
gofmt -l . ; go vet ./... && go test -race ./...
git add -A go.mod src internal
git commit -m "refactor: estructura hexagonal (settings, validate, crypto) y Go 1.27" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

Expected: `gofmt` no imprime nada; todos los paquetes `ok`. `internal/` queda solo con `appspec/`.

### Task B2: Contratos compartidos (`package/` y `shared/domain`)

Estos tipos los usan los cuatro módulos en paralelo, así que **su forma es fija**. Si algo no compila con este código exacto, repórtalo; no cambies las firmas.

**Files:**
- Create: `src/package/cipher/cipher.go` (ya creado en B1; verificar que coincide)
- Create: `src/package/clock/clock.go`, `src/package/clock/clock_test.go`
- Create: `src/package/audit/audit.go`
- Create: `src/shared/domain/entity/principal.go`, `src/shared/domain/entity/principal_test.go`, `src/shared/domain/entity/errors.go`
- Create: `src/shared/domain/repository/errors.go`, `src/shared/domain/repository/transactor.go`

- [ ] **Step 1: `src/package/cipher/cipher.go`**

```go
// Package cipher define cómo los módulos cifran secretos sin depender de la implementación.
package cipher

import "errors"

// ErrDecrypt indica un valor alterado, otra clave maestra o una AAD distinta.
var ErrDecrypt = errors.New("cipher: no se pudo descifrar")

// Cipher cifra valores ligándolos a su fila con aad (<tabla>:<dueño>:<clave>).
// aad nunca puede ser vacía: Encrypt hace panic y Decrypt devuelve ErrDecrypt.
type Cipher interface {
	Encrypt(plaintext []byte, aad string) []byte
	Decrypt(ciphertext []byte, aad string) ([]byte, error)
}
```

- [ ] **Step 2: `src/package/clock/clock.go`**

```go
// Package clock abstrae el reloj para que los tests controlen el tiempo.
package clock

import (
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
}

// System es el reloj real.
type System struct{}

func (System) Now() time.Time { return time.Now() }

// Manual es un reloj controlado para tests. Es seguro usarlo desde varias goroutines.
type Manual struct {
	mu  sync.Mutex
	now time.Time
}

func NewManual(t time.Time) *Manual { return &Manual{now: t} }

func (m *Manual) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now
}

func (m *Manual) Set(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = t
}

func (m *Manual) Advance(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = m.now.Add(d)
}
```

`clock_test.go`: `NewManual(t0).Now()` es `t0`; después de `Advance(time.Hour)` es `t0+1h`; después de `Set(t1)` es `t1`.

- [ ] **Step 3: `src/package/audit/audit.go`**

```go
// Package audit define el registro de auditoría que usan todos los módulos.
package audit

import "context"

// Recorder guarda un evento de auditoría. El actor sale del Principal del ctx
// (shared/domain/entity.PrincipalFrom); sin Principal, el actor es "system".
// details se guarda como JSON y NUNCA debe incluir valores secretos.
// Un fallo al guardar se registra en el log y no interrumpe la operación.
type Recorder interface {
	Record(ctx context.Context, action, target string, details any)
}
```

- [ ] **Step 4: `src/shared/domain/entity/errors.go`**

```go
package entity

import "errors"

var (
	ErrUnauthenticated = errors.New("inicia sesión")
	ErrForbidden       = errors.New("no tienes permiso para esta operación")
)
```

- [ ] **Step 5: `src/shared/domain/entity/principal.go`**

```go
// Package entity tiene las entidades que comparten todos los módulos.
package entity

import "context"

type ActorType string

const (
	ActorUser   ActorType = "user"
	ActorToken  ActorType = "token"
	ActorSystem ActorType = "system"
)

// TokenScope es el alcance de un token de API (§10.1 del spec).
type TokenScope struct {
	ID          int64
	Name        string
	ProjectSlug string
	StageSlug   string // vacío: todo el proyecto
}

// Principal es quien hace la petición: una sesión de usuario o un token de API.
// Lo produce identity y lo consultan los demás módulos.
type Principal struct {
	UserID         int64
	Username       string
	SessionHash    string
	MFAPending     bool // contraseña correcta, falta el código TOTP
	NeedsTOTPSetup bool // require_2fa activo y el usuario no tiene TOTP
	Token          *TokenScope
}

func (p Principal) IsToken() bool { return p.Token != nil }

// Actor identifica al autor de un evento de auditoría.
func (p Principal) Actor() (ActorType, *int64) {
	if p.Token != nil {
		id := p.Token.ID
		return ActorToken, &id
	}
	if p.UserID == 0 {
		return ActorSystem, nil
	}
	id := p.UserID
	return ActorUser, &id
}

// Allows dice si el principal puede operar sobre project/stage.
// Una sesión puede todo. Un token tiene que ser de ese proyecto; si está
// limitado a un stage, solo puede tocar ese stage, y sobre recursos del
// proyecto (stage vacío) solo puede leer.
func (p Principal) Allows(project, stage string, write bool) bool {
	if p.Token == nil {
		return true
	}
	if project != p.Token.ProjectSlug {
		return false
	}
	if p.Token.StageSlug == "" {
		return true
	}
	if stage == "" {
		return !write
	}
	return stage == p.Token.StageSlug
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}
```

`principal_test.go`, tabla de `Allows` con estos casos:

| Principal | project, stage, write | Esperado |
|---|---|---|
| sesión | cualquiera | `true` |
| token de proyecto `auditfy` | `auditfy, "", false` | `true` |
| token de proyecto `auditfy` | `auditfy, staging, true` | `true` |
| token de proyecto `auditfy` | `otro, "", false` | `false` |
| token `auditfy/staging` | `auditfy, staging, true` | `true` |
| token `auditfy/staging` | `auditfy, production, false` | `false` |
| token `auditfy/staging` | `auditfy, "", false` | `true` |
| token `auditfy/staging` | `auditfy, "", true` | `false` |

También probar `Actor()` para token, usuario y sistema, y la ida y vuelta de `WithPrincipal` / `PrincipalFrom`.

- [ ] **Step 6: `src/shared/domain/repository/errors.go` y `transactor.go`**

```go
// Package repository tiene los errores y contratos comunes a todos los repositorios.
package repository

import "errors"

var (
	ErrNotFound = errors.New("no existe")
	ErrConflict = errors.New("ya existe")
	ErrNotEmpty = errors.New("primero hay que borrar lo que contiene")
)
```

```go
package repository

import "context"

// Transactor ejecuta fn en una transacción. Los repositorios que reciben el
// ctx de fn participan en ella. Si fn devuelve error se hace rollback.
// Llamar a InTx dentro de fn reutiliza la transacción en curso.
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}
```

- [ ] **Step 7: Verificar y commit**

```bash
gofmt -l . ; go vet ./... && go test -race ./...
git add src/package src/shared/domain/entity src/shared/domain/repository
git commit -m "feat(shared): contratos compartidos: Principal, errores, Transactor, cipher, clock y audit" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task B3: SQLite compartido (conexión, migración completa, Transactor, auditoría)

Portar de la Tarea 6 (ref) `Open`, el DSN, `Migrate` y los helpers de tiempo y de errores de restricción, con estos cambios:
- el tipo pasa a ser `sqlite.DB` en `src/shared/infra/out/sqlite`;
- los errores pasan a ser los de `shared/domain/repository`;
- se agregan `InTx` y `Conn`;
- se quita el reloj del store (lo usan los módulos con `package/clock`).

**Files:**
- Create: `src/shared/infra/out/sqlite/sqlite.go`, `sqlite_test.go`
- Create: `src/shared/infra/out/sqlite/migrations/0001_init.sql`
- Create: `src/shared/infra/out/sqlite/audit.go`, `audit_test.go`
- Create: `src/shared/infra/out/sqlite/sqlitetest/sqlitetest.go`
- Modify: `go.mod`, `go.sum` (`modernc.org/sqlite v1.60.1`)

**API pública (fija; los módulos la usan):**

```go
package sqlite

// Executor es lo común entre *sql.DB y *sql.Tx.
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type DB struct{ /* *sql.DB */ }

func Open(path string) (*DB, error)                // DSN de la Tarea 6 (ref): WAL, foreign_keys, busy_timeout, _txlock=immediate
func (d *DB) Close() error
func (d *DB) Migrate(ctx context.Context) error    // runner de la Tarea 6 (ref)
func (d *DB) InTx(ctx context.Context, fn func(ctx context.Context) error) error // implementa repository.Transactor
func (d *DB) Conn(ctx context.Context) Executor    // la *sql.Tx del ctx si hay una; si no, la *sql.DB

func Millis(t time.Time) int64                     // t.UnixMilli()
func Time(ms int64) time.Time                      // time.UnixMilli(ms).UTC()
func NullMillis(t *time.Time) sql.NullInt64
func NullTime(v sql.NullInt64) *time.Time
func IsUnique(err error) bool                      // SQLITE_CONSTRAINT_UNIQUE o PRIMARYKEY
func IsForeignKey(err error) bool                  // SQLITE_CONSTRAINT_FOREIGNKEY

// AuditRecorder implementa audit.Recorder sobre la tabla audit_events.
func NewAuditRecorder(db *DB, c clock.Clock, log *slog.Logger) *AuditRecorder
func (r *AuditRecorder) Record(ctx context.Context, action, target string, details any)
```

```go
package sqlitetest

// New abre una base nueva en t.TempDir(), corre las migraciones y la cierra al terminar el test.
func New(t testing.TB) *sqlite.DB
```

- [ ] **Step 1: Migración.** `migrations/0001_init.sql` es la de la Tarea 6 (ref) con un solo cambio: `stages`, `apps` y `users` usan `id INTEGER PRIMARY KEY AUTOINCREMENT` (§12: ids no reutilizables). El resto queda igual: `projects`, `env_vars`, `invites`, `sessions`, `api_tokens`, `audit_events`, `settings` e índices.

- [ ] **Step 2: Tests de `sqlite_test.go`** (escribir primero):
  - Portar de la Tarea 6 (ref) los tests de `Open` y `Migrate`: aplica la migración, es idempotente y deja `foreign_keys` activo.
  - `TestInTxCommitAndRollback`:
    - una fila insertada con `d.Conn(ctx)` dentro de `InTx` que devuelve `nil` queda;
    - una insertada dentro de `InTx` que devuelve un error no queda, y `InTx` devuelve ese error.
  - `TestInTxNested`: un `InTx` dentro de otro usa la misma transacción. El rollback del externo deshace lo del interno.
  - `TestConnOutsideTx`: `Conn(context.Background())` funciona sin transacción.
  - `TestAutoincrementDoesNotReuseIDs`: insertar dos `users`, borrar el de id mayor e insertar otro; el nuevo id es mayor que ambos.
  - `TestIsUniqueAndForeignKey`: un `projects.slug` duplicado da `IsUnique`, y un `stages.project_id` inexistente da `IsForeignKey`.

- [ ] **Step 3: Implementar `sqlite.go`.** `InTx` guarda la `*sql.Tx` en el ctx con una clave privada. Si el ctx ya trae una, ejecuta `fn(ctx)` sin abrir otra. Si no, hace `BeginTx`, ejecuta `fn`, y hace `Commit` si `fn` devuelve `nil` o `Rollback` si devuelve error (también si hay panic, re-lanzándolo).

- [ ] **Step 4: Tests de `audit_test.go`:**
  - Sin Principal en el ctx, la fila guarda `actor_type='system'` y `actor_id NULL`.
  - Con un Principal de usuario guarda `user` y su id; con uno de token, `token` y el id del token.
  - `details` se guarda como JSON (`{"keys":["A"]}`); con `nil` se guarda `{}`.
  - `at` es el `clock.Manual` en milisegundos.
  - Dentro de un `InTx` que hace rollback, el evento tampoco queda, porque usa `Conn(ctx)`.

- [ ] **Step 5: Implementar `audit.go`** (`INSERT INTO audit_events (actor_type, actor_id, action, target, details, at)`). Si falla, `log.Error("auditoría", "action", action, "err", err)`.

- [ ] **Step 6: `sqlitetest.New`** con `t.Helper()`, `Open(filepath.Join(t.TempDir(), "muelle.db"))`, `Migrate` y `t.Cleanup(Close)`.

- [ ] **Step 7: Verificar y commit**

```bash
go get modernc.org/sqlite@v1.60.1
gofmt -l . ; go vet ./... && go test -race ./...
git add go.mod go.sum src/shared/infra/out/sqlite
git commit -m "feat(sqlite): conexión, migración inicial, Transactor y registro de auditoría" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task B4: Kit HTTP y servidor (`cmd/http`)

Portar de la Tarea 18 (ref) `respond.go` (`writeJSON`, `writeError`, `fail`) y `middleware.go` (`access`, `authenticate`, `tokenInScope`, `bearerToken`, `clientIP`), más la parte de `server.go` que pone los headers y revisa `Origin`. Todo se reparte entre `src/cmd/http/utils` (lo que usan los módulos) y `src/cmd/http` (el servidor).

**Cambios respecto de la referencia:**
- **Alcance de tokens:** `tokenInScope` se reemplaza por `entity.Principal.Allows(r.PathValue("p"), r.PathValue("s"), write)`, con `write := r.Method != GET && r.Method != HEAD`.
- **Errores:** `fail` deja de tener un `switch` fijo. Los módulos registran sus errores con `Kit.RegisterError`, y los comunes los registra `NewServer`.
- **JSON estricto:** `decodeJSON` pasa a `encoding/json/v2` en modo estricto.
- **Auditoría:** el `audit` del servidor desaparece; la hacen los casos de uso con `audit.Recorder`.

**Files:**
- Create: `src/cmd/http/utils/kit.go`: `Access`, `Authenticator`, `Kit` y su middleware de autenticación
- Create: `src/cmd/http/utils/respond.go`: `WriteJSON`, `WriteError`, `Kit.Fail`, `Kit.RegisterError`
- Create: `src/cmd/http/utils/decode.go`: `DecodeJSON`
- Create: `src/cmd/http/utils/request.go`: `ClientIP`, `BearerToken`, cookies de sesión
- Create: `src/cmd/http/server.go`
- Test: `src/cmd/http/utils/utils_test.go`, `src/cmd/http/server_test.go`

**API pública (fija):**

```go
package utils

const (
	SessionCookie = "muelle_session"
	MaxBodyBytes  = 1 << 20
)

// Access define quién puede llamar a un endpoint.
type Access int

const (
	AccessSession    Access = iota // solo sesión completa
	AccessToken                    // sesión completa, o token de API dentro de su alcance (Principal.Allows)
	AccessSetup                    // sesión completa, incluida la que todavía debe configurar 2FA
	AccessMFAPending               // solo la sesión que espera el código TOTP
)

// Authenticator lo implementa identity.
type Authenticator interface {
	AuthenticateSession(ctx context.Context, token string) (entity.Principal, error)
	AuthenticateToken(ctx context.Context, bearer string) (entity.Principal, error)
}

// Kit es lo que cmd/http le pasa a cada módulo para registrar sus rutas.
type Kit struct{ /* mux, auth, errores registrados, secureCookies, log */ }

func NewKit(mux *http.ServeMux, auth Authenticator, secureCookies bool, log *slog.Logger) *Kit

// Handle registra un endpoint autenticado con el nivel de acceso a.
// El Principal queda en el ctx (entity.PrincipalFrom).
func (k *Kit) Handle(pattern string, a Access, h http.HandlerFunc)
// HandlePublic registra un endpoint sin autenticación (login, setup, aceptar invitación).
func (k *Kit) HandlePublic(pattern string, h http.HandlerFunc)
// RegisterError asocia un error (comparado con errors.Is) a un status, código y mensaje.
func (k *Kit) RegisterError(target error, status int, code, message string)
// Fail responde el error: *validate.Error → 422 "invalid" con ve.Error();
// un error registrado → su status; cualquier otro → 500 "internal" y log.Error.
func (k *Kit) Fail(w http.ResponseWriter, err error)
func (k *Kit) SetSessionCookie(w http.ResponseWriter, token string, expires time.Time) // HttpOnly, Secure según config, SameSite=Strict, Path=/
func (k *Kit) ClearSessionCookie(w http.ResponseWriter)

func WriteJSON(w http.ResponseWriter, status int, v any)
func WriteError(w http.ResponseWriter, status int, code, message string)
// DecodeJSON lee el body con json/v2 estricto (máx. MaxBodyBytes): rechaza campos
// desconocidos, claves duplicadas, UTF-8 inválido y datos después del valor.
// Si falla responde 400 "invalid_json" con un mensaje en español sin detalles internos de Go, y devuelve false.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool
func ClientIP(r *http.Request) string
func BearerToken(r *http.Request) (string, bool)
```

```go
package http // src/cmd/http

type Config struct {
	PanelOrigin   string
	SecureCookies bool
	Auth          utils.Authenticator
	Logger        *slog.Logger
}

type Server struct{ /* mux, kit, origin */ }

// NewServer arma el mux y el Kit, y registra los errores comunes:
// repository.ErrNotFound 404 not_found, ErrConflict 409 conflict, ErrNotEmpty 409 not_empty,
// entity.ErrUnauthenticated 401 unauthenticated, entity.ErrForbidden 403 forbidden.
func NewServer(c Config) *Server
func (s *Server) Kit() *utils.Kit
// ServeHTTP pone X-Content-Type-Options: nosniff y, en /api/, Cache-Control: no-store;
// rechaza con 403 bad_origin las peticiones que modifican algo, sin Bearer y con un Origin distinto de PanelOrigin.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

**Comportamiento de `Kit.Handle`:** es el de `authenticate` de la Tarea 18 (ref), con estos códigos:

| Caso | Respuesta |
|---|---|
| Bearer en un endpoint que no es `AccessToken` | 403 `token_not_allowed` |
| Bearer fuera de alcance (`!Allows`) | 403 `out_of_scope` |
| Sin cookie | 401 `unauthenticated` |
| Sesión `MFAPending` en un endpoint que no es `AccessMFAPending` | 401 `mfa_required` |
| `AccessMFAPending` con una sesión completa | 400 `no_mfa_pending` |
| `NeedsTOTPSetup` en un endpoint que no es `AccessSetup` | 403 `totp_setup_required` |
| Error de `Authenticate*` | `k.Fail` |

- [ ] **Step 1: Tests de `utils_test.go`** (con un `Authenticator` falso dentro del archivo de test):
  - Cada fila de la tabla de `Kit.Handle`.
  - `DecodeJSON` rechaza con 400 `invalid_json`:
    - un campo desconocido;
    - `{"a":1,"a":2}`;
    - UTF-8 inválido (`"{\"a\":\"\xff\"}"`);
    - `{"a":1} x`;
    - un body de más de 1 MiB.
    
    Y acepta un JSON válido.
  - `Fail`:
    - un `*validate.Error` da 422 `invalid`;
    - un error registrado, aunque venga envuelto con `%w`, da su status;
    - uno desconocido da 500 `internal`, sin filtrar el texto del error.
  - `ClientIP` prefiere `X-Real-IP` y si no usa `RemoteAddr`.
  - `SetSessionCookie` respeta `secureCookies` y pone `HttpOnly` y `SameSite=Strict`.

- [ ] **Step 2: Tests de `server_test.go`:**
  - `POST` sin Bearer con `Origin` ajeno da 403 `bad_origin`.
  - `POST` con el `Origin` del panel pasa.
  - `POST` con Bearer y sin `Origin` pasa.
  - `GET` sin `Origin` pasa.
  - Los headers `nosniff` y `no-store` están presentes.
  - Los errores comunes registrados responden con su status.

- [ ] **Step 3: Implementar** hasta que pasen. Para el JSON estricto:

```go
import jsonv2 "encoding/json/v2"

err := jsonv2.UnmarshalRead(http.MaxBytesReader(w, r.Body, MaxBodyBytes), dst, jsonv2.RejectUnknownMembers(true))
```

  En json/v2 las claves duplicadas y el UTF-8 inválido ya se rechazan por defecto, y `UnmarshalRead` rechaza los datos después del valor. Verifícalo con los tests del Step 1.

- [ ] **Step 4: Verificar y commit**

```bash
gofmt -l . ; go vet ./... && go test -race ./...
git add src/cmd/http
git commit -m "feat(http): kit para los módulos, middleware de acceso, JSON estricto y servidor" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task B5: Cliente de prueba HTTP (`httptestutil`)

Portar los helpers de test de la Tarea 18 (ref) (`newTestEnv`, `do`, `withSession`, `withBearer`, `withOrigin`, `decode`, `errorCode`, `expect`) a un paquete reutilizable, para que cada módulo pruebe sus endpoints solo.

**Files:**
- Create: `src/cmd/http/httptestutil/httptestutil.go`, `httptestutil_test.go`

**API (fija):**

```go
package httptestutil

const PanelOrigin = "https://panel.test"

// FakeAuth mapea cookies y tokens a Principals fijos. Lo que no está devuelve entity.ErrUnauthenticated.
type FakeAuth struct {
	Sessions map[string]entity.Principal
	Tokens   map[string]entity.Principal
}

func (f FakeAuth) AuthenticateSession(ctx context.Context, token string) (entity.Principal, error)
func (f FakeAuth) AuthenticateToken(ctx context.Context, bearer string) (entity.Principal, error)

type Client struct{ /* http.Handler */ }

// New arma un Server (cmd/http) con PanelOrigin y cookies seguras, llama a
// register con su Kit y devuelve un cliente.
func New(t testing.TB, auth utils.Authenticator, register func(k *utils.Kit)) *Client

type Opt func(*http.Request)

func WithSession(token string) Opt // agrega la cookie muelle_session
func WithBearer(token string) Opt
func WithOrigin(origin string) Opt // reemplaza el Origin por defecto ("" lo quita)

// Do envía la petición con Origin: PanelOrigin por defecto; body se serializa a JSON si no es nil.
func (c *Client) Do(method, path string, body any, opts ...Opt) *httptest.ResponseRecorder

func Decode[T any](t testing.TB, w *httptest.ResponseRecorder) T
func ErrorCode(t testing.TB, w *httptest.ResponseRecorder) string
func Expect(t testing.TB, w *httptest.ResponseRecorder, status int) // falla mostrando el body
func SessionFrom(t testing.TB, w *httptest.ResponseRecorder) string // valor de la cookie muelle_session
```

- [ ] **Step 1:** Test que registra una ruta `GET /api/v1/ping` (`AccessSession`) y verifica:
  - sin sesión da 401;
  - con `WithSession("s")` y `FakeAuth{Sessions: {"s": …}}` da 200;
  - `Decode` y `ErrorCode` funcionan.
- [ ] **Step 2:** Implementar.
- [ ] **Step 3:** Verificar y commit: `feat(http): cliente de prueba para los módulos`.

### Task B6: Tareas periódicas (`cmd/cron`) y cierre de la Etapa 1

**Files:**
- Create: `src/cmd/cron/cron.go`, `cron_test.go`

```go
// Package cron ejecuta las tareas periódicas que registran los módulos.
package cron

type Job struct {
	Name  string
	Every time.Duration
	Run   func(ctx context.Context) error
}

// Run ejecuta cada job cada Every (la primera vez después de Every) hasta que
// ctx se cancele. Un error se registra en el log y no detiene al job. Bloquea hasta que ctx termina.
func Run(ctx context.Context, jobs []Job, log *slog.Logger)
```

- [ ] **Step 1:** Test con `Every: 10*time.Millisecond`:
  - el job corre al menos 2 veces en 50 ms;
  - un job que devuelve error sigue corriendo;
  - `Run` termina cuando se cancela el ctx.
- [ ] **Step 2:** Implementar (una goroutine con `time.Ticker` por job).
- [ ] **Step 3:** Verificar y commit: `feat(cron): ejecución de tareas periódicas`.
- [ ] **Step 4: Cierre.** `go test -race ./...` pasa entero, y queda un tag local `etapa-1` (`git tag etapa-1`) del que salen las ramas de módulo.

---

# Etapa 2 — Módulos (en paralelo)

**Reglas comunes a las cuatro tareas:**
- Trabajar en un worktree propio con la rama `fase-1-<módulo>` creada desde `etapa-1`.
- **Solo crear archivos en `src/internal/<módulo>/`**, salvo lo que la tarea diga explícitamente. Para dependencias nuevas, `go get` cambia `go.mod` y `go.sum`; está permitido.
- **No importar otro módulo** (`src/internal/<otro>/…`). Lo que se necesite de otro módulo se pide con un puerto en `domain/service`, y su adaptador SQL en `infra/out/service` lee esas tablas directo.
- **Capas:**
  - `domain` no importa `infra`, `database/sql` ni `net/http`;
  - los casos de uso dependen de interfaces de `domain`, no de `sqlite`.
- **`module.go`** en `src/internal/<m>/` define `Deps`, `New(Deps) *Module` y `RegisterRoutes(k *utils.Kit)`, con las firmas que indica cada tarea. Es el único lugar del módulo que conoce todas las piezas.
- **Tests por capa:**
  - dominio, puros;
  - repositorios, con `sqlitetest.New`;
  - casos de uso, con repositorios reales y `clock.NewManual`;
  - HTTP, con `httptestutil.New` contra `module.RegisterRoutes`.
- **Mapeo de los tests de referencia:** los de los paquetes `store`, `auth`, `envvars` y `api` de la referencia se portan a la capa que corresponda. Los tests de API de la referencia pasan a ser los tests HTTP del módulo, con los mismos casos y los mismos códigos de respuesta.
- **Commits:** chicos, uno por capa como mínimo (dominio, repositorios, casos de uso, HTTP).
- **Al terminar:** `go test -race ./...` pasa en el worktree; reportar los archivos creados y los commits.

### Task M-identity: usuarios, sesiones, 2FA, invitaciones y tokens

**Referencia:**
- Tarea 11: store de usuarios
- Tarea 12: store de sesiones
- Tarea 13: store de tokens e invitaciones
- Tarea 16: contraseñas, aleatorios y limitador
- Tarea 17: servicio de auth
- Tarea 18: handlers de `/auth` y sus tests
- Tarea 22: la parte de `/users`, `/invites` y `/tokens`, más `TestTokenScopes`

**`module.go`:**

```go
package identity

type Deps struct {
	DB     *sqlite.DB
	Cipher cipher.Cipher
	Clock  clock.Clock
	Audit  audit.Recorder
	Log    *slog.Logger
}

func New(d Deps) *Module
func (m *Module) Authenticator() utils.Authenticator
func (m *Module) RegisterRoutes(k *utils.Kit)
func (m *Module) Jobs() []cron.Job // "sesiones vencidas", cada hora
// EnsureSetupInvite crea la invitación de setup si no hay usuarios ni una vigente (Tarea 17 ref).
func (m *Module) EnsureSetupInvite(ctx context.Context) (token string, created bool, err error)
```

**Dominio:**
- **`domain/entity`:** `User`, `Session`, `Invite`, `APIToken`.
- **`domain/repository`:** `UserRepository`, `SessionRepository`, `InviteRepository`, `TokenRepository`, con los métodos que usa la Tarea 17 (ref). Las operaciones del store viejo pasan a ser métodos de estos repositorios.
- **`domain/service`:**
  - **Errores:** `ErrInvalidCredentials`, `ErrLocked`, `ErrRateLimited`, `ErrInvalidCode`, `ErrNoPendingTOTP`, `ErrInviteInvalid`.
  - **Constantes:** `SessionIdle = 12h`, `SessionMax = 7d`, `InviteTTL = 24h`, más las de bloqueo de la Tarea 16 (ref).
  - **`PasswordHasher`:** `Hash(password string) (string, error)` y `Verify(hash, password string) (bool, error)`.
  - **`TOTP`:** generar el secreto y validar códigos.
  - **`LoginLimiter`:** por IP (20 intentos cada 15 min).
  - **`LockoutFor(fails int) time.Duration`:** el bloqueo progresivo.
- **Puertos hacia otros módulos**, con adaptadores SQL en `infra/out/service`:
  - `ScopeResolver`: de proyecto y stage (slugs) a ids, para crear tokens. Si no existen, `repository.ErrNotFound`.
  - `SettingsReader`: `Require2FA(ctx) (bool, error)`, que lee `settings` con clave `require_2fa` y valor `"true"`/`"false"`; si no existe, `false`.

**Contraseñas:**
- **Argon2id** con `m=19*1024` KiB, `t=2`, `p=1`, salt de 16 bytes y clave de 32 bytes, en formato PHC (`$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>`).
- **`Verify`** lee los parámetros del propio hash.
- **Semáforo de capacidad 2** compartido por `Hash` y `Verify` (`chan struct{}`).
- **Test del semáforo:** con 4 goroutines y un hasher instrumentado, nunca hay más de 2 dentro a la vez.

**Casos de uso (`app/usecases`, uno por archivo):**
- `Setup`, `Login`, `CompleteTOTP`, `Logout`
- `AuthenticateSession`, `AuthenticateToken`: implementan `utils.Authenticator` y producen `entity.Principal`. El de token llena `TokenScope` con los slugs y actualiza `last_used_at`.
- `Me`, `BeginTOTPSetup`, `EnableTOTP`
- `CreateInvite`, `AcceptInvite`, `EnsureSetupInvite`
- `ListUsers`, `DeleteUser`, `ResetUserTOTP`
- `CreateToken`, `ListTokens`, `DeleteToken`
- `CleanupSessions`

Cada caso de uso:
- usa `audit.Recorder` con las mismas acciones que la referencia;
- usa `Transactor` donde hace falta atomicidad: `Setup` (solo si no hay usuarios), `AcceptInvite` (marca usada y crea el usuario), y el registro de fallos de login.
- **AAD del TOTP:** `users:totp:<id>`. El secreto pendiente usa `users:totp_pending:<id>`.

**HTTP (`infra/in/http`):**

| Ruta | Acceso |
|---|---|
| `POST /api/v1/auth/login`, `POST /api/v1/auth/setup`, `POST /api/v1/auth/invite/{token}` | `HandlePublic` |
| `POST /api/v1/auth/totp` | `AccessMFAPending` |
| `POST /api/v1/auth/logout`, `GET /api/v1/auth/me`, `POST /api/v1/auth/totp/setup`, `POST /api/v1/auth/totp/enable` | `AccessSetup` |
| `GET /api/v1/users`, `DELETE /api/v1/users/{username}`, `DELETE /api/v1/users/{username}/totp`, `POST /api/v1/invites`, `GET /api/v1/tokens`, `POST /api/v1/tokens`, `DELETE /api/v1/tokens/{id}` | `AccessSession` |

- Los cuerpos van en `infra/in/form`, las respuestas en `infra/in/http/res.go` y los errores en `infra/in/http/mapper_errors.go`, con los mismos status y códigos que el `fail` de la Tarea 18 (ref).
- Las cookies se manejan con `k.SetSessionCookie` y `k.ClearSessionCookie`, y la IP con `utils.ClientIP`.

**Tests:**
- Todos los de las Tareas 11, 12, 13, 16, 17 y 18 (ref), y los de `/users`, `/invites` y `/tokens` de la 22, en su capa.
- `TestTokenScopes` de la Tarea 22 (ref) se porta como test de `AuthenticateToken` + `Principal.Allows`. La matriz HTTP completa, que necesita las rutas de `catalog` y `secrets`, se prueba en la Integración (I3).

### Task M-catalog: proyectos, stages y apps

**Referencia:**
- Tarea 7: store de proyectos
- Tarea 8: store de stages
- Tarea 9: store de apps
- Tarea 5: appspec, ya implementado en `internal/appspec`
- Tarea 19: handlers de proyectos y stages
- Tarea 20: handlers de apps

**`module.go`:**

```go
package catalog

type Deps struct {
	DB    *sqlite.DB
	Clock clock.Clock
	Audit audit.Recorder
	Log   *slog.Logger
}

func New(d Deps) *Module
func (m *Module) RegisterRoutes(k *utils.Kit)
```

**Mudanza de appspec:**
- `git mv internal/appspec/appspec.go src/internal/catalog/domain/vo/appspec.go`, y lo mismo con el test. Después, borrar `internal/`.
- El paquete pasa a ser `vo`, con `type AppSpec` y `func ParseAppSpec(raw []byte) (AppSpec, error)`. Los demás nombres internos quedan igual.

**Arreglos de appspec** (revisión de la Tarea 5):
1. **Crítico, path del healthcheck:** quitar `&` de `healthPathRe` y agregar tests de rechazo para `&`, `;`, `|`, `$`, comilla invertida, comillas y espacio. Actualizar el comentario: en la Fase 2 el healthcheck `http` se genera en forma exec, sin shell (§6.1).
2. **Imagen:** validarla con `reference.ParseNormalizedNamed` de `github.com/distribution/reference` (`go get github.com/distribution/reference`). Además, rechazar la que empiece con `-` o contenga `$`. Tests:
   - se rechazan `${X}`, `--help`, `Ghcr.io/Foo/Bar:Latest`, NUL, `../../containers/json?` y espacios;
   - se aceptan `nginx`, `nginx:1.27`, `ghcr.io/aumi-app/backend-app:sha-<40 hex>`, `registry:5000/a/b@sha256:<64 hex>` y `ghcr.io/x/y:tag@sha256:<64 hex>`.
3. **JSON estricto:** `ParseAppSpec` decodifica con `encoding/json/v2` (`UnmarshalRead` o `Unmarshal` con `RejectUnknownMembers(true)`). Tests: datos después del JSON, clave duplicada, UTF-8 inválido y `{"IMAGE":…}` (las mayúsculas no deben mezclarse con `image`). Si `JSON()` pasa a v2, usar `omitzero` donde había `omitempty` en enteros.
4. **`start_period_s`:** subir el mínimo a 1 y ajustar el mensaje.
5. **Healthcheck `none`:** rechazar campos de tiempo distintos de cero.
6. **Montajes:** regex `^/[A-Za-z0-9._/-]{1,255}$`. Tests con `/data:ro`, un salto de línea, `/$HOME` y 5000 caracteres.
7. **NaN:** escribir el chequeo de CPU como `!(r.CPUs >= 0.01 && r.CPUs <= 256)`.
8. **Tests:** `TestParseRejects` verifica también `ve.Field`, y `TestJSONRoundTrip` usa `reflect.DeepEqual` más `bytes.Equal` de los JSON.

**Dominio:**
- **`domain/entity`:** `Project`, `Stage` y `App` (con `Kind` `service`|`compose`). Los constructores validan con `shared/domain/validate`.
- **`domain/repository`:** `ProjectRepository`, `StageRepository` y `AppRepository`, con las operaciones de las Tareas 7-9 (ref).
- **`domain/service/errors.go`:** `ErrKindMismatch`.
- **Borrado:**
  - un proyecto con stages, o un stage con apps, da `ErrNotEmpty`;
  - el chequeo y el borrado van en `InTx`;
  - las claves foráneas `RESTRICT` de la migración también lo garantizan.

**Casos de uso:**
- `CreateProject`, `ListProjects`, `GetProject`, `UpdateProject`, `DeleteProject`
- `CreateStage`, `ListStages`, `GetStage`, `DeleteStage`
- `CreateApp`, `ListApps`, `GetApp`, `UpdateApp`, `DeleteApp`

Detalles:
- Las apps de formulario guardan el spec **normalizado**. Las compose solo se validan como no vacías y de hasta 256 KiB (`validateCompose` de la Tarea 20 ref).
- `GetApp` y `ListApps` devuelven `pending_changes` (`config_changed_at > last_deployed_at`, o sin deploy).
- Cambiar el spec o el compose actualiza `config_changed_at`.
- Auditoría con las mismas acciones que la referencia.

**HTTP:**
- **Rutas:** las de las Tareas 19 y 20 (ref), con el mismo acceso.
  - `GET` de proyecto, stages, stage, apps y app: `AccessToken`.
  - El resto, incluido `GET /api/v1/projects`: `AccessSession`.
- **Permisos de los tokens:** cada handler con `AccessToken` igual verifica el alcance. El middleware ya lo hace con `{p}`/`{s}`, así que un test basta para cubrirlo.
- **Errores:** se registra `ErrKindMismatch` → 422 `kind_mismatch`.

**Tests:** todos los de las Tareas 7, 8, 9, 19 y 20 (ref), en su capa, más los nuevos de appspec.

### Task M-secrets: variables cifradas de stage y app

**Referencia:**
- Tarea 10: store de variables
- Tarea 15: servicio `envvars` (cifrar, enmascarar, mezclar)
- Tarea 21: handlers de variables

**`module.go`:**

```go
package secrets

type Deps struct {
	DB     *sqlite.DB
	Cipher cipher.Cipher
	Clock  clock.Clock
	Audit  audit.Recorder
	Log    *slog.Logger
}

func New(d Deps) *Module
func (m *Module) RegisterRoutes(k *utils.Kit)
```

**Dominio:**
- **`domain/entity`:** `EnvVar` (`Key`, `Value []byte` en claro solo en memoria, `Secret bool`, `UpdatedAt`) y `Owner` (`Kind` `stage`|`app`, `ID int64`).
- **`domain/repository`:** `EnvRepository`, que guarda y lee valores **ya cifrados**, con las operaciones de la Tarea 10 (ref): listar, reemplazar y aplicar un patch.
- **Puertos hacia `catalog`** (`domain/service`), con adaptadores SQL en `infra/out/service` sobre `projects`, `stages` y `apps`:
  - `OwnerResolver.Stage(ctx, project, stage string) (Owner, error)`
  - `OwnerResolver.App(ctx, project, stage, app string) (Owner, error)`
  - `ChangeMarker.Mark(ctx, owner Owner, at time.Time) error`: para una app, actualiza su `config_changed_at`; para un stage, el de todas sus apps.
  - Si un slug no existe, devuelven `repository.ErrNotFound`.
- **`domain/service/envcrypto.go`:** el cifrado, el enmascarado y la mezcla de la Tarea 15 (ref), sobre `cipher.Cipher`. AAD: `env:app:<id>:<key>` y `env:stage:<id>:<key>`.

**Casos de uso:**
- `GetEnv`: devuelve los valores enmascarados si son secretos.
- `ReplaceEnv` (PUT) y `PatchEnv` (`set`/`unset`): el cambio de variables y `ChangeMarker.Mark` van en el **mismo `InTx`**.
- `ResolveEnv`: mezcla las del stage con las de la app (gana la app). La usa la Fase 2, pero se implementa y se prueba ahora.

`secret` es `true` si se omite. La auditoría guarda **solo las claves**.

**HTTP:**
- **Rutas:** las de la Tarea 21 (ref), `GET`/`PUT`/`PATCH` en `…/stages/{s}/env` y `…/apps/{a}/env`, todas con `AccessToken`.
- **Validación:** las claves pasan por `validate.EnvKey("vars"|"set"|"unset", k)`.

**Tests:**
- Todos los de las Tareas 10, 15 y 21 (ref), en su capa.
- Nuevo: un `PUT` que falla en `Mark` deja las variables como estaban (rollback).
- Nuevo: un `PATCH` sobre un stage marca todas sus apps.

### Task M-admin: settings, auditoría, backup y clave maestra

**Referencia:**
- Tarea 14: auditoría, settings y backup en el store
- Tarea 22: la parte de `/settings` y `/audit`
- Tarea 23: `maintenance`

**`module.go`:**

```go
package admin

type Deps struct {
	DB      *sqlite.DB
	Cipher  cipher.Cipher
	Clock   clock.Clock
	Audit   audit.Recorder
	Log     *slog.Logger
	DataDir string // backups en <DataDir>/backups
}

func New(d Deps) *Module
func (m *Module) RegisterRoutes(k *utils.Kit)
func (m *Module) Jobs() []cron.Job // "retención de auditoría" y "backup diario", cada hora
// VerifyMasterKey: en el primer arranque guarda en settings (clave "master_key_check",
// encrypted=1, AAD "settings:master_key_check") un valor de control cifrado; en los
// siguientes lo descifra. Si falla devuelve ErrWrongMasterKey ("clave maestra incorrecta").
func (m *Module) VerifyMasterKey(ctx context.Context) error
```

**Dominio:**
- **`domain/repository`:** `SettingsRepository` (`Get`, `Set` y `GetEncrypted`/`SetEncrypted`, o el equivalente con la columna `encrypted`) y `AuditRepository` (`List(beforeID, limit)`, `DeleteBefore(t)`).
- **`domain/service`:**
  - `ErrWrongMasterKey`.
  - Puerto `TOTPStatus.HasTOTP(ctx, userID) (bool, error)`, con adaptador SQL sobre `users.totp_secret IS NOT NULL`.

**Casos de uso:**
- `GetSettings` y `PatchSettings`: en la Fase 1 solo existe `require_2fa`, guardado como `"true"`/`"false"` con clave `require_2fa`, el mismo formato que lee identity. No se puede activar sin tener el propio TOTP configurado (§10.1).
- `ListAudit`: paginada por id descendente, como en la Tarea 14 (ref).
- `PurgeAudit`: retención de 180 días.
- `Backup`: `VACUUM INTO <DataDir>/backups/muelle-<AAAA-MM-DD>.db`, conservando los últimos 7. El job corre cada hora y solo crea el backup si todavía no existe el del día.
- `VerifyMasterKey`.

**HTTP:**
- **Rutas:** `GET /api/v1/settings`, `PATCH /api/v1/settings` y `GET /api/v1/audit`, con `AccessSession`.
- **Errores:** se registra un error para "activar require_2fa sin TOTP propio", con el código de la Tarea 22 (ref).

**Tests:**
- Todos los de las Tareas 14 y 22 (ref, parte de settings y auditoría), en su capa.
- `VerifyMasterKey`:
  - el primer arranque guarda el valor;
  - el segundo con la misma clave pasa;
  - con otra clave (otro `crypto.New`) devuelve `ErrWrongMasterKey`.
- `Backup`: la segunda llamada del mismo día no crea otro archivo, y conserva solo 7.

---

# Etapa 3 — Integración (secuencial)

**Antes de I1:** mergear a `fase-1-base` las ramas `fase-1-identity`, `fase-1-catalog`, `fase-1-secrets` y `fase-1-admin`, de a una, corriendo `go test -race ./...` después de cada merge. Los únicos conflictos posibles están en `go.mod`/`go.sum` y se resuelven con `go mod tidy`.

### Task I1: Contenedor

**Files:**
- Create: `src/shared/infra/container/container.go`, `container_test.go`

```go
package container

type Container struct {
	DB       *sqlite.DB
	Identity *identity.Module
	Catalog  *catalog.Module
	Secrets  *secrets.Module
	Admin    *admin.Module
	Server   *httpserver.Server // src/cmd/http
	Jobs     []cron.Job
}

// New abre la base en <DataDir>/muelle.db, migra, carga la clave maestra
// (crypto.LoadKeyFile(s.MasterKeyFile)), arma los módulos con clock.System y un
// AuditRecorder, verifica la clave maestra (admin.VerifyMasterKey), arma el
// servidor con identity.Authenticator() y registra las rutas de los cuatro módulos.
func New(ctx context.Context, s settings.Settings, log *slog.Logger) (*Container, error)
func (c *Container) Close() error
```

- [ ] **Step 1: Tests:**
  - `New` con un `DataDir` temporal y una clave generada arma todo, y `GET /api/v1/auth/me` sin sesión da 401.
  - Con otra clave sobre la misma base devuelve un error que contiene "clave maestra incorrecta".
- [ ] **Step 2:** Implementar.
- [ ] **Step 3:** Verificar y commit: `feat(container): une los módulos`.

### Task I2: `main.go` (`serve` y `genkey`)

Portar de la Tarea 23 (ref) `main`, `serve` y `genkey`, con estos cambios:
- `serve` usa `settings.Load` y `container.New`;
- imprime la URL de setup con `Identity.EnsureSetupInvite`;
- corre `cron.Run(ctx, c.Jobs, log)`;
- usa `http.Server` con los mismos timeouts y el mismo apagado ordenado que la referencia, sobre `c.Server`.

**Files:**
- Create: `main.go` (en la raíz; `go build` genera el binario `muelle`)
- Delete: `cmd/` y `internal/`, si quedó algo

- [ ] **Step 1:** Implementar, y `go build -o /tmp/muelle-bin .`.
- [ ] **Step 2:** Verificar y commit: `feat: comando muelle serve`.

### Task I3: Prueba de punta a punta y matriz de alcance

- [ ] **Step 1:** Portar `TestTokenScopes` de la Tarea 22 (ref) completo como test de `container`. Usa el servidor real con los cuatro módulos y la misma matriz de filas y status.
- [ ] **Step 2:** Correr el flujo `curl` de la Tarea 23 (ref, Steps 3-4) contra el binario real:
  - setup → login → crear proyecto, stage y app → crear token de CI → `PUT …/env` con el token → `GET …/env` enmascarado → token de otro stage da 403;
  - al final, `unset` de las variables `MUELLE_*` exportadas.
- [ ] **Step 3:** Commit del test: `test: matriz de alcance de tokens con todos los módulos`.

### Task I4: CI y README

Portar la Tarea 24 (ref), con estos cambios:
- `ci.yaml` usa `go-version-file: go.mod`;
- el README describe la estructura de §4.1 y cómo correr `muelle serve` en desarrollo (`MUELLE_PANEL_ORIGIN=http://127.0.0.1:18080`, `MUELLE_SECURE_COOKIES=false`).

- [ ] Commit: `chore: CI y README`.

**Cierre de la Fase 1:** revisión final de todo el diff `0693ccd..HEAD` y después `superpowers:finishing-a-development-branch`.
