# muelle — Fase 1 (Base): Plan de implementación

> **Reemplazado** por [`2026-10-10-muelle-fase1-hexagonal.md`](2026-10-10-muelle-fase1-hexagonal.md), que reorganiza la Fase 1 con la arquitectura hexagonal (§4 del spec). Este plan queda como **código de referencia**: el SQL, la lógica y los tests de cada tarea se adaptan a la nueva estructura.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Un binario `muelle serve` con la API REST de la Fase 1: usuarios (setup, invitaciones, 2FA), sesiones, tokens de API con alcance, y CRUD de proyectos, stages, apps y variables cifradas, todo persistido en SQLite.

**Architecture:** Capas en paquetes chicos bajo `internal/`:
- **Persistencia:** `store` (SQLite) y `crypto` (AES-GCM).
- **Lógica:** `validate` (reglas), `appspec` (spec del formulario), `envvars` (cifrado y enmascarado de variables) y `auth` (sesiones, 2FA, tokens).
- **HTTP:** `api` (handlers con `net/http`, autenticación por cookie o token Bearer).
- **Entrada:** `cmd/muelle`, que une todo.

Cada capa solo conoce a las de abajo, y el store recibe los valores ya validados y cifrados.

**Tech Stack:**
- Go 1.27 con `net/http` de la librería estándar (patrones `MÉTODO /ruta/{param}`)
- `modernc.org/sqlite` v1.60.1 (Go puro, sin CGO)
- `golang.org/x/crypto` v0.58.0 (argon2id)
- `github.com/pquerna/otp` v1.5.0 (TOTP)

**Spec:** [`docs/superpowers/specs/2026-10-09-muelle-design.md`](../specs/2026-10-09-muelle-design.md). Las referencias `§N` apuntan a sus secciones.

---

## Contexto para quien ejecuta

muelle es un deploy manager ultraligero sobre Docker Swarm (§1). El spec divide la construcción en 5 fases (§17), y **este plan cubre solo la Fase 1**.

**Qué entrega:** la API sobre la que se apoyan las demás fases. Al final se puede crear el primer usuario, invitar a otros, activar 2FA, crear proyectos, stages y apps, cargar variables cifradas y crear tokens de CI que hacen `PUT …/env` igual que el `sync-env` actual de GitHub Actions.

**Qué NO hace todavía:**
- no habla con Docker (Fase 2)
- no configura Caddy ni Cloudflare (Fase 3)
- no tiene panel web (Fase 4)
- no tiene instalador (Fase 5)

**Cómo saber que terminó:** la Task 23 corre el binario real y prueba el flujo completo con `curl`.

## Convenciones

- **Repo:** `/home/felipe/Escritorio/dev/muelle`, rama `main`; ya tiene el commit del spec. **Todos los comandos se ejecutan desde la raíz del repo.**
- **Go:** 1.27 (la máquina tiene go1.27.1). Sin CGO: el binario sale estático.
- **Idioma:** comentarios, mensajes de error y commits en español; identificadores en inglés.
- **Errores:**
  - El store devuelve los sentinelas `store.ErrNotFound`, `ErrConflict`, `ErrNotEmpty`, `ErrKindMismatch` y `ErrInviteInvalid`.
  - Las reglas de validación devuelven `*validate.Error`.
  - `api.(*Server).fail` traduce todo eso a HTTP con el formato `{"error": {"code", "message"}}`.
  - No inventes otros tipos de error.
- **Tiempo:** SQLite guarda milisegundos Unix. `store.Store.Now` es el reloj, y los tests lo reemplazan para controlar el tiempo.
- **Tests:** en el mismo paquete que el código (white-box), con SQLite real en `t.TempDir()` y sin mocks. Los helpers de un archivo de test (`newTestStore`, `seedStage`, `newTestEnv`, …) se reutilizan en las tareas siguientes del mismo paquete.
- **TDD:** en cada tarea se escribe el test, se verifica que falla, se implementa, se verifica que pasa y se hace el commit. Los "Expected" de cada paso se capturaron simulando este plan completo sobre un repo vacío, así que si ves otra cosa, detente y revisa antes de seguir.
- **Commits:** cada commit termina con la línea `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` (el segundo `-m` de cada comando).
- **Alcance:** no modifiques archivos que la tarea no lista.

## Ajustes respecto al spec

Salieron al verificar este plan y ya están reflejados en el spec:

1. **`env_vars`** usa dos columnas, `stage_id` o `app_id` (exactamente una con valor), en vez de `owner_type`/`owner_id`. Así las variables se borran en cascada con su stage o app.
2. **`users.totp_pending`** guarda el secreto TOTP generado hasta que el usuario lo confirma con un código.
3. **`sessions.mfa_pending`** marca la sesión intermedia del login con 2FA.
4. **IP del cliente:** muelle la lee de `X-Real-IP`. La Fase 3 hace que Caddy lo envíe con `header_up X-Real-IP {client_ip}`.
5. **`PATCH /settings`** no permite activar `require_2fa` a quien no tiene su propio 2FA configurado.
6. **Origen del panel:** en la Fase 1 viene de la variable `MUELLE_PANEL_ORIGIN`. En la Fase 5, `install` la configura.
7. **Endpoints extra:** `GET /auth/me`, `POST /auth/totp/setup`, `POST /auth/totp/enable`, `DELETE /users/{username}` y `DELETE /users/{username}/totp`.
8. **Tablas posteriores:** `deploys` y `registries` llegan en la migración `0002` (Fase 2) y `domains` en la `0003` (Fase 3).

## Mapa de archivos

```
muelle/
├── go.mod, go.sum, .gitignore, README.md
├── .github/workflows/ci.yaml           gofmt + vet + test -race + build
├── cmd/muelle/main.go                  subcomandos serve y genkey; mantenimiento horario
└── internal/
    ├── config/config.go                variables MUELLE_*
    ├── crypto/crypto.go                AES-256-GCM con AAD, clave maestra
    ├── validate/validate.go            slugs, usuario, contraseña, claves; tipo *Error
    ├── appspec/appspec.go              spec del formulario: parseo, defaults, validación
    ├── store/                          SQLite (no cifra ni valida)
    │   ├── migrations/0001_init.sql    todas las tablas de la Fase 1
    │   ├── store.go                    Open, Migrate, errores, helpers de tiempo
    │   ├── projects.go stages.go apps.go envvars.go
    │   ├── users.go sessions.go tokens.go invites.go
    │   └── audit.go settings.go backup.go
    ├── envvars/envvars.go              cifra, enmascara y mezcla variables
    ├── auth/                           password.go random.go limiter.go service.go
    └── api/                            server.go middleware.go respond.go
                                        auth_handlers.go projects_handlers.go apps_handlers.go
                                        env_handlers.go admin_handlers.go
```

## Tareas


### Task 1: Esqueleto del módulo

Crea el módulo Go y el `.gitignore`. No hay código todavía; las dependencias se agregan en la tarea que las usa por primera vez.

**Files:**
- Create: `go.mod`, `.gitignore`

- [ ] **Step 1: Inicializar el módulo**

```bash
cd /home/felipe/Escritorio/dev/muelle
go mod init github.com/CPU-commits/muelle
go mod edit -go=1.27
```

Expected: `go.mod` contiene `module github.com/CPU-commits/muelle` y `go 1.27`.

- [ ] **Step 2: Crear `.gitignore`**

```
/muelle
*.db
*.db-shm
*.db-wal
/data/
/tmp/
```

- [ ] **Step 3: Commit**

```bash
git add go.mod .gitignore
git commit -m "chore: esqueleto del m\u00f3dulo Go" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 2: Configuración por variables de entorno

`config.Load()` lee las variables `MUELLE_*`. `MUELLE_PANEL_ORIGIN` es obligatoria y tiene que ser un origen exacto (sin ruta), porque se compara literalmente contra el header `Origin` para la protección CSRF (§10.1 del spec).

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Escribir el test**

`internal/config/config_test.go`:

```go
package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("MUELLE_PANEL_ORIGIN", "https://panel.example.com")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != ":8080" || c.DataDir != "/data" || c.MasterKeyFile != "/run/secrets/muelle_master_key" {
		t.Fatalf("defaults incorrectos: %+v", c)
	}
	if !c.SecureCookies {
		t.Fatal("SecureCookies debe ser true por defecto")
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("MUELLE_PANEL_ORIGIN", "http://localhost:8080")
	t.Setenv("MUELLE_LISTEN", "127.0.0.1:9000")
	t.Setenv("MUELLE_DATA_DIR", "/tmp/m")
	t.Setenv("MUELLE_MASTER_KEY_FILE", "/tmp/key")
	t.Setenv("MUELLE_SECURE_COOKIES", "false")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "127.0.0.1:9000" || c.DataDir != "/tmp/m" || c.MasterKeyFile != "/tmp/key" || c.SecureCookies {
		t.Fatalf("overrides incorrectos: %+v", c)
	}
}

func TestLoadRejectsBadOrigin(t *testing.T) {
	for _, origin := range []string{"", "panel.example.com", "ftp://x.com", "https://x.com/", "https://x.com/panel"} {
		t.Setenv("MUELLE_PANEL_ORIGIN", origin)
		if _, err := Load(); err == nil {
			t.Errorf("origen %q debió fallar", origin)
		}
	}
}

func TestLoadRejectsBadBool(t *testing.T) {
	t.Setenv("MUELLE_PANEL_ORIGIN", "https://panel.example.com")
	t.Setenv("MUELLE_SECURE_COOKIES", "quizás")
	if _, err := Load(); err == nil {
		t.Fatal("debió fallar")
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/config/
```

Expected: FAIL. La primera línea relevante es:

```
internal/config/config_test.go:7:12: undefined: Load
```

- [ ] **Step 3: Implementar**

`internal/config/config.go`:

```go
// Package config lee la configuración de muelle desde variables de entorno.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
)

type Config struct {
	Listen        string // MUELLE_LISTEN, por defecto ":8080"
	DataDir       string // MUELLE_DATA_DIR, por defecto "/data"
	MasterKeyFile string // MUELLE_MASTER_KEY_FILE, por defecto "/run/secrets/muelle_master_key"
	PanelOrigin   string // MUELLE_PANEL_ORIGIN, obligatorio, ej. "https://panel.tudominio.cl"
	SecureCookies bool   // MUELLE_SECURE_COOKIES, por defecto true
}

func Load() (Config, error) {
	c := Config{
		Listen:        getenv("MUELLE_LISTEN", ":8080"),
		DataDir:       getenv("MUELLE_DATA_DIR", "/data"),
		MasterKeyFile: getenv("MUELLE_MASTER_KEY_FILE", "/run/secrets/muelle_master_key"),
		PanelOrigin:   os.Getenv("MUELLE_PANEL_ORIGIN"),
		SecureCookies: true,
	}
	if v := os.Getenv("MUELLE_SECURE_COOKIES"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("MUELLE_SECURE_COOKIES: %w", err)
		}
		c.SecureCookies = b
	}
	if err := validateOrigin(c.PanelOrigin); err != nil {
		return Config{}, fmt.Errorf("MUELLE_PANEL_ORIGIN: %w", err)
	}
	return c, nil
}

// validateOrigin exige un origen exacto (esquema + host[:puerto]), sin ruta,
// porque se compara literalmente contra el header Origin.
func validateOrigin(origin string) error {
	if origin == "" {
		return fmt.Errorf("es obligatorio (ej. https://panel.tudominio.cl)")
	}
	u, err := url.Parse(origin)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("el esquema debe ser http o https")
	}
	if u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return fmt.Errorf("debe ser solo esquema y host, sin ruta ni barra final (ej. https://panel.tudominio.cl)")
	}
	return nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/config/ && go test ./internal/config/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/config
```

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): carga de configuraci\u00f3n desde MUELLE_*" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 3: Cifrado AES-256-GCM con la clave maestra

Todos los secretos se cifran con esta pieza (§12). `Encrypt` antepone un nonce aleatorio al ciphertext. El parámetro `aad` (datos asociados) liga el valor a su fila: descifrar con otra AAD falla. Desde Go 1.24, `crypto/rand.Read` nunca devuelve error, por eso no se chequea.

**Files:**
- Create: `internal/crypto/crypto.go`
- Test: `internal/crypto/crypto_test.go`

- [ ] **Step 1: Escribir el test**

`internal/crypto/crypto_test.go`:

```go
package crypto

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testCipher(t *testing.T) *Cipher {
	t.Helper()
	c, err := New(bytes.Repeat([]byte{7}, KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRoundTrip(t *testing.T) {
	c := testCipher(t)
	ct := c.Encrypt([]byte("postgres://secreto"), "env:app:1:DATABASE_URL")
	pt, err := c.Decrypt(ct, "env:app:1:DATABASE_URL")
	if err != nil {
		t.Fatal(err)
	}
	if string(pt) != "postgres://secreto" {
		t.Fatalf("got %q", pt)
	}
}

func TestNonceIsRandom(t *testing.T) {
	c := testCipher(t)
	a := c.Encrypt([]byte("x"), "aad")
	b := c.Encrypt([]byte("x"), "aad")
	if bytes.Equal(a, b) {
		t.Fatal("dos cifrados del mismo valor no deben ser iguales")
	}
}

func TestWrongAADFails(t *testing.T) {
	c := testCipher(t)
	ct := c.Encrypt([]byte("x"), "env:app:1:A")
	if _, err := c.Decrypt(ct, "env:app:2:A"); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("esperaba ErrDecrypt, got %v", err)
	}
}

func TestTamperedFails(t *testing.T) {
	c := testCipher(t)
	ct := c.Encrypt([]byte("x"), "aad")
	ct[len(ct)-1] ^= 0xff
	if _, err := c.Decrypt(ct, "aad"); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("esperaba ErrDecrypt, got %v", err)
	}
	if _, err := c.Decrypt([]byte{1, 2}, "aad"); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("esperaba ErrDecrypt con datos cortos, got %v", err)
	}
}

func TestNewRejectsBadKeySize(t *testing.T) {
	if _, err := New(make([]byte, 16)); err == nil {
		t.Fatal("debió fallar con 16 bytes")
	}
}

func TestLoadKeyFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good")
	os.WriteFile(good, []byte(GenerateKey()+"\n"), 0o600)
	key, err := LoadKeyFile(good)
	if err != nil || len(key) != KeySize {
		t.Fatalf("key=%d err=%v", len(key), err)
	}

	short := filepath.Join(dir, "short")
	os.WriteFile(short, []byte("c2hvcnQ="), 0o600)
	if _, err := LoadKeyFile(short); err == nil {
		t.Fatal("debió fallar con clave corta")
	}

	if _, err := LoadKeyFile(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("debió fallar con archivo inexistente")
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/crypto/
```

Expected: FAIL. La primera línea relevante es:

```
internal/crypto/crypto_test.go:11:32: undefined: Cipher
```

- [ ] **Step 3: Implementar**

`internal/crypto/crypto.go`:

```go
// Package crypto cifra valores con AES-256-GCM usando la clave maestra.
//
// Cada valor se cifra con un nonce aleatorio y con datos asociados (AAD) que
// lo ligan a su fila, así un valor cifrado copiado a otra fila no descifra.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

const KeySize = 32

var ErrDecrypt = errors.New("crypto: no se pudo descifrar")

type Cipher struct {
	aead cipher.AEAD
}

func New(key []byte) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("crypto: la clave debe tener %d bytes, tiene %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt devuelve nonce || ciphertext.
func (c *Cipher) Encrypt(plaintext []byte, aad string) []byte {
	nonce := make([]byte, c.aead.NonceSize())
	rand.Read(nonce)
	return c.aead.Seal(nonce, nonce, plaintext, []byte(aad))
}

func (c *Cipher) Decrypt(data []byte, aad string) ([]byte, error) {
	n := c.aead.NonceSize()
	if len(data) < n {
		return nil, ErrDecrypt
	}
	plaintext, err := c.aead.Open(nil, data[:n], data[n:], []byte(aad))
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}

// LoadKeyFile lee una clave en base64 estándar (se toleran espacios y saltos de línea).
func LoadKeyFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("crypto: leyendo la clave maestra: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("crypto: la clave maestra no es base64 válido: %w", err)
	}
	if len(key) != KeySize {
		return nil, fmt.Errorf("crypto: la clave maestra debe tener %d bytes, tiene %d", KeySize, len(key))
	}
	return key, nil
}

// GenerateKey devuelve una clave nueva en base64, lista para guardar en un archivo.
func GenerateKey() string {
	key := make([]byte, KeySize)
	rand.Read(key)
	return base64.StdEncoding.EncodeToString(key)
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/crypto/ && go test ./internal/crypto/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/crypto
```

- [ ] **Step 5: Commit**

```bash
git add internal/crypto/crypto.go internal/crypto/crypto_test.go
git commit -m "feat(crypto): AES-256-GCM con AAD y carga de la clave maestra" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 4: Reglas de nombres y error de validación

Reglas de §5.2: slugs `[a-z0-9-]` que empiezan con letra y no terminan en `-` (16 caracteres para proyecto/app/servicio, 12 para stage). El `_` queda prohibido porque es el separador de los nombres de Swarm. `*validate.Error` es el único tipo que la API traduce a 422.

**Files:**
- Create: `internal/validate/validate.go`
- Test: `internal/validate/validate_test.go`

- [ ] **Step 1: Escribir el test**

`internal/validate/validate_test.go`:

```go
package validate

import (
	"errors"
	"strings"
	"testing"
)

func TestSlugs(t *testing.T) {
	ok := []string{"a", "auditfy", "aumi-agents", "a1", "abcdefghijklmnop"}
	bad := []string{"", "1abc", "-abc", "abc-", "Auditfy", "audit_fy", "abcdefghijklmnopq", "a.b"}
	for _, s := range ok {
		if err := ProjectSlug(s); err != nil {
			t.Errorf("ProjectSlug(%q) = %v", s, err)
		}
		if err := AppSlug(s); err != nil {
			t.Errorf("AppSlug(%q) = %v", s, err)
		}
	}
	for _, s := range bad {
		if ProjectSlug(s) == nil {
			t.Errorf("ProjectSlug(%q) debió fallar", s)
		}
	}
	if StageSlug("production") != nil || StageSlug("abcdefghijkl") != nil {
		t.Error("stage válidos rechazados")
	}
	if StageSlug("abcdefghijklm") == nil {
		t.Error("stage de 13 caracteres debió fallar")
	}
}

func TestErrorType(t *testing.T) {
	err := ProjectSlug("X")
	var ve *Error
	if !errors.As(err, &ve) || ve.Field != "slug" {
		t.Fatalf("esperaba *Error con Field=slug, got %#v", err)
	}
	if !strings.HasPrefix(err.Error(), "slug: ") {
		t.Fatalf("mensaje: %q", err.Error())
	}
}

func TestUsernamePasswordEnvKeyName(t *testing.T) {
	if Username("felipe") != nil || Username("ab") == nil || Username("Felipe") == nil {
		t.Error("Username")
	}
	if Password("corta") == nil || Password("una-contraseña-larga") != nil || Password(strings.Repeat("x", 257)) == nil {
		t.Error("Password")
	}
	if EnvKey("DATABASE_URL") != nil || EnvKey("_X") != nil || EnvKey("1X") == nil || EnvKey("A-B") == nil {
		t.Error("EnvKey")
	}
	if Name("name", "Auditfy", 64) != nil || Name("name", "  ", 64) == nil || Name("name", strings.Repeat("x", 65), 64) == nil {
		t.Error("Name")
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/validate/
```

Expected: FAIL. La primera línea relevante es:

```
internal/validate/validate_test.go:13:13: undefined: ProjectSlug
```

- [ ] **Step 3: Implementar**

`internal/validate/validate.go`:

```go
// Package validate contiene las reglas de nombres y el tipo de error de
// validación que la API traduce a 422.
package validate

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Error es un error de validación de un campo concreto.
type Error struct {
	Field   string
	Message string
}

func (e *Error) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return e.Field + ": " + e.Message
}

func Errorf(field, format string, args ...any) error {
	return &Error{Field: field, Message: fmt.Sprintf(format, args...)}
}

var (
	// Empiezan con letra, terminan con letra o número, solo [a-z0-9-].
	// El "_" queda libre como separador en los nombres de Swarm (§5.2 del spec).
	slug16Re   = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,14}[a-z0-9])?$`)
	slug12Re   = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,10}[a-z0-9])?$`)
	usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,31}$`)
	envKeyRe   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
)

func ProjectSlug(s string) error { return slug("slug", s, slug16Re, 16) }
func StageSlug(s string) error   { return slug("slug", s, slug12Re, 12) }
func AppSlug(s string) error     { return slug("slug", s, slug16Re, 16) }

// VolumeName valida nombres de volúmenes y de servicios compose (mismas reglas que una app).
func VolumeName(field, s string) error { return slug(field, s, slug16Re, 16) }

func slug(field, s string, re *regexp.Regexp, max int) error {
	if !re.MatchString(s) {
		return Errorf(field, "%q no es válido: hasta %d caracteres, solo a-z, 0-9 y '-', empieza con letra y no termina en '-'", s, max)
	}
	return nil
}

func Username(s string) error {
	if !usernameRe.MatchString(s) {
		return Errorf("username", "entre 3 y 32 caracteres: a-z, 0-9, '.', '_' o '-'")
	}
	return nil
}

func Password(s string) error {
	n := utf8.RuneCountInString(s)
	if n < 12 {
		return Errorf("password", "debe tener al menos 12 caracteres")
	}
	if len(s) > 256 {
		return Errorf("password", "no puede superar 256 bytes")
	}
	return nil
}

func EnvKey(s string) error {
	if !envKeyRe.MatchString(s) {
		return Errorf("vars", "clave %q inválida: letras, números y '_', sin empezar con número, hasta 128", s)
	}
	return nil
}

// Name valida un texto libre obligatorio (nombres de proyectos, tokens).
func Name(field, s string, max int) error {
	if strings.TrimSpace(s) == "" {
		return Errorf(field, "es obligatorio")
	}
	if utf8.RuneCountInString(s) > max {
		return Errorf(field, "no puede superar %d caracteres", max)
	}
	return nil
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/validate/ && go test ./internal/validate/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/validate
```

- [ ] **Step 5: Commit**

```bash
git add internal/validate/validate.go internal/validate/validate_test.go
git commit -m "feat(validate): reglas de slugs, usuarios, contrase\u00f1as y claves de variables" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 5: Spec de las apps de formulario

El JSON del formulario de §6.1. `Parse` rechaza campos desconocidos, aplica valores por defecto (1 réplica; healthcheck 10 s / 5 s / 3 reintentos / 20 s de arranque) y valida. La ruta del healthcheck termina dentro de un comando de shell o de JS en la Fase 2, por eso solo admite caracteres que no necesitan escape. La Fase 2 usará este paquete para generar el compose.

**Files:**
- Create: `internal/appspec/appspec.go`
- Test: `internal/appspec/appspec_test.go`

- [ ] **Step 1: Escribir el test**

`internal/appspec/appspec_test.go`:

```go
package appspec

import (
	"errors"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/internal/validate"
)

func TestParseAppliesDefaults(t *testing.T) {
	s, err := Parse([]byte(`{"image":"ghcr.io/aumi-app/backend-app:staging","port":3000,"healthcheck":{"type":"http","path":"/health"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if *s.Replicas != 1 {
		t.Errorf("replicas = %d", *s.Replicas)
	}
	h := s.Healthcheck
	if h.IntervalS != 10 || h.TimeoutS != 5 || h.Retries != 3 || h.StartPeriodS != 20 {
		t.Errorf("defaults del healthcheck: %+v", h)
	}
	if s.Command == nil || s.Volumes == nil {
		t.Error("command y volumes deben normalizarse a listas vacías")
	}
}

func TestParseWithoutHealthcheckIsNone(t *testing.T) {
	s, err := Parse([]byte(`{"image":"redis:7","port":6379}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Healthcheck.Type != HealthNone || s.Healthcheck.IntervalS != 0 {
		t.Fatalf("healthcheck = %+v", s.Healthcheck)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	s, err := Parse([]byte(`{"image":"redis:7","port":6379,"replicas":0,"volumes":[{"name":"data","mount":"/data"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(s.JSON())
	if err != nil {
		t.Fatalf("el JSON normalizado debe volver a parsear: %v", err)
	}
	if *again.Replicas != 0 || again.Volumes[0].Mount != "/data" {
		t.Fatalf("got %+v", again)
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string]string{
		"vacío":                 ``,
		"null":                  `null`,
		"campo desconocido":     `{"image":"x","port":80,"foo":1}`,
		"sin imagen":            `{"port":80}`,
		"imagen con espacio":    `{"image":"x y","port":80}`,
		"puerto 0":              `{"image":"x","port":0}`,
		"puerto alto":           `{"image":"x","port":70000}`,
		"réplicas negativas":    `{"image":"x","port":80,"replicas":-1}`,
		"tipo desconocido":      `{"image":"x","port":80,"healthcheck":{"type":"tcp"}}`,
		"path sin barra":        `{"image":"x","port":80,"healthcheck":{"type":"http","path":"health"}}`,
		"path con comilla":      `{"image":"x","port":80,"healthcheck":{"type":"bun-http","path":"/h'"}}`,
		"command vacío":         `{"image":"x","port":80,"healthcheck":{"type":"command"}}`,
		"none con path":         `{"image":"x","port":80,"healthcheck":{"type":"none","path":"/h"}}`,
		"timeout alto":          `{"image":"x","port":80,"healthcheck":{"type":"http","path":"/h","timeout_s":601}}`,
		"volumen sin nombre":    `{"image":"x","port":80,"volumes":[{"name":"","mount":"/data"}]}`,
		"mount relativo":        `{"image":"x","port":80,"volumes":[{"name":"d","mount":"data"}]}`,
		"mount sucio":           `{"image":"x","port":80,"volumes":[{"name":"d","mount":"/data/"}]}`,
		"mount raíz":            `{"image":"x","port":80,"volumes":[{"name":"d","mount":"/"}]}`,
		"volumen repetido":      `{"image":"x","port":80,"volumes":[{"name":"d","mount":"/a"},{"name":"d","mount":"/b"}]}`,
		"mount repetido":        `{"image":"x","port":80,"volumes":[{"name":"a","mount":"/d"},{"name":"b","mount":"/d"}]}`,
		"memoria baja":          `{"image":"x","port":80,"resources":{"memory_mb":8}}`,
		"cpus altas":            `{"image":"x","port":80,"resources":{"cpus":1000}}`,
		"demasiados argumentos": `{"image":"x","port":80,"command":[` + strings.Repeat(`"a",`, 64) + `"a"]}`,
	}
	for name, raw := range cases {
		_, err := Parse([]byte(raw))
		var ve *validate.Error
		if !errors.As(err, &ve) {
			t.Errorf("%s: esperaba *validate.Error, got %v", name, err)
		}
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/appspec/
```

Expected: FAIL. La primera línea relevante es:

```
internal/appspec/appspec_test.go:12:12: undefined: Parse
```

- [ ] **Step 3: Implementar**

`internal/appspec/appspec.go`:

```go
// Package appspec define y valida el spec de una app de formulario (§6.1 del spec).
package appspec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/CPU-commits/muelle/internal/validate"
)

const (
	HealthHTTP    = "http"
	HealthBunHTTP = "bun-http"
	HealthCommand = "command"
	HealthNone    = "none"
)

type Spec struct {
	Image       string       `json:"image"`
	Port        int          `json:"port"`
	Replicas    *int         `json:"replicas"`
	Command     []string     `json:"command"`
	Healthcheck *Healthcheck `json:"healthcheck"`
	Volumes     []Volume     `json:"volumes"`
	Resources   *Resources   `json:"resources"`
}

type Healthcheck struct {
	Type         string   `json:"type"`
	Path         string   `json:"path,omitempty"`
	Command      []string `json:"command,omitempty"`
	IntervalS    int      `json:"interval_s,omitempty"`
	TimeoutS     int      `json:"timeout_s,omitempty"`
	Retries      int      `json:"retries,omitempty"`
	StartPeriodS int      `json:"start_period_s,omitempty"`
}

type Volume struct {
	Name  string `json:"name"`
	Mount string `json:"mount"`
}

type Resources struct {
	MemoryMB int     `json:"memory_mb,omitempty"`
	CPUs     float64 `json:"cpus,omitempty"`
}

// La ruta del healthcheck termina dentro de un comando de shell o de JS,
// así que solo se aceptan caracteres que no necesitan escape.
var healthPathRe = regexp.MustCompile(`^/[A-Za-z0-9._~/?&=%+-]*$`)

// Parse decodifica el JSON (rechazando campos desconocidos), aplica los valores
// por defecto y valida. Lo que devuelve está listo para guardarse con JSON().
func Parse(raw []byte) (Spec, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return Spec{}, validate.Errorf("spec", "es obligatorio")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var s Spec
	if err := dec.Decode(&s); err != nil {
		return Spec{}, validate.Errorf("spec", "JSON inválido: %v", err)
	}
	s.normalize()
	if err := s.Validate(); err != nil {
		return Spec{}, err
	}
	return s, nil
}

// JSON serializa el spec normalizado para guardarlo.
func (s Spec) JSON() json.RawMessage {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err) // Spec solo tiene tipos serializables
	}
	return b
}

func (s *Spec) normalize() {
	if s.Replicas == nil {
		one := 1
		s.Replicas = &one
	}
	if s.Command == nil {
		s.Command = []string{}
	}
	if s.Volumes == nil {
		s.Volumes = []Volume{}
	}
	if s.Healthcheck == nil {
		s.Healthcheck = &Healthcheck{Type: HealthNone}
	}
	h := s.Healthcheck
	if h.Type != HealthNone {
		if h.IntervalS == 0 {
			h.IntervalS = 10
		}
		if h.TimeoutS == 0 {
			h.TimeoutS = 5
		}
		if h.Retries == 0 {
			h.Retries = 3
		}
		if h.StartPeriodS == 0 {
			h.StartPeriodS = 20
		}
	}
}

func (s Spec) Validate() error {
	if s.Image == "" || len(s.Image) > 512 || strings.ContainsAny(s.Image, " \t\r\n") {
		return validate.Errorf("image", "es obligatoria, sin espacios y de hasta 512 caracteres")
	}
	if s.Port < 1 || s.Port > 65535 {
		return validate.Errorf("port", "debe estar entre 1 y 65535")
	}
	if s.Replicas == nil || *s.Replicas < 0 || *s.Replicas > 50 {
		return validate.Errorf("replicas", "debe estar entre 0 y 50")
	}
	if len(s.Command) > 64 {
		return validate.Errorf("command", "máximo 64 argumentos")
	}
	if err := s.Healthcheck.validate(); err != nil {
		return err
	}
	if err := validateVolumes(s.Volumes); err != nil {
		return err
	}
	if r := s.Resources; r != nil {
		if r.MemoryMB != 0 && (r.MemoryMB < 16 || r.MemoryMB > 1<<20) {
			return validate.Errorf("resources.memory_mb", "debe estar entre 16 y 1048576")
		}
		if r.CPUs != 0 && (r.CPUs < 0.01 || r.CPUs > 256) {
			return validate.Errorf("resources.cpus", "debe estar entre 0.01 y 256")
		}
	}
	return nil
}

func (h *Healthcheck) validate() error {
	if h == nil {
		return validate.Errorf("healthcheck", "es obligatorio")
	}
	switch h.Type {
	case HealthNone:
		if h.Path != "" || len(h.Command) > 0 {
			return validate.Errorf("healthcheck", "con type none no lleva path ni command")
		}
		return nil
	case HealthHTTP, HealthBunHTTP:
		if !healthPathRe.MatchString(h.Path) || len(h.Path) > 256 {
			return validate.Errorf("healthcheck.path", "debe empezar con / y usar solo letras, números y ._~/?&=%%+-")
		}
		if len(h.Command) > 0 {
			return validate.Errorf("healthcheck.command", "solo aplica a type command")
		}
	case HealthCommand:
		if len(h.Command) == 0 || len(h.Command) > 64 {
			return validate.Errorf("healthcheck.command", "es obligatorio con type command (máximo 64 argumentos)")
		}
		if h.Path != "" {
			return validate.Errorf("healthcheck.path", "solo aplica a type http o bun-http")
		}
	default:
		return validate.Errorf("healthcheck.type", "debe ser http, bun-http, command o none")
	}
	switch {
	case h.IntervalS < 1 || h.IntervalS > 3600:
		return validate.Errorf("healthcheck.interval_s", "debe estar entre 1 y 3600")
	case h.TimeoutS < 1 || h.TimeoutS > 600:
		return validate.Errorf("healthcheck.timeout_s", "debe estar entre 1 y 600")
	case h.Retries < 1 || h.Retries > 20:
		return validate.Errorf("healthcheck.retries", "debe estar entre 1 y 20")
	case h.StartPeriodS < 0 || h.StartPeriodS > 3600:
		return validate.Errorf("healthcheck.start_period_s", "debe estar entre 0 y 3600")
	}
	return nil
}

func validateVolumes(vols []Volume) error {
	if len(vols) > 10 {
		return validate.Errorf("volumes", "máximo 10")
	}
	names := map[string]bool{}
	mounts := map[string]bool{}
	for i, v := range vols {
		field := fmt.Sprintf("volumes[%d]", i)
		if err := validate.VolumeName(field+".name", v.Name); err != nil {
			return err
		}
		if !strings.HasPrefix(v.Mount, "/") || path.Clean(v.Mount) != v.Mount || v.Mount == "/" {
			return validate.Errorf(field+".mount", "debe ser una ruta absoluta limpia distinta de /")
		}
		if names[v.Name] {
			return validate.Errorf(field+".name", "%q está repetido", v.Name)
		}
		if mounts[v.Mount] {
			return validate.Errorf(field+".mount", "%q está repetido", v.Mount)
		}
		names[v.Name] = true
		mounts[v.Mount] = true
	}
	return nil
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/appspec/ && go test ./internal/appspec/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/appspec
```

- [ ] **Step 5: Commit**

```bash
git add internal/appspec/appspec.go internal/appspec/appspec_test.go
git commit -m "feat(appspec): parseo, defaults y validaci\u00f3n del spec de formulario" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 6: Store: apertura de SQLite y migraciones

SQLite con `modernc.org/sqlite` (Go puro, binario estático sin CGO). El DSN activa WAL, `foreign_keys`, `busy_timeout` y transacciones `IMMEDIATE` (los escritores esperan en vez de fallar con `SQLITE_BUSY`). `Store.Now` es el reloj: los tests lo reemplazan. Todas las marcas de tiempo se guardan como milisegundos Unix. La migración 0001 crea **todas** las tablas de la Fase 1; las fases siguientes agregan `0002_…sql`, etc.

**Files:**
- Create: `internal/store/migrations/0001_init.sql`
- Create: `internal/store/store.go`
- Test: `internal/store/store_test.go`

- [ ] **Step 1: Escribir el test**

`internal/store/store_test.go`:

```go
package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// newTestStore abre un SQLite nuevo en un directorio temporal, con migraciones
// aplicadas y un reloj fijo que el test puede mover con clock.advance.
func newTestStore(t *testing.T) (*Store, *testClock) {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	clock := &testClock{now: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
	st.Now = clock.Now
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return st, clock
}

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time          { return c.now }
func (c *testClock) advance(d time.Duration) { c.now = c.now.Add(d) }

func TestMigrateIsIdempotent(t *testing.T) {
	st, _ := newTestStore(t)
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("segunda migración: %v", err)
	}
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("schema_migrations tiene %d filas, esperaba 1", n)
	}
}

func TestForeignKeysAreOn(t *testing.T) {
	st, _ := newTestStore(t)
	_, err := st.db.Exec(`INSERT INTO stages (project_id, slug, created_at) VALUES (999, 'x', 0)`)
	if err == nil {
		t.Fatal("foreign_keys debería rechazar un project_id inexistente")
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/store/
```

Expected: FAIL. La primera línea relevante es:

```
internal/store/store_test.go:12:35: undefined: Store
```

- [ ] **Step 3: Implementar**

`internal/store/migrations/0001_init.sql`:

```sql
-- Todas las marcas de tiempo son milisegundos Unix (UTC).

CREATE TABLE projects (
  id         INTEGER PRIMARY KEY,
  slug       TEXT    NOT NULL UNIQUE,
  name       TEXT    NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE stages (
  id         INTEGER PRIMARY KEY,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
  slug       TEXT    NOT NULL,
  created_at INTEGER NOT NULL,
  UNIQUE (project_id, slug)
);

CREATE TABLE apps (
  id                INTEGER PRIMARY KEY,
  stage_id          INTEGER NOT NULL REFERENCES stages(id) ON DELETE RESTRICT,
  slug              TEXT    NOT NULL,
  kind              TEXT    NOT NULL CHECK (kind IN ('service', 'compose')),
  spec              TEXT,
  compose_yaml      TEXT,
  deploy_timeout_s  INTEGER NOT NULL DEFAULT 300,
  storage_node_id   TEXT,
  config_changed_at INTEGER NOT NULL,
  last_deployed_at  INTEGER,
  created_at        INTEGER NOT NULL,
  UNIQUE (stage_id, slug),
  CHECK ((kind = 'service' AND spec IS NOT NULL AND compose_yaml IS NULL)
      OR (kind = 'compose' AND compose_yaml IS NOT NULL AND spec IS NULL))
);

-- Exactamente uno de stage_id / app_id. El valor siempre va cifrado.
CREATE TABLE env_vars (
  id         INTEGER PRIMARY KEY,
  stage_id   INTEGER REFERENCES stages(id) ON DELETE CASCADE,
  app_id     INTEGER REFERENCES apps(id) ON DELETE CASCADE,
  key        TEXT    NOT NULL,
  value      BLOB    NOT NULL,
  secret     INTEGER NOT NULL DEFAULT 1,
  updated_at INTEGER NOT NULL,
  CHECK ((stage_id IS NULL) <> (app_id IS NULL))
);
CREATE UNIQUE INDEX env_vars_stage_key ON env_vars(stage_id, key) WHERE stage_id IS NOT NULL;
CREATE UNIQUE INDEX env_vars_app_key ON env_vars(app_id, key) WHERE app_id IS NOT NULL;

CREATE TABLE users (
  id            INTEGER PRIMARY KEY,
  username      TEXT    NOT NULL UNIQUE,
  password_hash TEXT    NOT NULL,
  totp_secret   BLOB,
  totp_pending  BLOB,
  created_by    INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at    INTEGER NOT NULL,
  failed_logins INTEGER NOT NULL DEFAULT 0,
  locked_until  INTEGER
);

-- created_by NULL = invitación de setup creada por el sistema.
CREATE TABLE invites (
  id         INTEGER PRIMARY KEY,
  token_hash TEXT    NOT NULL UNIQUE,
  created_by INTEGER REFERENCES users(id) ON DELETE CASCADE,
  expires_at INTEGER NOT NULL,
  used_at    INTEGER
);

CREATE TABLE sessions (
  id_hash      TEXT    PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  mfa_pending  INTEGER NOT NULL DEFAULT 0,
  created_at   INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL,
  expires_at   INTEGER NOT NULL,
  user_agent   TEXT    NOT NULL DEFAULT '',
  ip           TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user ON sessions(user_id);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY,
  name         TEXT    NOT NULL,
  token_hash   TEXT    NOT NULL UNIQUE,
  project_id   INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  stage_id     INTEGER REFERENCES stages(id) ON DELETE CASCADE,
  created_by   INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at   INTEGER NOT NULL,
  last_used_at INTEGER,
  expires_at   INTEGER
);

CREATE TABLE audit_events (
  id         INTEGER PRIMARY KEY,
  actor_type TEXT    NOT NULL,
  actor_id   INTEGER,
  action     TEXT    NOT NULL,
  target     TEXT    NOT NULL,
  details    TEXT    NOT NULL DEFAULT '{}',
  at         INTEGER NOT NULL
);
CREATE INDEX audit_events_at ON audit_events(at);

CREATE TABLE settings (
  key       TEXT    PRIMARY KEY,
  value     BLOB    NOT NULL,
  encrypted INTEGER NOT NULL DEFAULT 0
);
```

`internal/store/store.go`:

```go
// Package store es la capa de persistencia en SQLite. No cifra ni valida:
// recibe los valores ya cifrados y validados.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var (
	ErrNotFound = errors.New("store: no encontrado")
	ErrConflict = errors.New("store: ya existe")
	ErrNotEmpty = errors.New("store: tiene elementos dependientes")
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Store struct {
	db *sql.DB
	// Now es el reloj del store. Los tests lo reemplazan para controlar el tiempo.
	Now func() time.Time
}

func Open(path string) (*Store, error) {
	dsn := "file:" + path +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: abriendo %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: abriendo %s: %w", path, err)
	}
	return &Store{db: db, Now: time.Now}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Migrate aplica en orden las migraciones embebidas que aún no se aplicaron.
// Cada archivo se llama NNNN_descripcion.sql y corre en su propia transacción.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("store: creando schema_migrations: %w", err)
	}
	var current int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	type migration struct {
		version int
		name    string
	}
	var migs []migration
	for _, e := range entries {
		prefix, _, ok := strings.Cut(e.Name(), "_")
		v, err := strconv.Atoi(prefix)
		if !ok || err != nil {
			return fmt.Errorf("store: la migración %s no empieza con NNNN_", e.Name())
		}
		migs = append(migs, migration{v, e.Name()})
	}
	sort.Slice(migs, func(i, j int) bool { return migs[i].version < migs[j].version })

	for _, m := range migs {
		if m.version <= current {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + m.name)
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: migración %s: %w", m.name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, m.version, ms(s.Now())); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func isUnique(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func ms(t time.Time) int64     { return t.UnixMilli() }
func fromMS(v int64) time.Time { return time.UnixMilli(v).UTC() }

func fromNullMS(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := fromMS(v.Int64)
	return &t
}

func nullMS(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: ms(*t), Valid: true}
}

func nullInt(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

func fromNullInt(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}
```

Agregar las dependencias:

```bash
go get modernc.org/sqlite@v1.60.1
go mod tidy
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/store/ && go test ./internal/store/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/store
```

- [ ] **Step 5: Commit**

```bash
git add internal/store/migrations/0001_init.sql internal/store/store.go internal/store/store_test.go go.mod go.sum
git commit -m "feat(store): SQLite con migraciones embebidas" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 7: Store: proyectos

CRUD de proyectos. Borrar un proyecto con stages devuelve `ErrNotEmpty`; ese caso se prueba en la tarea de stages.

**Files:**
- Create: `internal/store/projects.go`
- Test: `internal/store/projects_test.go`

- [ ] **Step 1: Escribir el test**

`internal/store/projects_test.go`:

```go
package store

import (
	"context"
	"errors"
	"testing"
)

func TestProjectsCRUD(t *testing.T) {
	st, _ := newTestStore(t)
	ctx := context.Background()

	p, err := st.CreateProject(ctx, "auditfy", "Auditfy")
	if err != nil {
		t.Fatal(err)
	}
	if p.ID == 0 || p.Slug != "auditfy" || p.CreatedAt.IsZero() {
		t.Fatalf("got %+v", p)
	}
	if _, err := st.CreateProject(ctx, "auditfy", "Otro"); !errors.Is(err, ErrConflict) {
		t.Fatalf("esperaba ErrConflict, got %v", err)
	}
	if _, err := st.CreateProject(ctx, "beta", "Beta"); err != nil {
		t.Fatal(err)
	}

	list, err := st.ListProjects(ctx)
	if err != nil || len(list) != 2 || list[0].Slug != "auditfy" {
		t.Fatalf("list=%+v err=%v", list, err)
	}

	p, err = st.UpdateProjectName(ctx, "auditfy", "Auditfy SpA")
	if err != nil || p.Name != "Auditfy SpA" {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	if _, err := st.UpdateProjectName(ctx, "nope", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}

	if err := st.DeleteProject(ctx, "beta"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetProject(ctx, "beta"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
	if err := st.DeleteProject(ctx, "beta"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/store/
```

Expected: FAIL. La primera línea relevante es:

```
internal/store/projects_test.go:13:15: st.CreateProject undefined (type *Store has no field or method CreateProject)
```

- [ ] **Step 3: Implementar**

`internal/store/projects.go`:

```go
package store

import (
	"context"
	"time"
)

type Project struct {
	ID        int64     `json:"-"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) CreateProject(ctx context.Context, slug, name string) (Project, error) {
	now := ms(s.Now())
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO projects (slug, name, created_at) VALUES (?, ?, ?)`, slug, name, now)
	if isUnique(err) {
		return Project{}, ErrConflict
	}
	if err != nil {
		return Project{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Project{}, err
	}
	return Project{ID: id, Slug: slug, Name: name, CreatedAt: fromMS(now)}, nil
}

func (s *Store) GetProject(ctx context.Context, slug string) (Project, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, slug, name, created_at FROM projects WHERE slug = ?`, slug)
	p, err := scanProject(row)
	return p, notFound(err)
}

func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, slug, name, created_at FROM projects ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpdateProjectName(ctx context.Context, slug, name string) (Project, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE projects SET name = ? WHERE slug = ?`, name, slug)
	if err != nil {
		return Project{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Project{}, ErrNotFound
	}
	return s.GetProject(ctx, slug)
}

// DeleteProject borra un proyecto sin stages. Con stages devuelve ErrNotEmpty.
func (s *Store) DeleteProject(ctx context.Context, slug string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM projects WHERE slug = ?`, slug).Scan(&id); err != nil {
		return notFound(err)
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM stages WHERE project_id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrNotEmpty
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func scanProject(sc scanner) (Project, error) {
	var p Project
	var created int64
	if err := sc.Scan(&p.ID, &p.Slug, &p.Name, &created); err != nil {
		return Project{}, err
	}
	p.CreatedAt = fromMS(created)
	return p, nil
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/store/ && go test ./internal/store/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/store
```

- [ ] **Step 5: Commit**

```bash
git add internal/store/projects.go internal/store/projects_test.go
git commit -m "feat(store): proyectos" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 8: Store: stages

Un stage pertenece a un proyecto y su slug es único dentro del proyecto. Borrar un stage con apps devuelve `ErrNotEmpty` (se prueba en la tarea de apps).

**Files:**
- Create: `internal/store/stages.go`
- Test: `internal/store/stages_test.go`

- [ ] **Step 1: Escribir el test**

`internal/store/stages_test.go`:

```go
package store

import (
	"context"
	"errors"
	"testing"
)

func TestStagesCRUD(t *testing.T) {
	st, _ := newTestStore(t)
	ctx := context.Background()

	if _, err := st.CreateStage(ctx, "nope", "staging"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("proyecto inexistente: esperaba ErrNotFound, got %v", err)
	}
	st.CreateProject(ctx, "auditfy", "Auditfy")
	st.CreateProject(ctx, "beta", "Beta")

	s, err := st.CreateStage(ctx, "auditfy", "staging")
	if err != nil || s.ProjectSlug != "auditfy" || s.Slug != "staging" {
		t.Fatalf("s=%+v err=%v", s, err)
	}
	if _, err := st.CreateStage(ctx, "auditfy", "staging"); !errors.Is(err, ErrConflict) {
		t.Fatalf("esperaba ErrConflict, got %v", err)
	}
	// El mismo slug en otro proyecto sí se permite.
	if _, err := st.CreateStage(ctx, "beta", "staging"); err != nil {
		t.Fatal(err)
	}
	st.CreateStage(ctx, "auditfy", "production")

	list, err := st.ListStages(ctx, "auditfy")
	if err != nil || len(list) != 2 || list[0].Slug != "production" {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if _, err := st.ListStages(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}

	got, err := st.GetStage(ctx, "auditfy", "staging")
	if err != nil || got.ID != s.ID {
		t.Fatalf("got=%+v err=%v", got, err)
	}

	if err := st.DeleteStage(ctx, "auditfy", "production"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteStage(ctx, "auditfy", "production"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
}

func TestDeleteProjectWithStagesFails(t *testing.T) {
	st, _ := newTestStore(t)
	ctx := context.Background()
	st.CreateProject(ctx, "auditfy", "Auditfy")
	if _, err := st.CreateStage(ctx, "auditfy", "staging"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteProject(ctx, "auditfy"); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("esperaba ErrNotEmpty, got %v", err)
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/store/
```

Expected: FAIL. La primera línea relevante es:

```
internal/store/stages_test.go:13:18: st.CreateStage undefined (type *Store has no field or method CreateStage)
```

- [ ] **Step 3: Implementar**

`internal/store/stages.go`:

```go
package store

import (
	"context"
	"time"
)

type Stage struct {
	ID          int64     `json:"-"`
	ProjectID   int64     `json:"-"`
	ProjectSlug string    `json:"project"`
	Slug        string    `json:"slug"`
	CreatedAt   time.Time `json:"created_at"`
}

const stageSelect = `SELECT s.id, s.project_id, p.slug, s.slug, s.created_at
	FROM stages s JOIN projects p ON p.id = s.project_id`

func (s *Store) CreateStage(ctx context.Context, projectSlug, slug string) (Stage, error) {
	p, err := s.GetProject(ctx, projectSlug)
	if err != nil {
		return Stage{}, err
	}
	now := ms(s.Now())
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO stages (project_id, slug, created_at) VALUES (?, ?, ?)`, p.ID, slug, now)
	if isUnique(err) {
		return Stage{}, ErrConflict
	}
	if err != nil {
		return Stage{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Stage{}, err
	}
	return Stage{ID: id, ProjectID: p.ID, ProjectSlug: p.Slug, Slug: slug, CreatedAt: fromMS(now)}, nil
}

func (s *Store) GetStage(ctx context.Context, projectSlug, slug string) (Stage, error) {
	row := s.db.QueryRowContext(ctx, stageSelect+` WHERE p.slug = ? AND s.slug = ?`, projectSlug, slug)
	st, err := scanStage(row)
	return st, notFound(err)
}

func (s *Store) ListStages(ctx context.Context, projectSlug string) ([]Stage, error) {
	if _, err := s.GetProject(ctx, projectSlug); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, stageSelect+` WHERE p.slug = ? ORDER BY s.slug`, projectSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Stage{}
	for rows.Next() {
		st, err := scanStage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// DeleteStage borra un stage sin apps (sus variables y tokens se borran en cascada).
func (s *Store) DeleteStage(ctx context.Context, projectSlug, slug string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx,
		`SELECT s.id FROM stages s JOIN projects p ON p.id = s.project_id WHERE p.slug = ? AND s.slug = ?`,
		projectSlug, slug).Scan(&id)
	if err != nil {
		return notFound(err)
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM apps WHERE stage_id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrNotEmpty
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM stages WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func scanStage(sc scanner) (Stage, error) {
	var st Stage
	var created int64
	if err := sc.Scan(&st.ID, &st.ProjectID, &st.ProjectSlug, &st.Slug, &created); err != nil {
		return Stage{}, err
	}
	st.CreatedAt = fromMS(created)
	return st, nil
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/store/ && go test ./internal/store/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/store
```

- [ ] **Step 5: Commit**

```bash
git add internal/store/stages.go internal/store/stages_test.go
git commit -m "feat(store): stages" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 9: Store: apps

Una app es `service` (con `spec` JSON) o `compose` (con `compose_yaml`); un `CHECK` de la tabla garantiza que tenga solo el campo de su tipo. Cambiar spec o compose actualiza `config_changed_at`; `PendingChanges()` lo compara con `last_deployed_at`, que llenará la Fase 2 (§5.2, regla 4). El store no valida el contenido: eso lo hace la API con `appspec`.

**Files:**
- Create: `internal/store/apps.go`
- Test: `internal/store/apps_test.go`

- [ ] **Step 1: Escribir el test**

`internal/store/apps_test.go`:

```go
package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func seedStage(t *testing.T, st *Store) Stage {
	t.Helper()
	ctx := context.Background()
	if _, err := st.CreateProject(ctx, "auditfy", "Auditfy"); err != nil {
		t.Fatal(err)
	}
	s, err := st.CreateStage(ctx, "auditfy", "staging")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAppsCRUD(t *testing.T) {
	st, clock := newTestStore(t)
	ctx := context.Background()
	seedStage(t, st)

	api, err := st.CreateApp(ctx, NewApp{ProjectSlug: "auditfy", StageSlug: "staging", Slug: "api",
		Kind: KindService, Spec: json.RawMessage(`{"image":"x"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if api.Kind != KindService || string(api.Spec) != `{"image":"x"}` || api.DeployTimeoutS != DefaultDeployTimeoutS {
		t.Fatalf("api=%+v", api)
	}
	if api.ProjectSlug != "auditfy" || api.StageSlug != "staging" || api.LastDeployedAt != nil {
		t.Fatalf("api=%+v", api)
	}
	if _, err := st.CreateApp(ctx, NewApp{ProjectSlug: "auditfy", StageSlug: "staging", Slug: "api",
		Kind: KindService, Spec: json.RawMessage(`{}`)}); !errors.Is(err, ErrConflict) {
		t.Fatalf("esperaba ErrConflict, got %v", err)
	}
	nats, err := st.CreateApp(ctx, NewApp{ProjectSlug: "auditfy", StageSlug: "staging", Slug: "nats",
		Kind: KindCompose, ComposeYAML: "services: {}\n", DeployTimeoutS: 60})
	if err != nil || nats.ComposeYAML != "services: {}\n" || nats.Spec != nil || nats.DeployTimeoutS != 60 {
		t.Fatalf("nats=%+v err=%v", nats, err)
	}

	list, err := st.ListApps(ctx, "auditfy", "staging")
	if err != nil || len(list) != 2 || list[0].Slug != "api" {
		t.Fatalf("list=%+v err=%v", list, err)
	}

	// Cambiar el spec mueve config_changed_at.
	clock.advance(time.Minute)
	newSpec := json.RawMessage(`{"image":"y"}`)
	api2, err := st.UpdateApp(ctx, "auditfy", "staging", "api", AppUpdate{Spec: &newSpec})
	if err != nil || string(api2.Spec) != `{"image":"y"}` || !api2.ConfigChangedAt.After(api.ConfigChangedAt) {
		t.Fatalf("api2=%+v err=%v", api2, err)
	}
	// Cambiar solo el timeout no.
	clock.advance(time.Minute)
	timeout := 120
	api3, err := st.UpdateApp(ctx, "auditfy", "staging", "api", AppUpdate{DeployTimeoutS: &timeout})
	if err != nil || api3.DeployTimeoutS != 120 || !api3.ConfigChangedAt.Equal(api2.ConfigChangedAt) {
		t.Fatalf("api3=%+v err=%v", api3, err)
	}
	// Campo de otro tipo.
	yaml := "services: {}"
	if _, err := st.UpdateApp(ctx, "auditfy", "staging", "api", AppUpdate{ComposeYAML: &yaml}); !errors.Is(err, ErrKindMismatch) {
		t.Fatalf("esperaba ErrKindMismatch, got %v", err)
	}

	if err := st.DeleteApp(ctx, "auditfy", "staging", "nats"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetApp(ctx, "auditfy", "staging", "nats"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
}

func TestPendingChanges(t *testing.T) {
	deployed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := App{ConfigChangedAt: deployed.Add(-time.Second)}
	if a.PendingChanges() {
		t.Error("sin deploy no hay cambios pendientes")
	}
	a.LastDeployedAt = &deployed
	if a.PendingChanges() {
		t.Error("config anterior al deploy no es pendiente")
	}
	a.ConfigChangedAt = deployed.Add(time.Millisecond)
	if !a.PendingChanges() {
		t.Error("config posterior al deploy es pendiente")
	}
}

func TestDeleteStageWithAppsFails(t *testing.T) {
	st, _ := newTestStore(t)
	ctx := context.Background()
	st.CreateProject(ctx, "auditfy", "Auditfy")
	st.CreateStage(ctx, "auditfy", "staging")
	_, err := st.CreateApp(ctx, NewApp{ProjectSlug: "auditfy", StageSlug: "staging", Slug: "api",
		Kind: KindService, Spec: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteStage(ctx, "auditfy", "staging"); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("esperaba ErrNotEmpty, got %v", err)
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/store/
```

Expected: FAIL. La primera línea relevante es:

```
internal/store/apps_test.go:29:17: st.CreateApp undefined (type *Store has no field or method CreateApp)
```

- [ ] **Step 3: Implementar**

`internal/store/apps.go`:

```go
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	KindService = "service"
	KindCompose = "compose"

	DefaultDeployTimeoutS = 300
)

var ErrKindMismatch = errors.New("store: el campo no corresponde al tipo de app")

type App struct {
	ID              int64           `json:"-"`
	StageID         int64           `json:"-"`
	ProjectSlug     string          `json:"project"`
	StageSlug       string          `json:"stage"`
	Slug            string          `json:"slug"`
	Kind            string          `json:"kind"`
	Spec            json.RawMessage `json:"spec,omitempty"`
	ComposeYAML     string          `json:"compose_yaml,omitempty"`
	DeployTimeoutS  int             `json:"deploy_timeout_s"`
	StorageNodeID   string          `json:"-"`
	ConfigChangedAt time.Time       `json:"config_changed_at"`
	LastDeployedAt  *time.Time      `json:"last_deployed_at"`
	CreatedAt       time.Time       `json:"created_at"`
}

// PendingChanges indica si la configuración cambió después del último deploy.
// Una app que nunca se desplegó no tiene "cambios pendientes": no tiene deploy.
func (a App) PendingChanges() bool {
	return a.LastDeployedAt != nil && a.ConfigChangedAt.After(*a.LastDeployedAt)
}

type NewApp struct {
	ProjectSlug    string
	StageSlug      string
	Slug           string
	Kind           string
	Spec           json.RawMessage // solo KindService, ya validado
	ComposeYAML    string          // solo KindCompose
	DeployTimeoutS int             // 0 = DefaultDeployTimeoutS
}

// AppUpdate: los campos nil no se tocan.
type AppUpdate struct {
	Spec           *json.RawMessage
	ComposeYAML    *string
	DeployTimeoutS *int
}

const appSelect = `SELECT a.id, a.stage_id, p.slug, s.slug, a.slug, a.kind, a.spec, a.compose_yaml,
	a.deploy_timeout_s, a.storage_node_id, a.config_changed_at, a.last_deployed_at, a.created_at
	FROM apps a JOIN stages s ON s.id = a.stage_id JOIN projects p ON p.id = s.project_id`

func (s *Store) CreateApp(ctx context.Context, n NewApp) (App, error) {
	st, err := s.GetStage(ctx, n.ProjectSlug, n.StageSlug)
	if err != nil {
		return App{}, err
	}
	var spec, compose sql.NullString
	switch n.Kind {
	case KindService:
		spec = sql.NullString{String: string(n.Spec), Valid: true}
	case KindCompose:
		compose = sql.NullString{String: n.ComposeYAML, Valid: true}
	default:
		return App{}, fmt.Errorf("store: kind inválido %q", n.Kind)
	}
	timeout := n.DeployTimeoutS
	if timeout == 0 {
		timeout = DefaultDeployTimeoutS
	}
	now := ms(s.Now())
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO apps (stage_id, slug, kind, spec, compose_yaml, deploy_timeout_s, config_changed_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		st.ID, n.Slug, n.Kind, spec, compose, timeout, now, now)
	if isUnique(err) {
		return App{}, ErrConflict
	}
	if err != nil {
		return App{}, err
	}
	return s.GetApp(ctx, n.ProjectSlug, n.StageSlug, n.Slug)
}

func (s *Store) GetApp(ctx context.Context, projectSlug, stageSlug, slug string) (App, error) {
	row := s.db.QueryRowContext(ctx, appSelect+` WHERE p.slug = ? AND s.slug = ? AND a.slug = ?`,
		projectSlug, stageSlug, slug)
	a, err := scanApp(row)
	return a, notFound(err)
}

func (s *Store) ListApps(ctx context.Context, projectSlug, stageSlug string) ([]App, error) {
	if _, err := s.GetStage(ctx, projectSlug, stageSlug); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, appSelect+` WHERE p.slug = ? AND s.slug = ? ORDER BY a.slug`,
		projectSlug, stageSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []App{}
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdateApp aplica los campos no nil. Cambiar spec o compose marca cambios pendientes.
func (s *Store) UpdateApp(ctx context.Context, projectSlug, stageSlug, slug string, u AppUpdate) (App, error) {
	app, err := s.GetApp(ctx, projectSlug, stageSlug, slug)
	if err != nil {
		return App{}, err
	}
	var sets []string
	var args []any
	configChanged := false
	if u.Spec != nil {
		if app.Kind != KindService {
			return App{}, ErrKindMismatch
		}
		sets, args = append(sets, "spec = ?"), append(args, string(*u.Spec))
		configChanged = true
	}
	if u.ComposeYAML != nil {
		if app.Kind != KindCompose {
			return App{}, ErrKindMismatch
		}
		sets, args = append(sets, "compose_yaml = ?"), append(args, *u.ComposeYAML)
		configChanged = true
	}
	if u.DeployTimeoutS != nil {
		sets, args = append(sets, "deploy_timeout_s = ?"), append(args, *u.DeployTimeoutS)
	}
	if len(sets) == 0 {
		return app, nil
	}
	if configChanged {
		sets, args = append(sets, "config_changed_at = ?"), append(args, ms(s.Now()))
	}
	args = append(args, app.ID)
	if _, err := s.db.ExecContext(ctx, `UPDATE apps SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return App{}, err
	}
	return s.GetApp(ctx, projectSlug, stageSlug, slug)
}

// DeleteApp borra la app y, en cascada, sus variables.
func (s *Store) DeleteApp(ctx context.Context, projectSlug, stageSlug, slug string) error {
	app, err := s.GetApp(ctx, projectSlug, stageSlug, slug)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM apps WHERE id = ?`, app.ID)
	return err
}

func scanApp(sc scanner) (App, error) {
	var a App
	var spec, compose, node sql.NullString
	var changed, created int64
	var deployed sql.NullInt64
	err := sc.Scan(&a.ID, &a.StageID, &a.ProjectSlug, &a.StageSlug, &a.Slug, &a.Kind, &spec, &compose,
		&a.DeployTimeoutS, &node, &changed, &deployed, &created)
	if err != nil {
		return App{}, err
	}
	if spec.Valid {
		a.Spec = json.RawMessage(spec.String)
	}
	a.ComposeYAML = compose.String
	a.StorageNodeID = node.String
	a.ConfigChangedAt = fromMS(changed)
	a.LastDeployedAt = fromNullMS(deployed)
	a.CreatedAt = fromMS(created)
	return a, nil
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/store/ && go test ./internal/store/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/store
```

- [ ] **Step 5: Commit**

```bash
git add internal/store/apps.go internal/store/apps_test.go
git commit -m "feat(store): apps de formulario y compose" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 10: Store: variables de entorno

Las variables pertenecen a un stage o a una app (`EnvOwner`). El store recibe los valores **ya cifrados**. `ReplaceEnv`/`PatchEnv` actualizan en la misma transacción el `config_changed_at` de las apps afectadas: la app dueña, o todas las apps del stage. `AAD()` es el texto que se usa al cifrar cada valor.

**Files:**
- Create: `internal/store/envvars.go`
- Test: `internal/store/envvars_test.go`

- [ ] **Step 1: Escribir el test**

`internal/store/envvars_test.go`:

```go
package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func seedApp(t *testing.T, st *Store, slug string) App {
	t.Helper()
	a, err := st.CreateApp(context.Background(), NewApp{ProjectSlug: "auditfy", StageSlug: "staging", Slug: slug,
		Kind: KindService, Spec: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func keys(vars []EnvVar) []string {
	out := []string{}
	for _, v := range vars {
		out = append(out, v.Key)
	}
	return out
}

func TestEnvOwnerAAD(t *testing.T) {
	if got := AppOwner(3).AAD("K"); got != "env:app:3:K" {
		t.Errorf("got %q", got)
	}
	if got := StageOwner(4).AAD("K"); got != "env:stage:4:K" {
		t.Errorf("got %q", got)
	}
}

func TestReplaceAndPatchEnv(t *testing.T) {
	st, clock := newTestStore(t)
	ctx := context.Background()
	seedStage(t, st)
	api := seedApp(t, st, "api")
	o := AppOwner(api.ID)

	err := st.ReplaceEnv(ctx, o, []EnvVar{
		{Key: "B", Value: []byte("cb"), Secret: true},
		{Key: "A", Value: []byte("ca"), Secret: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	vars, err := st.ListEnv(ctx, o)
	if err != nil || len(vars) != 2 || vars[0].Key != "A" || vars[0].Secret || !vars[1].Secret || string(vars[1].Value) != "cb" {
		t.Fatalf("vars=%+v err=%v", vars, err)
	}

	// Replace borra lo que no viene.
	if err := st.ReplaceEnv(ctx, o, []EnvVar{{Key: "C", Value: []byte("cc")}}); err != nil {
		t.Fatal(err)
	}
	vars, _ = st.ListEnv(ctx, o)
	if got := keys(vars); len(got) != 1 || got[0] != "C" {
		t.Fatalf("keys=%v", got)
	}

	// Patch: agrega D, reemplaza C, y borra una clave inexistente sin fallar.
	if err := st.PatchEnv(ctx, o, []EnvVar{{Key: "C", Value: []byte("c2")}, {Key: "D", Value: []byte("cd")}}, []string{"Z"}); err != nil {
		t.Fatal(err)
	}
	if err := st.PatchEnv(ctx, o, nil, []string{"D"}); err != nil {
		t.Fatal(err)
	}
	vars, _ = st.ListEnv(ctx, o)
	if len(vars) != 1 || vars[0].Key != "C" || string(vars[0].Value) != "c2" || !vars[0].UpdatedAt.Equal(clock.now) {
		t.Fatalf("vars=%+v", vars)
	}
}

func TestEnvMarksAppsPending(t *testing.T) {
	st, clock := newTestStore(t)
	ctx := context.Background()
	stage := seedStage(t, st)
	api := seedApp(t, st, "api")
	worker := seedApp(t, st, "worker")

	clock.advance(time.Minute)
	if err := st.ReplaceEnv(ctx, AppOwner(api.ID), []EnvVar{{Key: "A", Value: []byte("x")}}); err != nil {
		t.Fatal(err)
	}
	a, _ := st.GetApp(ctx, "auditfy", "staging", "api")
	w, _ := st.GetApp(ctx, "auditfy", "staging", "worker")
	if !a.ConfigChangedAt.After(api.ConfigChangedAt) || !w.ConfigChangedAt.Equal(worker.ConfigChangedAt) {
		t.Fatal("cambiar variables de una app solo marca esa app")
	}

	clock.advance(time.Minute)
	if err := st.PatchEnv(ctx, StageOwner(stage.ID), []EnvVar{{Key: "S", Value: []byte("x")}}, nil); err != nil {
		t.Fatal(err)
	}
	a2, _ := st.GetApp(ctx, "auditfy", "staging", "api")
	w2, _ := st.GetApp(ctx, "auditfy", "staging", "worker")
	if !a2.ConfigChangedAt.After(a.ConfigChangedAt) || !w2.ConfigChangedAt.After(w.ConfigChangedAt) {
		t.Fatal("cambiar variables del stage marca todas sus apps")
	}
}

func TestEnvIsolatedPerOwnerAndCascades(t *testing.T) {
	st, _ := newTestStore(t)
	ctx := context.Background()
	stage := seedStage(t, st)
	api := seedApp(t, st, "api")
	st.ReplaceEnv(ctx, StageOwner(stage.ID), []EnvVar{{Key: "A", Value: []byte("stage")}})
	st.ReplaceEnv(ctx, AppOwner(api.ID), []EnvVar{{Key: "A", Value: []byte("app")}})

	sv, _ := st.ListEnv(ctx, StageOwner(stage.ID))
	av, _ := st.ListEnv(ctx, AppOwner(api.ID))
	if len(sv) != 1 || string(sv[0].Value) != "stage" || len(av) != 1 || string(av[0].Value) != "app" {
		t.Fatalf("stage=%+v app=%+v", sv, av)
	}

	if err := st.DeleteApp(ctx, "auditfy", "staging", "api"); err != nil {
		t.Fatal(err)
	}
	var n int
	st.db.QueryRow(`SELECT COUNT(*) FROM env_vars WHERE app_id IS NOT NULL`).Scan(&n)
	if n != 0 {
		t.Fatalf("borrar la app debe borrar sus variables, quedan %d", n)
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/store/
```

Expected: FAIL. La primera línea relevante es:

```
internal/store/envvars_test.go:20:18: undefined: EnvVar
```

- [ ] **Step 3: Implementar**

`internal/store/envvars.go`:

```go
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// EnvOwner identifica al dueño de un set de variables: un stage o una app.
// Exactamente uno de los dos ids es distinto de cero.
type EnvOwner struct {
	StageID int64
	AppID   int64
}

func StageOwner(id int64) EnvOwner { return EnvOwner{StageID: id} }
func AppOwner(id int64) EnvOwner   { return EnvOwner{AppID: id} }

// AAD liga cada valor cifrado a su dueño y su clave (§12 del spec).
func (o EnvOwner) AAD(key string) string {
	if o.AppID != 0 {
		return fmt.Sprintf("env:app:%d:%s", o.AppID, key)
	}
	return fmt.Sprintf("env:stage:%d:%s", o.StageID, key)
}

func (o EnvOwner) where() (string, int64) {
	if o.AppID != 0 {
		return "app_id = ?", o.AppID
	}
	return "stage_id = ?", o.StageID
}

type EnvVar struct {
	Key       string
	Value     []byte // cifrado
	Secret    bool
	UpdatedAt time.Time
}

func (s *Store) ListEnv(ctx context.Context, o EnvOwner) ([]EnvVar, error) {
	cond, id := o.where()
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value, secret, updated_at FROM env_vars WHERE `+cond+` ORDER BY key`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EnvVar{}
	for rows.Next() {
		var v EnvVar
		var updated int64
		if err := rows.Scan(&v.Key, &v.Value, &v.Secret, &updated); err != nil {
			return nil, err
		}
		v.UpdatedAt = fromMS(updated)
		out = append(out, v)
	}
	return out, rows.Err()
}

// ReplaceEnv reemplaza todas las variables del dueño y marca con cambios
// pendientes las apps afectadas (la app, o todas las apps del stage).
func (s *Store) ReplaceEnv(ctx context.Context, o EnvOwner, vars []EnvVar) error {
	return s.writeEnv(ctx, o, func(tx *sql.Tx, now int64) error {
		cond, id := o.where()
		if _, err := tx.ExecContext(ctx, `DELETE FROM env_vars WHERE `+cond, id); err != nil {
			return err
		}
		for _, v := range vars {
			if err := insertEnv(ctx, tx, o, v, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// PatchEnv agrega o reemplaza las variables de set y borra las claves de unset.
func (s *Store) PatchEnv(ctx context.Context, o EnvOwner, set []EnvVar, unset []string) error {
	return s.writeEnv(ctx, o, func(tx *sql.Tx, now int64) error {
		cond, id := o.where()
		for _, key := range unset {
			if _, err := tx.ExecContext(ctx, `DELETE FROM env_vars WHERE `+cond+` AND key = ?`, id, key); err != nil {
				return err
			}
		}
		for _, v := range set {
			if _, err := tx.ExecContext(ctx, `DELETE FROM env_vars WHERE `+cond+` AND key = ?`, id, v.Key); err != nil {
				return err
			}
			if err := insertEnv(ctx, tx, o, v, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) writeEnv(ctx context.Context, o EnvOwner, fn func(*sql.Tx, int64) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := ms(s.Now())
	if err := fn(tx, now); err != nil {
		return err
	}
	if o.AppID != 0 {
		_, err = tx.ExecContext(ctx, `UPDATE apps SET config_changed_at = ? WHERE id = ?`, now, o.AppID)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE apps SET config_changed_at = ? WHERE stage_id = ?`, now, o.StageID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func insertEnv(ctx context.Context, tx *sql.Tx, o EnvOwner, v EnvVar, now int64) error {
	var stageID, appID sql.NullInt64
	if o.AppID != 0 {
		appID = sql.NullInt64{Int64: o.AppID, Valid: true}
	} else {
		stageID = sql.NullInt64{Int64: o.StageID, Valid: true}
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO env_vars (stage_id, app_id, key, value, secret, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		stageID, appID, v.Key, v.Value, v.Secret, now)
	return err
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/store/ && go test ./internal/store/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/store
```

- [ ] **Step 5: Commit**

```bash
git add internal/store/envvars.go internal/store/envvars_test.go
git commit -m "feat(store): variables de stage y app con marca de cambios pendientes" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 11: Store: usuarios

Usuarios con hash de contraseña, contador de fallos y bloqueo, y TOTP en dos columnas: `totp_pending` (recién generado, sin confirmar) y `totp_secret` (activo). Ambas usan la misma AAD, así que `ActivateTOTP` mueve el valor en SQL sin descifrarlo.

**Files:**
- Create: `internal/store/users.go`
- Test: `internal/store/users_test.go`

- [ ] **Step 1: Escribir el test**

`internal/store/users_test.go`:

```go
package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUsers(t *testing.T) {
	st, clock := newTestStore(t)
	ctx := context.Background()

	if n, _ := st.CountUsers(ctx); n != 0 {
		t.Fatalf("count=%d", n)
	}
	felipe, err := st.CreateUser(ctx, "felipe", "hash1", nil)
	if err != nil || felipe.ID == 0 || felipe.CreatedBy != nil || felipe.HasTOTP() {
		t.Fatalf("felipe=%+v err=%v", felipe, err)
	}
	ana, err := st.CreateUser(ctx, "ana", "hash2", &felipe.ID)
	if err != nil || ana.CreatedBy == nil || *ana.CreatedBy != felipe.ID {
		t.Fatalf("ana=%+v err=%v", ana, err)
	}
	if _, err := st.CreateUser(ctx, "ana", "x", nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("esperaba ErrConflict, got %v", err)
	}
	if n, _ := st.CountUsers(ctx); n != 2 {
		t.Fatalf("count=%d", n)
	}
	list, _ := st.ListUsers(ctx)
	if len(list) != 2 || list[0].Username != "ana" {
		t.Fatalf("list=%+v", list)
	}
	if _, err := st.GetUserByUsername(ctx, "nadie"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}

	// Fallos de login y bloqueo.
	if n, _ := st.IncrementFailedLogins(ctx, ana.ID); n != 1 {
		t.Fatalf("n=%d", n)
	}
	if n, _ := st.IncrementFailedLogins(ctx, ana.ID); n != 2 {
		t.Fatalf("n=%d", n)
	}
	st.SetLockedUntil(ctx, ana.ID, clock.now.Add(time.Minute))
	ana, _ = st.GetUserByID(ctx, ana.ID)
	if ana.FailedLogins != 2 || ana.LockedUntil == nil || !ana.LockedUntil.Equal(clock.now.Add(time.Minute)) {
		t.Fatalf("ana=%+v", ana)
	}
	st.ResetLoginFailures(ctx, ana.ID)
	ana, _ = st.GetUserByID(ctx, ana.ID)
	if ana.FailedLogins != 0 || ana.LockedUntil != nil {
		t.Fatalf("ana=%+v", ana)
	}

	// TOTP: pendiente → activo → borrado.
	if err := st.ActivateTOTP(ctx, ana.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("sin pendiente: esperaba ErrNotFound, got %v", err)
	}
	st.SetTOTPPending(ctx, ana.ID, []byte("cifrado"))
	if err := st.ActivateTOTP(ctx, ana.ID); err != nil {
		t.Fatal(err)
	}
	ana, _ = st.GetUserByID(ctx, ana.ID)
	if !ana.HasTOTP() || string(ana.TOTPSecret) != "cifrado" || ana.TOTPPending != nil {
		t.Fatalf("ana=%+v", ana)
	}
	st.ClearTOTP(ctx, ana.ID)
	ana, _ = st.GetUserByID(ctx, ana.ID)
	if ana.HasTOTP() {
		t.Fatal("ClearTOTP no borró el secreto")
	}

	// Borrar a felipe deja created_by de ana en NULL.
	if err := st.DeleteUser(ctx, felipe.ID); err != nil {
		t.Fatal(err)
	}
	ana, _ = st.GetUserByID(ctx, ana.ID)
	if ana.CreatedBy != nil {
		t.Fatal("created_by debió quedar NULL")
	}
	if err := st.DeleteUser(ctx, felipe.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/store/
```

Expected: FAIL. La primera línea relevante es:

```
internal/store/users_test.go:14:16: st.CountUsers undefined (type *Store has no field or method CountUsers)
```

- [ ] **Step 3: Implementar**

`internal/store/users.go`:

```go
package store

import (
	"context"
	"database/sql"
	"time"
)

type User struct {
	ID           int64      `json:"-"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	TOTPSecret   []byte     `json:"-"` // cifrado
	TOTPPending  []byte     `json:"-"` // cifrado, esperando confirmación
	CreatedBy    *int64     `json:"-"`
	CreatedAt    time.Time  `json:"created_at"`
	FailedLogins int        `json:"-"`
	LockedUntil  *time.Time `json:"-"`
}

func (u User) HasTOTP() bool { return len(u.TOTPSecret) > 0 }

const userSelect = `SELECT id, username, password_hash, totp_secret, totp_pending, created_by, created_at,
	failed_logins, locked_until FROM users`

func (s *Store) CreateUser(ctx context.Context, username, passwordHash string, createdBy *int64) (User, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, created_by, created_at) VALUES (?, ?, ?, ?)`,
		username, passwordHash, nullInt(createdBy), ms(s.Now()))
	if isUnique(err) {
		return User{}, ErrConflict
	}
	if err != nil {
		return User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return s.GetUserByID(ctx, id)
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, userSelect+` WHERE id = ?`, id))
	return u, notFound(err)
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, userSelect+` WHERE username = ?`, username))
	return u, notFound(err)
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, userSelect+` ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// DeleteUser borra el usuario; sus sesiones e invitaciones se borran en cascada.
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// IncrementFailedLogins suma un fallo y devuelve el total acumulado.
func (s *Store) IncrementFailedLogins(ctx context.Context, id int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`UPDATE users SET failed_logins = failed_logins + 1 WHERE id = ? RETURNING failed_logins`, id).Scan(&n)
	return n, notFound(err)
}

func (s *Store) SetLockedUntil(ctx context.Context, id int64, until time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET locked_until = ? WHERE id = ?`, ms(until), id)
	return err
}

func (s *Store) ResetLoginFailures(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET failed_logins = 0, locked_until = NULL WHERE id = ?`, id)
	return err
}

func (s *Store) SetTOTPPending(ctx context.Context, id int64, encrypted []byte) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET totp_pending = ? WHERE id = ?`, encrypted, id)
	return err
}

// ActivateTOTP mueve el secreto pendiente a activo. Ambas columnas usan la misma AAD.
func (s *Store) ActivateTOTP(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET totp_secret = totp_pending, totp_pending = NULL WHERE id = ? AND totp_pending IS NOT NULL`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ClearTOTP(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET totp_secret = NULL, totp_pending = NULL WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanUser(sc scanner) (User, error) {
	var u User
	var createdBy, locked sql.NullInt64
	var created int64
	err := sc.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.TOTPSecret, &u.TOTPPending, &createdBy, &created,
		&u.FailedLogins, &locked)
	if err != nil {
		return User{}, err
	}
	u.CreatedBy = fromNullInt(createdBy)
	u.CreatedAt = fromMS(created)
	u.LockedUntil = fromNullMS(locked)
	return u, nil
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/store/ && go test ./internal/store/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/store
```

- [ ] **Step 5: Commit**

```bash
git add internal/store/users.go internal/store/users_test.go
git commit -m "feat(store): usuarios, bloqueo por fallos y TOTP" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 12: Store: sesiones

Se guarda solo el hash del token de sesión. `mfa_pending` marca la sesión intermedia del login con 2FA (contraseña correcta, falta el código).

**Files:**
- Create: `internal/store/sessions.go`
- Test: `internal/store/sessions_test.go`

- [ ] **Step 1: Escribir el test**

`internal/store/sessions_test.go`:

```go
package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSessions(t *testing.T) {
	st, clock := newTestStore(t)
	ctx := context.Background()
	u, _ := st.CreateUser(ctx, "felipe", "h", nil)
	now := clock.now

	err := st.CreateSession(ctx, Session{IDHash: "s1", UserID: u.ID, CreatedAt: now, LastSeenAt: now,
		ExpiresAt: now.Add(7 * 24 * time.Hour), UserAgent: "curl", IP: "1.2.3.4"})
	if err != nil {
		t.Fatal(err)
	}
	st.CreateSession(ctx, Session{IDHash: "pending", UserID: u.ID, MFAPending: true, CreatedAt: now, LastSeenAt: now,
		ExpiresAt: now.Add(5 * time.Minute)})

	ss, err := st.GetSession(ctx, "s1")
	if err != nil || ss.UserID != u.ID || ss.MFAPending || ss.IP != "1.2.3.4" || !ss.ExpiresAt.Equal(now.Add(7*24*time.Hour)) {
		t.Fatalf("ss=%+v err=%v", ss, err)
	}
	p, _ := st.GetSession(ctx, "pending")
	if !p.MFAPending {
		t.Fatal("mfa_pending no se guardó")
	}

	st.TouchSession(ctx, "s1", now.Add(time.Hour))
	ss, _ = st.GetSession(ctx, "s1")
	if !ss.LastSeenAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("last_seen=%v", ss.LastSeenAt)
	}

	// 13 h después: s1 lleva 12 h inactiva y "pending" venció.
	n, err := st.DeleteExpiredSessions(ctx, now.Add(13*time.Hour+time.Second), 12*time.Hour)
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if _, err := st.GetSession(ctx, "s1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
	if err := st.DeleteSession(ctx, "no-existe"); err != nil {
		t.Fatalf("borrar inexistente no debe fallar: %v", err)
	}
}

func TestDeleteUserCascadesSessions(t *testing.T) {
	st, clock := newTestStore(t)
	ctx := context.Background()
	u, _ := st.CreateUser(ctx, "felipe", "h", nil)
	st.CreateSession(ctx, Session{IDHash: "s1", UserID: u.ID, CreatedAt: clock.now, LastSeenAt: clock.now, ExpiresAt: clock.now.Add(time.Hour)})
	st.DeleteUser(ctx, u.ID)
	if _, err := st.GetSession(ctx, "s1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("la sesión debió borrarse en cascada, got %v", err)
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/store/
```

Expected: FAIL. La primera línea relevante es:

```
internal/store/sessions_test.go:16:12: st.CreateSession undefined (type *Store has no field or method CreateSession)
```

- [ ] **Step 3: Implementar**

`internal/store/sessions.go`:

```go
package store

import (
	"context"
	"time"
)

type Session struct {
	IDHash     string
	UserID     int64
	MFAPending bool
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	UserAgent  string
	IP         string
}

func (s *Store) CreateSession(ctx context.Context, ss Session) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id_hash, user_id, mfa_pending, created_at, last_seen_at, expires_at, user_agent, ip)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ss.IDHash, ss.UserID, ss.MFAPending, ms(ss.CreatedAt), ms(ss.LastSeenAt), ms(ss.ExpiresAt), ss.UserAgent, ss.IP)
	return err
}

func (s *Store) GetSession(ctx context.Context, idHash string) (Session, error) {
	var ss Session
	var created, seen, expires int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id_hash, user_id, mfa_pending, created_at, last_seen_at, expires_at, user_agent, ip
		 FROM sessions WHERE id_hash = ?`, idHash).
		Scan(&ss.IDHash, &ss.UserID, &ss.MFAPending, &created, &seen, &expires, &ss.UserAgent, &ss.IP)
	if err != nil {
		return Session{}, notFound(err)
	}
	ss.CreatedAt, ss.LastSeenAt, ss.ExpiresAt = fromMS(created), fromMS(seen), fromMS(expires)
	return ss, nil
}

func (s *Store) TouchSession(ctx context.Context, idHash string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ? WHERE id_hash = ?`, ms(at), idHash)
	return err
}

// DeleteSession no falla si la sesión ya no existe.
func (s *Store) DeleteSession(ctx context.Context, idHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id_hash = ?`, idHash)
	return err
}

// DeleteExpiredSessions borra las sesiones vencidas o inactivas por más de idle.
func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time, idle time.Duration) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE expires_at <= ? OR (mfa_pending = 0 AND last_seen_at < ?)`,
		ms(now), ms(now.Add(-idle)))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/store/ && go test ./internal/store/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/store
```

- [ ] **Step 5: Commit**

```bash
git add internal/store/sessions.go internal/store/sessions_test.go
git commit -m "feat(store): sesiones" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 13: Store: tokens de API e invitaciones

Un token de API tiene alcance de proyecto (`stage` vacío) o de proyecto + stage, y se borra en cascada con su stage. `AcceptInvite` consume la invitación y crea el usuario en una sola transacción: si el nombre ya existe, la invitación queda sin usar. Las invitaciones con `created_by` NULL son los links de setup del sistema.

**Files:**
- Create: `internal/store/tokens.go`
- Create: `internal/store/invites.go`
- Test: `internal/store/tokens_test.go`
- Test: `internal/store/invites_test.go`

- [ ] **Step 1: Escribir los tests**

`internal/store/tokens_test.go`:

```go
package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTokens(t *testing.T) {
	st, clock := newTestStore(t)
	ctx := context.Background()
	seedStage(t, st)
	u, _ := st.CreateUser(ctx, "felipe", "h", nil)

	exp := clock.now.Add(24 * time.Hour)
	proj, err := st.CreateToken(ctx, NewAPIToken{Name: "ci-todo", Hash: "h1", ProjectSlug: "auditfy", CreatedBy: &u.ID, ExpiresAt: &exp})
	if err != nil || proj.ProjectSlug != "auditfy" || proj.StageSlug != "" || proj.StageID != nil || !proj.ExpiresAt.Equal(exp) {
		t.Fatalf("proj=%+v err=%v", proj, err)
	}
	stg, err := st.CreateToken(ctx, NewAPIToken{Name: "ci-staging", Hash: "h2", ProjectSlug: "auditfy", StageSlug: "staging"})
	if err != nil || stg.StageSlug != "staging" || stg.StageID == nil || stg.ExpiresAt != nil {
		t.Fatalf("stg=%+v err=%v", stg, err)
	}
	if _, err := st.CreateToken(ctx, NewAPIToken{Name: "x", Hash: "h3", ProjectSlug: "auditfy", StageSlug: "nope"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stage inexistente: esperaba ErrNotFound, got %v", err)
	}

	got, err := st.GetTokenByHash(ctx, "h2")
	if err != nil || got.ID != stg.ID || got.LastUsedAt != nil {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	st.TouchToken(ctx, stg.ID, clock.now)
	got, _ = st.GetTokenByHash(ctx, "h2")
	if got.LastUsedAt == nil || !got.LastUsedAt.Equal(clock.now) {
		t.Fatalf("last_used=%v", got.LastUsedAt)
	}

	list, _ := st.ListTokens(ctx)
	if len(list) != 2 {
		t.Fatalf("list=%+v", list)
	}
	if err := st.DeleteToken(ctx, proj.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteToken(ctx, proj.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}

	// Borrar el stage borra sus tokens.
	if err := st.DeleteStage(ctx, "auditfy", "staging"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetTokenByHash(ctx, "h2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("el token del stage debió borrarse, got %v", err)
	}
}
```

`internal/store/invites_test.go`:

```go
package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAcceptInvite(t *testing.T) {
	st, clock := newTestStore(t)
	ctx := context.Background()
	felipe, _ := st.CreateUser(ctx, "felipe", "h", nil)
	st.CreateInvite(ctx, "inv1", &felipe.ID, clock.now.Add(24*time.Hour))

	ana, err := st.AcceptInvite(ctx, "inv1", "ana", "h2")
	if err != nil || ana.Username != "ana" || ana.CreatedBy == nil || *ana.CreatedBy != felipe.ID {
		t.Fatalf("ana=%+v err=%v", ana, err)
	}
	if _, err := st.AcceptInvite(ctx, "inv1", "otra", "h"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("invitación usada: esperaba ErrInviteInvalid, got %v", err)
	}
	if _, err := st.AcceptInvite(ctx, "no-existe", "otra", "h"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("esperaba ErrInviteInvalid, got %v", err)
	}
}

func TestAcceptInviteExpiredOrNameTaken(t *testing.T) {
	st, clock := newTestStore(t)
	ctx := context.Background()
	st.CreateUser(ctx, "felipe", "h", nil)
	st.CreateInvite(ctx, "vieja", nil, clock.now.Add(time.Hour))
	st.CreateInvite(ctx, "nueva", nil, clock.now.Add(48*time.Hour))

	// Nombre tomado: falla y la invitación sigue disponible.
	if _, err := st.AcceptInvite(ctx, "nueva", "felipe", "h"); !errors.Is(err, ErrConflict) {
		t.Fatalf("esperaba ErrConflict, got %v", err)
	}
	if _, err := st.AcceptInvite(ctx, "nueva", "ana", "h"); err != nil {
		t.Fatalf("la invitación no debió consumirse: %v", err)
	}

	clock.advance(2 * time.Hour)
	if _, err := st.AcceptInvite(ctx, "vieja", "beto", "h"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("vencida: esperaba ErrInviteInvalid, got %v", err)
	}
}

func TestDeleteUnusedSystemInvites(t *testing.T) {
	st, clock := newTestStore(t)
	ctx := context.Background()
	felipe, _ := st.CreateUser(ctx, "felipe", "h", nil)
	st.CreateInvite(ctx, "sys", nil, clock.now.Add(time.Hour))
	st.CreateInvite(ctx, "user", &felipe.ID, clock.now.Add(time.Hour))
	if err := st.DeleteUnusedSystemInvites(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AcceptInvite(ctx, "sys", "ana", "h"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("la invitación del sistema debió borrarse, got %v", err)
	}
	if _, err := st.AcceptInvite(ctx, "user", "ana", "h"); err != nil {
		t.Fatalf("la invitación de un usuario no se toca: %v", err)
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/store/
```

Expected: FAIL. La primera línea relevante es:

```
internal/store/invites_test.go:14:5: st.CreateInvite undefined (type *Store has no field or method CreateInvite)
```

- [ ] **Step 3: Implementar**

`internal/store/tokens.go`:

```go
package store

import (
	"context"
	"database/sql"
	"time"
)

// APIToken es un token de CI con alcance de proyecto, o de proyecto + stage.
type APIToken struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	TokenHash   string     `json:"-"`
	ProjectID   int64      `json:"-"`
	ProjectSlug string     `json:"project"`
	StageID     *int64     `json:"-"`
	StageSlug   string     `json:"stage,omitempty"` // vacío = todo el proyecto
	CreatedBy   *int64     `json:"-"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

type NewAPIToken struct {
	Name        string
	Hash        string
	ProjectSlug string
	StageSlug   string // vacío = todo el proyecto
	CreatedBy   *int64
	ExpiresAt   *time.Time
}

const tokenSelect = `SELECT t.id, t.name, t.token_hash, t.project_id, p.slug, t.stage_id, COALESCE(s.slug, ''),
	t.created_by, t.created_at, t.last_used_at, t.expires_at
	FROM api_tokens t JOIN projects p ON p.id = t.project_id LEFT JOIN stages s ON s.id = t.stage_id`

func (s *Store) CreateToken(ctx context.Context, n NewAPIToken) (APIToken, error) {
	p, err := s.GetProject(ctx, n.ProjectSlug)
	if err != nil {
		return APIToken{}, err
	}
	var stageID sql.NullInt64
	if n.StageSlug != "" {
		st, err := s.GetStage(ctx, n.ProjectSlug, n.StageSlug)
		if err != nil {
			return APIToken{}, err
		}
		stageID = sql.NullInt64{Int64: st.ID, Valid: true}
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO api_tokens (name, token_hash, project_id, stage_id, created_by, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		n.Name, n.Hash, p.ID, stageID, nullInt(n.CreatedBy), ms(s.Now()), nullMS(n.ExpiresAt))
	if isUnique(err) {
		return APIToken{}, ErrConflict
	}
	if err != nil {
		return APIToken{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return APIToken{}, err
	}
	t, err := scanToken(s.db.QueryRowContext(ctx, tokenSelect+` WHERE t.id = ?`, id))
	return t, notFound(err)
}

func (s *Store) GetTokenByHash(ctx context.Context, hash string) (APIToken, error) {
	t, err := scanToken(s.db.QueryRowContext(ctx, tokenSelect+` WHERE t.token_hash = ?`, hash))
	return t, notFound(err)
}

func (s *Store) ListTokens(ctx context.Context) ([]APIToken, error) {
	rows, err := s.db.QueryContext(ctx, tokenSelect+` ORDER BY t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIToken{}
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) DeleteToken(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) TouchToken(ctx context.Context, id int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, ms(at), id)
	return err
}

func scanToken(sc scanner) (APIToken, error) {
	var t APIToken
	var stageID, createdBy, used, expires sql.NullInt64
	var created int64
	err := sc.Scan(&t.ID, &t.Name, &t.TokenHash, &t.ProjectID, &t.ProjectSlug, &stageID, &t.StageSlug,
		&createdBy, &created, &used, &expires)
	if err != nil {
		return APIToken{}, err
	}
	t.StageID = fromNullInt(stageID)
	t.CreatedBy = fromNullInt(createdBy)
	t.CreatedAt = fromMS(created)
	t.LastUsedAt = fromNullMS(used)
	t.ExpiresAt = fromNullMS(expires)
	return t, nil
}
```

`internal/store/invites.go`:

```go
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrInviteInvalid = errors.New("store: invitación inválida, usada o vencida")

// CreateInvite guarda una invitación. createdBy nil = invitación de setup del sistema.
func (s *Store) CreateInvite(ctx context.Context, tokenHash string, createdBy *int64, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO invites (token_hash, created_by, expires_at) VALUES (?, ?, ?)`,
		tokenHash, nullInt(createdBy), ms(expiresAt))
	return err
}

// DeleteUnusedSystemInvites borra los links de setup sin usar (antes de generar uno nuevo).
func (s *Store) DeleteUnusedSystemInvites(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM invites WHERE created_by IS NULL AND used_at IS NULL`)
	return err
}

// AcceptInvite consume la invitación y crea el usuario en una misma transacción.
// Si el nombre ya existe devuelve ErrConflict y la invitación queda sin usar.
func (s *Store) AcceptInvite(ctx context.Context, tokenHash, username, passwordHash string) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	var inviteID int64
	var createdBy, used sql.NullInt64
	var expires int64
	err = tx.QueryRowContext(ctx, `SELECT id, created_by, expires_at, used_at FROM invites WHERE token_hash = ?`, tokenHash).
		Scan(&inviteID, &createdBy, &expires, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrInviteInvalid
	}
	if err != nil {
		return User{}, err
	}
	now := ms(s.Now())
	if used.Valid || now >= expires {
		return User{}, ErrInviteInvalid
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO users (username, password_hash, created_by, created_at) VALUES (?, ?, ?, ?)`,
		username, passwordHash, createdBy, now)
	if isUnique(err) {
		return User{}, ErrConflict
	}
	if err != nil {
		return User{}, err
	}
	userID, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE invites SET used_at = ? WHERE id = ?`, now, inviteID); err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return s.GetUserByID(ctx, userID)
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/store/ && go test ./internal/store/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/store
```

- [ ] **Step 5: Commit**

```bash
git add internal/store/tokens.go internal/store/invites.go internal/store/tokens_test.go internal/store/invites_test.go
git commit -m "feat(store): tokens de API con alcance e invitaciones" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 14: Store: auditoría, settings y backup

Auditoría paginada por id descendente. Settings como clave/valor (en la Fase 1 solo `require_2fa`). `Backup` usa `VACUUM INTO`, que genera una copia consistente con la base en uso, y conserva las últimas N copias (§12).

**Files:**
- Create: `internal/store/audit.go`
- Create: `internal/store/settings.go`
- Create: `internal/store/backup.go`
- Test: `internal/store/misc_test.go`

- [ ] **Step 1: Escribir el test**

`internal/store/misc_test.go`:

```go
package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestAudit(t *testing.T) {
	st, clock := newTestStore(t)
	ctx := context.Background()
	uid := int64(7)
	st.AddAudit(ctx, AuditEvent{ActorType: "user", ActorID: &uid, Action: "project.create", Target: "project:a"})
	clock.advance(time.Hour)
	st.AddAudit(ctx, AuditEvent{ActorType: "token", Action: "env.replace", Target: "app:a/s/x",
		Details: json.RawMessage(`{"keys":["A"]}`)})

	list, err := st.ListAudit(ctx, 0, 10)
	if err != nil || len(list) != 2 || list[0].Action != "env.replace" || string(list[0].Details) != `{"keys":["A"]}` {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if string(list[1].Details) != "{}" || list[1].ActorID == nil || *list[1].ActorID != 7 {
		t.Fatalf("evento 1=%+v", list[1])
	}
	older, _ := st.ListAudit(ctx, list[0].ID, 10)
	if len(older) != 1 || older[0].ID != list[1].ID {
		t.Fatalf("older=%+v", older)
	}

	n, err := st.DeleteAuditBefore(ctx, clock.now.Add(-time.Minute))
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestSettings(t *testing.T) {
	st, _ := newTestStore(t)
	ctx := context.Background()
	if _, ok, err := st.GetSetting(ctx, "require_2fa"); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	st.SetSetting(ctx, "require_2fa", "true")
	st.SetSetting(ctx, "require_2fa", "false")
	v, ok, err := st.GetSetting(ctx, "require_2fa")
	if !ok || err != nil || v != "false" {
		t.Fatalf("v=%q ok=%v err=%v", v, ok, err)
	}
}

func TestBackupKeepsLastN(t *testing.T) {
	st, clock := newTestStore(t)
	ctx := context.Background()
	st.CreateProject(ctx, "auditfy", "Auditfy")
	dir := filepath.Join(t.TempDir(), "backups")

	var last string
	for i := 0; i < 4; i++ {
		p, err := st.Backup(ctx, dir, 3)
		if err != nil {
			t.Fatal(err)
		}
		last = p
		clock.advance(24 * time.Hour)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "muelle-*.db"))
	if len(files) != 3 || files[2] != last {
		t.Fatalf("files=%v last=%s", files, last)
	}

	// La copia es una base válida con los datos.
	cp, err := Open(last)
	if err != nil {
		t.Fatal(err)
	}
	defer cp.Close()
	if _, err := cp.GetProject(ctx, "auditfy"); err != nil {
		t.Fatalf("la copia no tiene el proyecto: %v", err)
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/store/
```

Expected: FAIL. La primera línea relevante es:

```
internal/store/misc_test.go:15:5: st.AddAudit undefined (type *Store has no field or method AddAudit)
```

- [ ] **Step 3: Implementar**

`internal/store/audit.go`:

```go
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type AuditEvent struct {
	ID        int64           `json:"id"`
	ActorType string          `json:"actor_type"` // "user" | "token" | "system"
	ActorID   *int64          `json:"actor_id"`
	Action    string          `json:"action"` // ej. "project.create"
	Target    string          `json:"target"` // ej. "project:auditfy"
	Details   json.RawMessage `json:"details"`
	At        time.Time       `json:"at"`
}

// AddAudit guarda un evento. At vacío = ahora; Details vacío o null = {}.
func (s *Store) AddAudit(ctx context.Context, e AuditEvent) error {
	if e.At.IsZero() {
		e.At = s.Now()
	}
	details := string(e.Details)
	if details == "" || details == "null" {
		details = "{}"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_events (actor_type, actor_id, action, target, details, at) VALUES (?, ?, ?, ?, ?, ?)`,
		e.ActorType, nullInt(e.ActorID), e.Action, e.Target, details, ms(e.At))
	return err
}

// ListAudit devuelve los eventos más nuevos primero. beforeID 0 = desde el último.
func (s *Store) ListAudit(ctx context.Context, beforeID int64, limit int) ([]AuditEvent, error) {
	query := `SELECT id, actor_type, actor_id, action, target, details, at FROM audit_events`
	args := []any{}
	if beforeID > 0 {
		query += ` WHERE id < ?`
		args = append(args, beforeID)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEvent{}
	for rows.Next() {
		var e AuditEvent
		var actor sql.NullInt64
		var details string
		var at int64
		if err := rows.Scan(&e.ID, &e.ActorType, &actor, &e.Action, &e.Target, &details, &at); err != nil {
			return nil, err
		}
		e.ActorID = fromNullInt(actor)
		e.Details = json.RawMessage(details)
		e.At = fromMS(at)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) DeleteAuditBefore(ctx context.Context, t time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM audit_events WHERE at < ?`, ms(t))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
```

`internal/store/settings.go`:

```go
package store

import (
	"context"
	"database/sql"
	"errors"
)

// GetSetting devuelve el valor y si existe. Solo valores sin cifrar en la Fase 1.
func (s *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (key, value, encrypted) VALUES (?, ?, 0)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value, encrypted = 0`, key, value)
	return err
}
```

`internal/store/backup.go`:

```go
package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Backup escribe una copia consistente de la base con VACUUM INTO en dir y
// conserva solo las keep copias más recientes. Devuelve la ruta creada.
func (s *Store) Backup(ctx context.Context, dir string, keep int) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "muelle-"+s.Now().UTC().Format("20060102-150405")+".db")
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return "", fmt.Errorf("store: backup en %s: %w", path, err)
	}
	old, err := filepath.Glob(filepath.Join(dir, "muelle-*.db"))
	if err != nil {
		return "", err
	}
	sort.Strings(old) // el formato de fecha ordena cronológicamente
	for len(old) > keep {
		if err := os.Remove(old[0]); err != nil {
			return "", err
		}
		old = old[1:]
	}
	return path, nil
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/store/ && go test ./internal/store/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/store
```

- [ ] **Step 5: Commit**

```bash
git add internal/store/audit.go internal/store/settings.go internal/store/backup.go internal/store/misc_test.go
git commit -m "feat(store): auditor\u00eda, settings y backup con VACUUM INTO" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 15: Servicio de variables: cifrado y enmascarado

Une el store con el cifrado. `List` nunca devuelve el valor de una variable secreta. `Replace`/`Patch` validan las claves, el tamaño (64 KiB por valor) y el máximo de 500 variables. `Resolve` (stage + app, gana la app) es lo que usará el deploy en la Fase 2.

**Files:**
- Create: `internal/envvars/envvars.go`
- Test: `internal/envvars/envvars_test.go`

- [ ] **Step 1: Escribir el test**

`internal/envvars/envvars_test.go`:

```go
package envvars

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/internal/crypto"
	"github.com/CPU-commits/muelle/internal/store"
	"github.com/CPU-commits/muelle/internal/validate"
)

type fixture struct {
	svc   *Service
	st    *store.Store
	stage store.Stage
	app   store.App
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	c, _ := crypto.New(make([]byte, crypto.KeySize))
	st.CreateProject(ctx, "auditfy", "Auditfy")
	stage, _ := st.CreateStage(ctx, "auditfy", "staging")
	app, err := st.CreateApp(ctx, store.NewApp{ProjectSlug: "auditfy", StageSlug: "staging", Slug: "api",
		Kind: store.KindService, Spec: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{svc: NewService(st, c), st: st, stage: stage, app: app}
}

func isValidation(err error) bool {
	var ve *validate.Error
	return errors.As(err, &ve)
}

func TestListMasksSecrets(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	o := store.AppOwner(f.app.ID)
	if err := f.svc.Replace(ctx, o, map[string]string{"DATABASE_URL": "postgres://x"}, true); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Patch(ctx, o, map[string]string{"LOG_LEVEL": "debug"}, false, nil); err != nil {
		t.Fatal(err)
	}
	vars, err := f.svc.List(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(vars) != 2 || vars[0].Key != "DATABASE_URL" || vars[0].Value != nil || !vars[0].Secret {
		t.Fatalf("vars=%+v", vars)
	}
	if vars[1].Value == nil || *vars[1].Value != "debug" || vars[1].Secret {
		t.Fatalf("vars[1]=%+v", vars[1])
	}
}

func TestValuesAreEncryptedAtRest(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	o := store.AppOwner(f.app.ID)
	f.svc.Replace(ctx, o, map[string]string{"TOKEN": "super-secreto"}, true)
	rows, _ := f.st.ListEnv(ctx, o)
	if len(rows) != 1 || strings.Contains(string(rows[0].Value), "super-secreto") {
		t.Fatalf("el valor quedó en claro: %q", rows[0].Value)
	}
}

func TestResolveMergesStageAndApp(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.svc.Replace(ctx, store.StageOwner(f.stage.ID), map[string]string{"NATS": "nats:4222", "LEVEL": "info"}, false)
	f.svc.Replace(ctx, store.AppOwner(f.app.ID), map[string]string{"LEVEL": "debug", "DB": "pg"}, true)
	got, err := f.svc.Resolve(ctx, f.stage.ID, f.app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got["NATS"] != "nats:4222" || got["LEVEL"] != "debug" || got["DB"] != "pg" {
		t.Fatalf("got=%v", got)
	}
}

func TestValidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	o := store.AppOwner(f.app.ID)
	if err := f.svc.Replace(ctx, o, map[string]string{"1BAD": "x"}, true); !isValidation(err) {
		t.Errorf("clave inválida: got %v", err)
	}
	if err := f.svc.Replace(ctx, o, map[string]string{"BIG": strings.Repeat("x", MaxValueBytes+1)}, true); !isValidation(err) {
		t.Errorf("valor grande: got %v", err)
	}
	if err := f.svc.Patch(ctx, o, map[string]string{"A": "1"}, true, []string{"A"}); !isValidation(err) {
		t.Errorf("set y unset a la vez: got %v", err)
	}
	if err := f.svc.Patch(ctx, o, nil, true, []string{"bad-key"}); !isValidation(err) {
		t.Errorf("unset inválido: got %v", err)
	}
	many := map[string]string{}
	for i := 0; i < MaxVars; i++ {
		many[fmt.Sprintf("K%d", i)] = "x"
	}
	if err := f.svc.Replace(ctx, o, many, true); err != nil {
		t.Fatalf("%d variables deberían caber: %v", len(many), err)
	}
	if err := f.svc.Patch(ctx, o, map[string]string{"ONE_MORE": "x"}, true, nil); !isValidation(err) {
		t.Errorf("superar MaxVars con patch: got %v", err)
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/envvars/
```

Expected: FAIL. La primera línea relevante es:

```
internal/envvars/envvars_test.go:18:9: undefined: Service
```

- [ ] **Step 3: Implementar**

`internal/envvars/envvars.go`:

```go
// Package envvars cifra, valida y enmascara las variables de stages y apps.
package envvars

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/CPU-commits/muelle/internal/crypto"
	"github.com/CPU-commits/muelle/internal/store"
	"github.com/CPU-commits/muelle/internal/validate"
)

const (
	MaxVars       = 500
	MaxValueBytes = 64 << 10
)

type Service struct {
	st *store.Store
	c  *crypto.Cipher
}

func NewService(st *store.Store, c *crypto.Cipher) *Service {
	return &Service{st: st, c: c}
}

// Var es una variable tal como la ve la API. Value es nil si es secreta.
type Var struct {
	Key       string    `json:"key"`
	Value     *string   `json:"value"`
	Secret    bool      `json:"secret"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Service) List(ctx context.Context, o store.EnvOwner) ([]Var, error) {
	rows, err := s.st.ListEnv(ctx, o)
	if err != nil {
		return nil, err
	}
	out := make([]Var, 0, len(rows))
	for _, r := range rows {
		v := Var{Key: r.Key, Secret: r.Secret, UpdatedAt: r.UpdatedAt}
		if !r.Secret {
			plain, err := s.c.Decrypt(r.Value, o.AAD(r.Key))
			if err != nil {
				return nil, fmt.Errorf("envvars: %s: %w", r.Key, err)
			}
			str := string(plain)
			v.Value = &str
		}
		out = append(out, v)
	}
	return out, nil
}

// Replace reemplaza todas las variables del dueño. secret aplica a todas.
func (s *Service) Replace(ctx context.Context, o store.EnvOwner, vars map[string]string, secret bool) error {
	if len(vars) > MaxVars {
		return validate.Errorf("vars", "máximo %d variables", MaxVars)
	}
	enc, err := s.encrypt(o, vars, secret)
	if err != nil {
		return err
	}
	return s.st.ReplaceEnv(ctx, o, enc)
}

// Patch agrega o reemplaza las variables de set (con el secret indicado) y borra las de unset.
func (s *Service) Patch(ctx context.Context, o store.EnvOwner, set map[string]string, secret bool, unset []string) error {
	for _, k := range unset {
		if err := validate.EnvKey(k); err != nil {
			return err
		}
		if _, ok := set[k]; ok {
			return validate.Errorf("unset", "%q no puede estar en set y unset a la vez", k)
		}
	}
	current, err := s.st.ListEnv(ctx, o)
	if err != nil {
		return err
	}
	total := map[string]bool{}
	for _, v := range current {
		total[v.Key] = true
	}
	for k := range set {
		total[k] = true
	}
	for _, k := range unset {
		delete(total, k)
	}
	if len(total) > MaxVars {
		return validate.Errorf("vars", "máximo %d variables", MaxVars)
	}
	enc, err := s.encrypt(o, set, secret)
	if err != nil {
		return err
	}
	return s.st.PatchEnv(ctx, o, enc, unset)
}

// Resolve devuelve las variables descifradas que recibe una app: las del stage
// y encima las de la app (la app gana). Lo usa el deploy (Fase 2).
func (s *Service) Resolve(ctx context.Context, stageID, appID int64) (map[string]string, error) {
	out := map[string]string{}
	for _, o := range []store.EnvOwner{store.StageOwner(stageID), store.AppOwner(appID)} {
		rows, err := s.st.ListEnv(ctx, o)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			plain, err := s.c.Decrypt(r.Value, o.AAD(r.Key))
			if err != nil {
				return nil, fmt.Errorf("envvars: %s: %w", r.Key, err)
			}
			out[r.Key] = string(plain)
		}
	}
	return out, nil
}

func (s *Service) encrypt(o store.EnvOwner, vars map[string]string, secret bool) ([]store.EnvVar, error) {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]store.EnvVar, 0, len(keys))
	for _, k := range keys {
		if err := validate.EnvKey(k); err != nil {
			return nil, err
		}
		v := vars[k]
		if len(v) > MaxValueBytes {
			return nil, validate.Errorf("vars", "el valor de %q supera 64 KiB", k)
		}
		out = append(out, store.EnvVar{Key: k, Value: s.c.Encrypt([]byte(v), o.AAD(k)), Secret: secret})
	}
	return out, nil
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/envvars/ && go test ./internal/envvars/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/envvars
```

- [ ] **Step 5: Commit**

```bash
git add internal/envvars/envvars.go internal/envvars/envvars_test.go
git commit -m "feat(envvars): cifrado, enmascarado y mezcla de variables" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 16: Auth: contraseñas, tokens aleatorios y límites

argon2id con los parámetros de §10.1 en formato PHC (`$argon2id$v=19$m=65536,t=3,p=2$salt$hash`). `RandomToken` genera 32 bytes en base64url; solo se guarda su SHA-256 (los tokens tienen 256 bits de entropía, así que no hace falta un hash lento). `IPLimiter` y `LockoutFor` implementan los límites de login.

**Files:**
- Create: `internal/auth/password.go`
- Create: `internal/auth/random.go`
- Create: `internal/auth/limiter.go`
- Test: `internal/auth/primitives_test.go`

- [ ] **Step 1: Escribir el test**

`internal/auth/primitives_test.go`:

```go
package auth

import (
	"strings"
	"testing"
	"time"
)

func TestPasswordHashAndVerify(t *testing.T) {
	h, err := HashPassword("una-contraseña-larga")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatalf("formato: %s", h)
	}
	ok, err := VerifyPassword(h, "una-contraseña-larga")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	ok, _ = VerifyPassword(h, "otra-contraseña-larga")
	if ok {
		t.Fatal("contraseña incorrecta aceptada")
	}
	h2, _ := HashPassword("una-contraseña-larga")
	if h == h2 {
		t.Fatal("el salt debe ser aleatorio")
	}
	if _, err := VerifyPassword("$bcrypt$x", "x"); err == nil {
		t.Fatal("formato inválido debió fallar")
	}
}

func TestRandomToken(t *testing.T) {
	plain, hash := RandomToken(APITokenPrefix)
	if !strings.HasPrefix(plain, "mk_") || len(plain) != 3+43 {
		t.Fatalf("plain=%q (%d)", plain, len(plain))
	}
	if hash != HashToken(plain) || len(hash) != 64 {
		t.Fatalf("hash=%q", hash)
	}
	other, _ := RandomToken(APITokenPrefix)
	if other == plain {
		t.Fatal("tokens repetidos")
	}
}

func TestIPLimiter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := NewIPLimiter(3, 15*time.Minute, func() time.Time { return now })
	for i := 0; i < 3; i++ {
		if !l.Allow("1.1.1.1") {
			t.Fatalf("intento %d rechazado", i+1)
		}
	}
	if l.Allow("1.1.1.1") {
		t.Fatal("el cuarto intento debió rechazarse")
	}
	if !l.Allow("2.2.2.2") {
		t.Fatal("otra IP no debe verse afectada")
	}
	now = now.Add(15 * time.Minute)
	if !l.Allow("1.1.1.1") {
		t.Fatal("la ventana nueva debe permitir")
	}
}

func TestLockoutFor(t *testing.T) {
	cases := map[int]time.Duration{
		0: 0, 4: 0, 5: time.Minute, 6: 2 * time.Minute, 7: 4 * time.Minute,
		10: 32 * time.Minute, 11: time.Hour, 50: time.Hour,
	}
	for n, want := range cases {
		if got := LockoutFor(n); got != want {
			t.Errorf("LockoutFor(%d) = %v, want %v", n, got, want)
		}
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/auth/
```

Expected: FAIL. La primera línea relevante es:

```
internal/auth/primitives_test.go:10:12: undefined: HashPassword
```

- [ ] **Step 3: Implementar**

`internal/auth/password.go`:

```go
// Package auth maneja contraseñas, sesiones, 2FA, invitaciones y tokens de API.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

// Parámetros de argon2id del spec (§10.1): m=64MiB, t=3, p=2.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 2
	argonKeyLen  = 32
	saltLen      = 16
)

var errBadHash = errors.New("auth: hash de contraseña con formato inválido")

// HashPassword devuelve el hash en formato PHC:
// $argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	rand.Read(salt)
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword compara en tiempo constante usando los parámetros guardados en el hash.
func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errBadHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errBadHash
	}
	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, errBadHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errBadHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, errBadHash
	}
	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// dummyHash se usa cuando el usuario no existe, para que el login tarde lo
// mismo y no revele qué nombres existen.
var dummyHash = sync.OnceValue(func() string {
	h, _ := HashPassword("muelle-usuario-inexistente")
	return h
})
```

`internal/auth/random.go`:

```go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// APITokenPrefix identifica los tokens de API (§10.1).
const APITokenPrefix = "mk_"

// RandomToken genera 32 bytes aleatorios en base64url con el prefijo dado y
// devuelve el token en claro y su hash SHA-256 (lo único que se guarda).
func RandomToken(prefix string) (plain, hash string) {
	b := make([]byte, 32)
	rand.Read(b)
	plain = prefix + base64.RawURLEncoding.EncodeToString(b)
	return plain, HashToken(plain)
}

func HashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
```

`internal/auth/limiter.go`:

```go
package auth

import (
	"sync"
	"time"
)

// IPLimiter cuenta intentos por IP en ventanas fijas (20 cada 15 min en el spec).
type IPLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	now    func() time.Time
	hits   map[string]ipWindow
}

type ipWindow struct {
	start time.Time
	count int
}

func NewIPLimiter(max int, window time.Duration, now func() time.Time) *IPLimiter {
	return &IPLimiter{max: max, window: window, now: now, hits: map[string]ipWindow{}}
}

// Allow registra un intento y dice si está permitido.
func (l *IPLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	w := l.hits[ip]
	if now.Sub(w.start) >= l.window {
		w = ipWindow{start: now}
	}
	if w.count >= l.max {
		l.hits[ip] = w
		return false
	}
	w.count++
	l.hits[ip] = w
	if len(l.hits) > 10000 { // limpieza para que el mapa no crezca sin límite
		for k, v := range l.hits {
			if now.Sub(v.start) >= l.window {
				delete(l.hits, k)
			}
		}
	}
	return true
}

// LockoutFor devuelve cuánto se bloquea un usuario tras n fallos seguidos:
// nada hasta 4, 1 min con 5, y se duplica con cada fallo hasta 1 h.
func LockoutFor(failures int) time.Duration {
	if failures < 5 {
		return 0
	}
	if failures >= 11 { // 1 min << 6 ya supera la hora
		return time.Hour
	}
	return time.Minute << (failures - 5)
}
```

Agregar las dependencias:

```bash
go get golang.org/x/crypto@v0.58.0
go mod tidy
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/auth/ && go test ./internal/auth/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/auth
```

- [ ] **Step 5: Commit**

```bash
git add internal/auth/password.go internal/auth/random.go internal/auth/limiter.go internal/auth/primitives_test.go go.mod go.sum
git commit -m "feat(auth): argon2id, tokens aleatorios y limitadores de login" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 17: Auth: servicio de sesiones, 2FA, invitaciones y tokens

Toda la lógica de autenticación. Puntos clave:
- Si el usuario no existe, el login igual calcula un argon2 (`dummyHash`) para no revelar qué nombres existen.
- Con TOTP, el login crea una sesión `mfa_pending` de 5 minutos. `CompleteTOTP` la borra y emite una sesión nueva, con otro token (evita fijación de sesión).
- Sesión: 12 h de inactividad y 7 días de máximo. `last_seen_at` se actualiza como mucho una vez por minuto.
- `NeedsTOTPSetup` se calcula en cada request a partir de `require_2fa`.

**Files:**
- Create: `internal/auth/service.go`
- Test: `internal/auth/service_test.go`

- [ ] **Step 1: Escribir el test**

`internal/auth/service_test.go`:

```go
package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/CPU-commits/muelle/internal/crypto"
	"github.com/CPU-commits/muelle/internal/store"
)

const testPassword = "una-contraseña-larga"

type fixture struct {
	svc *Service
	st  *store.Store
	now time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	f := &fixture{st: st, now: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
	st.Now = func() time.Time { return f.now }
	c, _ := crypto.New(make([]byte, crypto.KeySize))
	f.svc = NewService(st, c)
	return f
}

func (f *fixture) advance(d time.Duration) { f.now = f.now.Add(d) }

func (f *fixture) user(t *testing.T, name string) store.User {
	t.Helper()
	h, _ := HashPassword(testPassword)
	u, err := f.st.CreateUser(context.Background(), name, h, nil)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestLoginAndAuthenticateSession(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")

	res, err := f.svc.Login(ctx, "felipe", testPassword, "1.1.1.1", "curl")
	if err != nil || res.MFARequired || res.Token == "" || !res.ExpiresAt.Equal(f.now.Add(SessionMax)) {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	p, err := f.svc.AuthenticateSession(ctx, res.Token)
	if err != nil || p.UserID != u.ID || p.Username != "felipe" || p.MFAPending || p.IsToken() {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	if _, err := f.svc.AuthenticateSession(ctx, "token-falso"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("esperaba ErrUnauthenticated, got %v", err)
	}
	if err := f.svc.Logout(ctx, res.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AuthenticateSession(ctx, res.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("tras logout: esperaba ErrUnauthenticated, got %v", err)
	}
}

func TestSessionIdleAndMaxExpiry(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.user(t, "felipe")
	res, _ := f.svc.Login(ctx, "felipe", testPassword, "1.1.1.1", "")

	// Usarla cada 11 h la mantiene viva hasta las 165 h...
	for i := 1; i <= 15; i++ {
		f.advance(11 * time.Hour)
		if _, err := f.svc.AuthenticateSession(ctx, res.Token); err != nil {
			t.Fatalf("a las %d h: %v", 11*i, err)
		}
	}
	// ...pero a las 176 h ya pasó el máximo de 7 días (168 h).
	f.advance(11 * time.Hour)
	if _, err := f.svc.AuthenticateSession(ctx, res.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("pasado el máximo: esperaba ErrUnauthenticated, got %v", err)
	}

	res2, _ := f.svc.Login(ctx, "felipe", testPassword, "1.1.1.1", "")
	f.advance(12*time.Hour + time.Second)
	if _, err := f.svc.AuthenticateSession(ctx, res2.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("12 h sin uso: esperaba ErrUnauthenticated, got %v", err)
	}
}

func TestLoginFailuresLockAndReset(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.user(t, "felipe")

	if _, err := f.svc.Login(ctx, "nadie", testPassword, "9.9.9.9", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("usuario inexistente: got %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := f.svc.Login(ctx, "felipe", "contraseña-mala!!", "1.1.1.1", ""); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("intento %d: got %v", i, err)
		}
	}
	// Tras 5 fallos queda bloqueado 1 min, aunque la contraseña sea correcta.
	if _, err := f.svc.Login(ctx, "felipe", testPassword, "1.1.1.1", ""); !errors.Is(err, ErrLocked) {
		t.Fatalf("esperaba ErrLocked, got %v", err)
	}
	f.advance(time.Minute)
	if _, err := f.svc.Login(ctx, "felipe", testPassword, "1.1.1.1", ""); err != nil {
		t.Fatalf("tras el bloqueo: %v", err)
	}
	u, _ := f.st.GetUserByUsername(ctx, "felipe")
	if u.FailedLogins != 0 || u.LockedUntil != nil {
		t.Fatalf("el login correcto debe resetear los fallos: %+v", u)
	}
}

func TestIPRateLimit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		f.svc.Login(ctx, "nadie", "x", "6.6.6.6", "")
	}
	if _, err := f.svc.Login(ctx, "nadie", "x", "6.6.6.6", ""); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("esperaba ErrRateLimited, got %v", err)
	}
}

func TestTOTPFlow(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")

	if err := f.svc.EnableTOTP(ctx, u.ID, "123456"); !errors.Is(err, ErrNoPendingTOTP) {
		t.Fatalf("esperaba ErrNoPendingTOTP, got %v", err)
	}
	secret, url, err := f.svc.BeginTOTPSetup(ctx, u.ID)
	if err != nil || secret == "" || url == "" {
		t.Fatalf("secret=%q url=%q err=%v", secret, url, err)
	}
	if err := f.svc.EnableTOTP(ctx, u.ID, "000000"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("esperaba ErrInvalidCode, got %v", err)
	}
	code, _ := totp.GenerateCode(secret, f.now)
	if err := f.svc.EnableTOTP(ctx, u.ID, code); err != nil {
		t.Fatal(err)
	}

	// Con TOTP activo el login queda pendiente hasta el código.
	res, err := f.svc.Login(ctx, "felipe", testPassword, "1.1.1.1", "")
	if err != nil || !res.MFARequired || !res.ExpiresAt.Equal(f.now.Add(MFAPendingTTL)) {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	p, err := f.svc.AuthenticateSession(ctx, res.Token)
	if err != nil || !p.MFAPending {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	if _, err := f.svc.CompleteTOTP(ctx, res.Token, "000000", "1.1.1.1", ""); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("esperaba ErrInvalidCode, got %v", err)
	}
	code, _ = totp.GenerateCode(secret, f.now)
	full, err := f.svc.CompleteTOTP(ctx, res.Token, code, "1.1.1.1", "")
	if err != nil || full.MFARequired || full.Token == res.Token {
		t.Fatalf("full=%+v err=%v", full, err)
	}
	if _, err := f.svc.AuthenticateSession(ctx, res.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("la sesión pendiente debe dejar de existir")
	}
	p, _ = f.svc.AuthenticateSession(ctx, full.Token)
	if p.MFAPending || p.UserID != u.ID {
		t.Fatalf("p=%+v", p)
	}

	// Una sesión pendiente vence a los 5 min.
	res, _ = f.svc.Login(ctx, "felipe", testPassword, "1.1.1.1", "")
	f.advance(MFAPendingTTL)
	code, _ = totp.GenerateCode(secret, f.now)
	if _, err := f.svc.CompleteTOTP(ctx, res.Token, code, "1.1.1.1", ""); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("pendiente vencida: got %v", err)
	}
}

func TestRequire2FA(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.user(t, "felipe")
	res, _ := f.svc.Login(ctx, "felipe", testPassword, "1.1.1.1", "")

	p, _ := f.svc.AuthenticateSession(ctx, res.Token)
	if p.NeedsTOTPSetup {
		t.Fatal("sin require_2fa no se exige")
	}
	f.svc.SetRequire2FA(ctx, true)
	p, _ = f.svc.AuthenticateSession(ctx, res.Token)
	if !p.NeedsTOTPSetup {
		t.Fatal("con require_2fa y sin TOTP debe exigirlo")
	}
}

func TestInvitesAndSetup(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	token, created, err := f.svc.EnsureSetupInvite(ctx)
	if err != nil || !created || token == "" {
		t.Fatalf("token=%q created=%v err=%v", token, created, err)
	}
	// Un segundo arranque sin usuarios invalida el link anterior.
	token2, _, _ := f.svc.EnsureSetupInvite(ctx)
	if _, err := f.svc.AcceptInvite(ctx, token, "felipe", testPassword, "", ""); !errors.Is(err, store.ErrInviteInvalid) {
		t.Fatalf("el link viejo debió invalidarse, got %v", err)
	}
	res, err := f.svc.AcceptInvite(ctx, token2, "felipe", testPassword, "", "")
	if err != nil || res.Token == "" || res.UserID == 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if _, created, _ := f.svc.EnsureSetupInvite(ctx); created {
		t.Fatal("con usuarios no se crea link de setup")
	}

	inv, exp, err := f.svc.CreateInvite(ctx, &res.UserID)
	if err != nil || !exp.Equal(f.now.Add(InviteTTL)) {
		t.Fatalf("exp=%v err=%v", exp, err)
	}
	if _, err := f.svc.AcceptInvite(ctx, inv, "ana", "corta", "", ""); err == nil {
		t.Fatal("contraseña corta debió fallar")
	}
	if _, err := f.svc.AcceptInvite(ctx, inv, "ana", testPassword, "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestAPITokens(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	f.st.CreateProject(ctx, "auditfy", "Auditfy")
	f.st.CreateStage(ctx, "auditfy", "staging")

	plain, tok, err := f.svc.CreateAPIToken(ctx, "ci", "auditfy", "staging", u.ID, 30)
	if err != nil || tok.StageSlug != "staging" || tok.ExpiresAt == nil {
		t.Fatalf("tok=%+v err=%v", tok, err)
	}
	p, err := f.svc.AuthenticateToken(ctx, plain)
	if err != nil || !p.IsToken() || p.Token.ProjectSlug != "auditfy" {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	kind, id := p.Actor()
	if kind != "token" || *id != tok.ID {
		t.Fatalf("actor=%s %v", kind, id)
	}
	if _, err := f.svc.AuthenticateToken(ctx, "mk_inventado"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("got %v", err)
	}
	if _, err := f.svc.AuthenticateToken(ctx, "sin-prefijo"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("got %v", err)
	}
	f.advance(31 * 24 * time.Hour)
	if _, err := f.svc.AuthenticateToken(ctx, plain); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("token vencido: got %v", err)
	}
	if _, _, err := f.svc.CreateAPIToken(ctx, "", "auditfy", "", u.ID, 0); err == nil {
		t.Fatal("nombre vacío debió fallar")
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/auth/
```

Expected: FAIL. La primera línea relevante es:

```
internal/auth/service_test.go:10:2: no required module provides package github.com/pquerna/otp/totp; to add it:
```

- [ ] **Step 3: Implementar**

`internal/auth/service.go`:

```go
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/CPU-commits/muelle/internal/crypto"
	"github.com/CPU-commits/muelle/internal/store"
	"github.com/CPU-commits/muelle/internal/validate"
)

var (
	ErrUnauthenticated    = errors.New("auth: no autenticado")
	ErrInvalidCredentials = errors.New("auth: usuario o contraseña incorrectos")
	ErrLocked             = errors.New("auth: usuario bloqueado temporalmente")
	ErrRateLimited        = errors.New("auth: demasiados intentos desde esta IP")
	ErrInvalidCode        = errors.New("auth: código TOTP inválido")
	ErrNoPendingTOTP      = errors.New("auth: no hay configuración de 2FA pendiente")
)

const (
	SessionIdle   = 12 * time.Hour
	SessionMax    = 7 * 24 * time.Hour
	MFAPendingTTL = 5 * time.Minute
	InviteTTL     = 24 * time.Hour

	// Para no escribir en cada request, last_seen/last_used se actualizan como mucho una vez por minuto.
	touchEvery = time.Minute

	SettingRequire2FA = "require_2fa"
)

type Service struct {
	st        *store.Store
	c         *crypto.Cipher
	ipLimiter *IPLimiter
}

func NewService(st *store.Store, c *crypto.Cipher) *Service {
	now := func() time.Time { return st.Now() }
	return &Service{st: st, c: c, ipLimiter: NewIPLimiter(20, 15*time.Minute, now)}
}

// Principal es quien hace la petición: un usuario con sesión o un token de API.
type Principal struct {
	UserID         int64
	Username       string
	SessionHash    string
	MFAPending     bool // contraseña correcta, falta el código TOTP
	NeedsTOTPSetup bool // require_2fa activo y el usuario no tiene TOTP
	Token          *store.APIToken
}

func (p Principal) IsToken() bool { return p.Token != nil }

// Actor devuelve tipo e id para la auditoría.
func (p Principal) Actor() (string, *int64) {
	if p.Token != nil {
		id := p.Token.ID
		return "token", &id
	}
	if p.UserID == 0 {
		return "system", nil
	}
	id := p.UserID
	return "user", &id
}

type SessionResult struct {
	Token       string
	UserID      int64
	ExpiresAt   time.Time
	MFARequired bool
}

func (s *Service) Login(ctx context.Context, username, password, ip, ua string) (SessionResult, error) {
	if !s.ipLimiter.Allow(ip) {
		return SessionResult{}, ErrRateLimited
	}
	u, err := s.st.GetUserByUsername(ctx, username)
	if errors.Is(err, store.ErrNotFound) {
		VerifyPassword(dummyHash(), password)
		return SessionResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return SessionResult{}, err
	}
	if u.LockedUntil != nil && s.st.Now().Before(*u.LockedUntil) {
		return SessionResult{}, ErrLocked
	}
	ok, err := VerifyPassword(u.PasswordHash, password)
	if err != nil {
		return SessionResult{}, err
	}
	if !ok {
		if err := s.registerFailure(ctx, u.ID); err != nil {
			return SessionResult{}, err
		}
		return SessionResult{}, ErrInvalidCredentials
	}
	if err := s.st.ResetLoginFailures(ctx, u.ID); err != nil {
		return SessionResult{}, err
	}
	return s.newSession(ctx, u.ID, u.HasTOTP(), ip, ua)
}

// CompleteTOTP valida el código de una sesión pendiente y la cambia por una sesión completa.
func (s *Service) CompleteTOTP(ctx context.Context, pendingToken, code, ip, ua string) (SessionResult, error) {
	if !s.ipLimiter.Allow(ip) {
		return SessionResult{}, ErrRateLimited
	}
	ss, err := s.st.GetSession(ctx, HashToken(pendingToken))
	if errors.Is(err, store.ErrNotFound) {
		return SessionResult{}, ErrUnauthenticated
	}
	if err != nil {
		return SessionResult{}, err
	}
	now := s.st.Now()
	if !ss.MFAPending || !now.Before(ss.ExpiresAt) {
		return SessionResult{}, ErrUnauthenticated
	}
	u, err := s.st.GetUserByID(ctx, ss.UserID)
	if err != nil {
		return SessionResult{}, err
	}
	if u.LockedUntil != nil && now.Before(*u.LockedUntil) {
		return SessionResult{}, ErrLocked
	}
	secret, err := s.c.Decrypt(u.TOTPSecret, totpAAD(u.ID))
	if err != nil {
		return SessionResult{}, err
	}
	if !s.validCode(code, string(secret)) {
		if err := s.registerFailure(ctx, u.ID); err != nil {
			return SessionResult{}, err
		}
		return SessionResult{}, ErrInvalidCode
	}
	if err := s.st.ResetLoginFailures(ctx, u.ID); err != nil {
		return SessionResult{}, err
	}
	if err := s.st.DeleteSession(ctx, ss.IDHash); err != nil {
		return SessionResult{}, err
	}
	return s.newSession(ctx, u.ID, false, ip, ua)
}

func (s *Service) Logout(ctx context.Context, sessionToken string) error {
	return s.st.DeleteSession(ctx, HashToken(sessionToken))
}

func (s *Service) AuthenticateSession(ctx context.Context, token string) (Principal, error) {
	ss, err := s.st.GetSession(ctx, HashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, err
	}
	now := s.st.Now()
	if !now.Before(ss.ExpiresAt) || (!ss.MFAPending && now.Sub(ss.LastSeenAt) > SessionIdle) {
		if err := s.st.DeleteSession(ctx, ss.IDHash); err != nil {
			return Principal{}, err
		}
		return Principal{}, ErrUnauthenticated
	}
	u, err := s.st.GetUserByID(ctx, ss.UserID)
	if errors.Is(err, store.ErrNotFound) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, err
	}
	p := Principal{UserID: u.ID, Username: u.Username, SessionHash: ss.IDHash, MFAPending: ss.MFAPending}
	if ss.MFAPending {
		return p, nil
	}
	if now.Sub(ss.LastSeenAt) >= touchEvery {
		if err := s.st.TouchSession(ctx, ss.IDHash, now); err != nil {
			return Principal{}, err
		}
	}
	required, err := s.Require2FA(ctx)
	if err != nil {
		return Principal{}, err
	}
	p.NeedsTOTPSetup = required && !u.HasTOTP()
	return p, nil
}

func (s *Service) AuthenticateToken(ctx context.Context, bearer string) (Principal, error) {
	if !strings.HasPrefix(bearer, APITokenPrefix) {
		return Principal{}, ErrUnauthenticated
	}
	t, err := s.st.GetTokenByHash(ctx, HashToken(bearer))
	if errors.Is(err, store.ErrNotFound) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, err
	}
	now := s.st.Now()
	if t.ExpiresAt != nil && !now.Before(*t.ExpiresAt) {
		return Principal{}, ErrUnauthenticated
	}
	if t.LastUsedAt == nil || now.Sub(*t.LastUsedAt) >= touchEvery {
		if err := s.st.TouchToken(ctx, t.ID, now); err != nil {
			return Principal{}, err
		}
	}
	return Principal{Token: &t}, nil
}

func (s *Service) Require2FA(ctx context.Context) (bool, error) {
	v, ok, err := s.st.GetSetting(ctx, SettingRequire2FA)
	if err != nil || !ok {
		return false, err
	}
	return v == "true", nil
}

func (s *Service) SetRequire2FA(ctx context.Context, required bool) error {
	return s.st.SetSetting(ctx, SettingRequire2FA, fmt.Sprint(required))
}

// CreateInvite devuelve el token en claro de una invitación nueva. createdBy nil = setup.
func (s *Service) CreateInvite(ctx context.Context, createdBy *int64) (string, time.Time, error) {
	plain, hash := RandomToken("")
	expires := s.st.Now().Add(InviteTTL)
	if err := s.st.CreateInvite(ctx, hash, createdBy, expires); err != nil {
		return "", time.Time{}, err
	}
	return plain, expires, nil
}

// EnsureSetupInvite crea un link de setup nuevo si todavía no hay usuarios.
func (s *Service) EnsureSetupInvite(ctx context.Context) (string, bool, error) {
	n, err := s.st.CountUsers(ctx)
	if err != nil || n > 0 {
		return "", false, err
	}
	if err := s.st.DeleteUnusedSystemInvites(ctx); err != nil {
		return "", false, err
	}
	token, _, err := s.CreateInvite(ctx, nil)
	if err != nil {
		return "", false, err
	}
	return token, true, nil
}

// AcceptInvite crea el usuario de la invitación y le abre una sesión.
func (s *Service) AcceptInvite(ctx context.Context, token, username, password, ip, ua string) (SessionResult, error) {
	if err := validate.Username(username); err != nil {
		return SessionResult{}, err
	}
	if err := validate.Password(password); err != nil {
		return SessionResult{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return SessionResult{}, err
	}
	u, err := s.st.AcceptInvite(ctx, HashToken(token), username, hash)
	if err != nil {
		return SessionResult{}, err
	}
	return s.newSession(ctx, u.ID, false, ip, ua)
}

// BeginTOTPSetup genera un secreto nuevo y lo deja pendiente hasta EnableTOTP.
func (s *Service) BeginTOTPSetup(ctx context.Context, userID int64) (secret, url string, err error) {
	u, err := s.st.GetUserByID(ctx, userID)
	if err != nil {
		return "", "", err
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "muelle", AccountName: u.Username})
	if err != nil {
		return "", "", err
	}
	if err := s.st.SetTOTPPending(ctx, u.ID, s.c.Encrypt([]byte(key.Secret()), totpAAD(u.ID))); err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

func (s *Service) EnableTOTP(ctx context.Context, userID int64, code string) error {
	u, err := s.st.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if len(u.TOTPPending) == 0 {
		return ErrNoPendingTOTP
	}
	secret, err := s.c.Decrypt(u.TOTPPending, totpAAD(u.ID))
	if err != nil {
		return err
	}
	if !s.validCode(code, string(secret)) {
		return ErrInvalidCode
	}
	return s.st.ActivateTOTP(ctx, u.ID)
}

func (s *Service) ResetTOTP(ctx context.Context, userID int64) error {
	return s.st.ClearTOTP(ctx, userID)
}

// CreateAPIToken crea un token con alcance de proyecto (stage vacío) o de stage.
// expiresInDays 0 = no vence. Devuelve el token en claro: se muestra una sola vez.
func (s *Service) CreateAPIToken(ctx context.Context, name, project, stage string, createdBy int64, expiresInDays int) (string, store.APIToken, error) {
	if err := validate.Name("name", name, 64); err != nil {
		return "", store.APIToken{}, err
	}
	if expiresInDays < 0 || expiresInDays > 3650 {
		return "", store.APIToken{}, validate.Errorf("expires_in_days", "debe estar entre 0 y 3650")
	}
	var expires *time.Time
	if expiresInDays > 0 {
		t := s.st.Now().AddDate(0, 0, expiresInDays)
		expires = &t
	}
	plain, hash := RandomToken(APITokenPrefix)
	t, err := s.st.CreateToken(ctx, store.NewAPIToken{Name: name, Hash: hash, ProjectSlug: project,
		StageSlug: stage, CreatedBy: &createdBy, ExpiresAt: expires})
	if err != nil {
		return "", store.APIToken{}, err
	}
	return plain, t, nil
}

func (s *Service) newSession(ctx context.Context, userID int64, pending bool, ip, ua string) (SessionResult, error) {
	plain, hash := RandomToken("")
	now := s.st.Now()
	ttl := SessionMax
	if pending {
		ttl = MFAPendingTTL
	}
	if len(ua) > 256 {
		ua = ua[:256]
	}
	ss := store.Session{IDHash: hash, UserID: userID, MFAPending: pending, CreatedAt: now, LastSeenAt: now,
		ExpiresAt: now.Add(ttl), UserAgent: ua, IP: ip}
	if err := s.st.CreateSession(ctx, ss); err != nil {
		return SessionResult{}, err
	}
	return SessionResult{Token: plain, UserID: userID, ExpiresAt: ss.ExpiresAt, MFARequired: pending}, nil
}

func (s *Service) registerFailure(ctx context.Context, userID int64) error {
	n, err := s.st.IncrementFailedLogins(ctx, userID)
	if err != nil {
		return err
	}
	if d := LockoutFor(n); d > 0 {
		return s.st.SetLockedUntil(ctx, userID, s.st.Now().Add(d))
	}
	return nil
}

func (s *Service) validCode(code, secret string) bool {
	ok, err := totp.ValidateCustom(code, secret, s.st.Now(), totp.ValidateOpts{
		Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && ok
}

// totpSecret y totpPending usan la misma AAD: ActivateTOTP mueve el valor sin descifrarlo.
func totpAAD(userID int64) string { return fmt.Sprintf("users:totp:%d", userID) }
```

Agregar las dependencias:

```bash
go get github.com/pquerna/otp@v1.5.0
go mod tidy
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/auth/ && go test ./internal/auth/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/auth
```

- [ ] **Step 5: Commit**

```bash
git add internal/auth/service.go internal/auth/service_test.go go.mod go.sum
git commit -m "feat(auth): login, 2FA en dos pasos, invitaciones y tokens de API" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 18: API: base, middleware y endpoints de sesión

La base HTTP. Puntos clave:
- `ServeHTTP` rechaza con 403 `bad_origin` toda petición que modifica algo, no trae `Authorization: Bearer` y cuyo `Origin` no es exactamente el del panel (protección CSRF).
- `handle(pattern, access, h)` registra rutas autenticadas con cuatro niveles de acceso: `accessSession`, `accessToken`, `accessSetup` y `accessMFAPending`.
- `tokenInScope` es la regla de alcance de los tokens.
- `fail` traduce los errores conocidos al formato `{"error": {"code", "message"}}` de §10.
- `clientIP` lee `X-Real-IP`, que pondrá Caddy en la Fase 3.

Esta tarea registra solo los endpoints de `/auth`; las siguientes agregan una línea cada una a `routes()`.

**Files:**
- Create: `internal/api/respond.go`
- Create: `internal/api/middleware.go`
- Create: `internal/api/server.go`
- Create: `internal/api/auth_handlers.go`
- Test: `internal/api/helpers_test.go`
- Test: `internal/api/auth_handlers_test.go`

- [ ] **Step 1: Escribir los tests**

`internal/api/helpers_test.go`:

```go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/CPU-commits/muelle/internal/auth"
	"github.com/CPU-commits/muelle/internal/crypto"
	"github.com/CPU-commits/muelle/internal/envvars"
	"github.com/CPU-commits/muelle/internal/store"
)

const (
	testOrigin   = "https://panel.test"
	testPassword = "una-contraseña-larga"
)

type testEnv struct {
	t    *testing.T
	st   *store.Store
	auth *auth.Service
	srv  *Server
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	c, _ := crypto.New(make([]byte, crypto.KeySize))
	a := auth.NewService(st, c)
	srv := New(Deps{Store: st, Auth: a, Env: envvars.NewService(st, c), PanelOrigin: testOrigin,
		SecureCookies: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	return &testEnv{t: t, st: st, auth: a, srv: srv}
}

// opt modifica la petición de prueba.
type opt func(*http.Request)

func withSession(token string) opt {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: sessionCookie, Value: token}) }
}

func withBearer(token string) opt {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
}

func withOrigin(origin string) opt {
	return func(r *http.Request) {
		if origin == "" {
			r.Header.Del("Origin")
		} else {
			r.Header.Set("Origin", origin)
		}
	}
}

// do hace una petición. Por defecto lleva el Origin del panel.
func (e *testEnv) do(method, path string, body any, opts ...opt) *httptest.ResponseRecorder {
	e.t.Helper()
	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		reader = bytes.NewBufferString(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			e.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	r := httptest.NewRequest(method, path, reader)
	r.Header.Set("Origin", testOrigin)
	if reader != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	e.srv.ServeHTTP(w, r)
	return w
}

// user crea un usuario directo en el store.
func (e *testEnv) user(name string) store.User {
	e.t.Helper()
	h, _ := auth.HashPassword(testPassword)
	u, err := e.st.CreateUser(context.Background(), name, h, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	return u
}

// login crea el usuario "admin" (si no existe) y devuelve un token de sesión completa.
func (e *testEnv) login() string {
	e.t.Helper()
	if _, err := e.st.GetUserByUsername(context.Background(), "admin"); err != nil {
		e.user("admin")
	}
	res, err := e.auth.Login(context.Background(), "admin", testPassword, "127.0.0.1", "test")
	if err != nil {
		e.t.Fatal(err)
	}
	return res.Token
}

func sessionFrom(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			return c.Value
		}
	}
	t.Fatalf("la respuesta no trae cookie de sesión: %v", w.Result().Header)
	return ""
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("JSON inválido (%d): %s", w.Code, w.Body.String())
	}
	return v
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	return decode[errorBody](t, w).Error.Code
}

func expect(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d, esperaba %d: %s", w.Code, status, w.Body.String())
	}
}
```

`internal/api/auth_handlers_test.go`:

```go
package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestSetupCreatesFirstUser(t *testing.T) {
	e := newTestEnv(t)
	token, _, err := e.auth.EnsureSetupInvite(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	w := e.do("POST", "/api/v1/auth/setup", map[string]string{"token": token, "username": "felipe", "password": "corta"})
	expect(t, w, http.StatusUnprocessableEntity)

	w = e.do("POST", "/api/v1/auth/setup", map[string]string{"token": token, "username": "felipe", "password": testPassword})
	expect(t, w, http.StatusCreated)
	session := sessionFrom(t, w)

	w = e.do("GET", "/api/v1/auth/me", nil, withSession(session))
	expect(t, w, http.StatusOK)
	if me := decode[map[string]any](t, w); me["username"] != "felipe" || me["totp_enabled"] != false {
		t.Fatalf("me=%v", me)
	}

	// Con un usuario creado, setup ya no funciona.
	w = e.do("POST", "/api/v1/auth/setup", map[string]string{"token": token, "username": "otro", "password": testPassword})
	expect(t, w, http.StatusConflict)
	if code := errorCode(t, w); code != "already_set_up" {
		t.Fatalf("code=%s", code)
	}
}

func TestSessionCookieFlags(t *testing.T) {
	e := newTestEnv(t)
	e.user("admin")
	w := e.do("POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": testPassword})
	expect(t, w, http.StatusOK)
	c := w.Result().Cookies()[0]
	if c.Name != sessionCookie || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
		t.Fatalf("cookie=%+v", c)
	}
	if decode[map[string]bool](t, w)["mfa_required"] {
		t.Fatal("sin TOTP no se pide código")
	}
}

func TestLoginErrors(t *testing.T) {
	e := newTestEnv(t)
	e.user("admin")
	w := e.do("POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": "mala-contraseña-x"})
	expect(t, w, http.StatusUnauthorized)
	if code := errorCode(t, w); code != "invalid_credentials" {
		t.Fatalf("code=%s", code)
	}
	w = e.do("POST", "/api/v1/auth/login", `{"username":"admin","password":"x","extra":1}`)
	expect(t, w, http.StatusBadRequest)
}

func TestCSRFOriginCheck(t *testing.T) {
	e := newTestEnv(t)
	e.user("admin")
	body := map[string]string{"username": "admin", "password": testPassword}

	w := e.do("POST", "/api/v1/auth/login", body, withOrigin(""))
	expect(t, w, http.StatusForbidden)
	w = e.do("POST", "/api/v1/auth/login", body, withOrigin("https://evil.example"))
	expect(t, w, http.StatusForbidden)
	if code := errorCode(t, w); code != "bad_origin" {
		t.Fatalf("code=%s", code)
	}
	// GET no necesita Origin.
	session := e.login()
	w = e.do("GET", "/api/v1/auth/me", nil, withSession(session), withOrigin(""))
	expect(t, w, http.StatusOK)
}

func TestLogout(t *testing.T) {
	e := newTestEnv(t)
	session := e.login()
	w := e.do("POST", "/api/v1/auth/logout", nil, withSession(session))
	expect(t, w, http.StatusNoContent)
	w = e.do("GET", "/api/v1/auth/me", nil, withSession(session))
	expect(t, w, http.StatusUnauthorized)
	w = e.do("GET", "/api/v1/auth/me", nil)
	expect(t, w, http.StatusUnauthorized)
}

func TestTOTPEndToEnd(t *testing.T) {
	e := newTestEnv(t)
	session := e.login()

	w := e.do("POST", "/api/v1/auth/totp/setup", nil, withSession(session))
	expect(t, w, http.StatusOK)
	secret := decode[map[string]string](t, w)["secret"]
	code, _ := totp.GenerateCode(secret, time.Now())
	w = e.do("POST", "/api/v1/auth/totp/enable", map[string]string{"code": code}, withSession(session))
	expect(t, w, http.StatusNoContent)

	// Login nuevo: queda pendiente del código.
	w = e.do("POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": testPassword})
	expect(t, w, http.StatusOK)
	if !decode[map[string]bool](t, w)["mfa_required"] {
		t.Fatal("con TOTP debe pedir código")
	}
	pending := sessionFrom(t, w)

	// La sesión pendiente no sirve para otra cosa.
	w = e.do("GET", "/api/v1/auth/me", nil, withSession(pending))
	expect(t, w, http.StatusUnauthorized)
	if c := errorCode(t, w); c != "mfa_required" {
		t.Fatalf("code=%s", c)
	}
	// Una sesión completa no puede usar el endpoint del código.
	w = e.do("POST", "/api/v1/auth/totp", map[string]string{"code": "000000"}, withSession(session))
	expect(t, w, http.StatusBadRequest)

	w = e.do("POST", "/api/v1/auth/totp", map[string]string{"code": "000000"}, withSession(pending))
	expect(t, w, http.StatusUnauthorized)
	code, _ = totp.GenerateCode(secret, time.Now())
	w = e.do("POST", "/api/v1/auth/totp", map[string]string{"code": code}, withSession(pending))
	expect(t, w, http.StatusOK)
	full := sessionFrom(t, w)
	w = e.do("GET", "/api/v1/auth/me", nil, withSession(full))
	expect(t, w, http.StatusOK)
	if me := decode[map[string]any](t, w); me["totp_enabled"] != true {
		t.Fatalf("me=%v", me)
	}
}

func TestRequire2FABlocksUntilConfigured(t *testing.T) {
	e := newTestEnv(t)
	session := e.login()
	e.auth.SetRequire2FA(context.Background(), true)

	w := e.do("GET", "/api/v1/auth/me", nil, withSession(session))
	expect(t, w, http.StatusOK)
	if me := decode[map[string]any](t, w); me["needs_totp_setup"] != true {
		t.Fatalf("me=%v", me)
	}
	w = e.do("POST", "/api/v1/auth/totp/setup", nil, withSession(session))
	expect(t, w, http.StatusOK)
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/api/
```

Expected: FAIL. La primera línea relevante es:

```
internal/api/helpers_test.go:29:8: undefined: Server
```

- [ ] **Step 3: Implementar**

`internal/api/respond.go`:

```go
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/CPU-commits/muelle/internal/auth"
	"github.com/CPU-commits/muelle/internal/store"
	"github.com/CPU-commits/muelle/internal/validate"
)

const maxBodyBytes = 1 << 20

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError usa el formato del spec: {"error": {"code": "...", "message": "..."}}.
func writeError(w http.ResponseWriter, status int, code, message string) {
	var body errorBody
	body.Error.Code = code
	body.Error.Message = message
	writeJSON(w, status, body)
}

// decodeJSON lee el body (máximo limit bytes, sin campos desconocidos).
// Si falla responde 400 y devuelve false.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, limit int64) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", fmt.Sprintf("JSON inválido: %v", err))
		return false
	}
	return true
}

// fail traduce los errores conocidos a respuestas HTTP. El resto es 500.
func (s *Server) fail(w http.ResponseWriter, err error) {
	var ve *validate.Error
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusUnprocessableEntity, "invalid", ve.Error())
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no existe")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "ya existe")
	case errors.Is(err, store.ErrNotEmpty):
		writeError(w, http.StatusConflict, "not_empty", "primero hay que borrar lo que contiene")
	case errors.Is(err, store.ErrKindMismatch):
		writeError(w, http.StatusUnprocessableEntity, "kind_mismatch", "el campo no corresponde al tipo de la app")
	case errors.Is(err, store.ErrInviteInvalid):
		writeError(w, http.StatusBadRequest, "invalid_invite", "la invitación no existe, ya se usó o venció")
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "usuario o contraseña incorrectos")
	case errors.Is(err, auth.ErrLocked):
		writeError(w, http.StatusLocked, "locked", "usuario bloqueado temporalmente por intentos fallidos")
	case errors.Is(err, auth.ErrRateLimited):
		writeError(w, http.StatusTooManyRequests, "rate_limited", "demasiados intentos, espera unos minutos")
	case errors.Is(err, auth.ErrInvalidCode):
		writeError(w, http.StatusUnauthorized, "invalid_code", "código incorrecto")
	case errors.Is(err, auth.ErrNoPendingTOTP):
		writeError(w, http.StatusConflict, "no_pending_totp", "primero inicia la configuración de 2FA")
	case errors.Is(err, auth.ErrUnauthenticated):
		writeError(w, http.StatusUnauthorized, "unauthenticated", "inicia sesión")
	default:
		s.log.Error("error interno", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "error interno")
	}
}
```

`internal/api/middleware.go`:

```go
package api

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/CPU-commits/muelle/internal/auth"
	"github.com/CPU-commits/muelle/internal/store"
)

// access define quién puede llamar a un endpoint.
type access int

const (
	accessSession    access = iota // solo sesión completa
	accessToken                    // sesión completa o token de API dentro de su alcance
	accessSetup                    // sesión completa, incluida la que todavía debe configurar 2FA
	accessMFAPending               // solo la sesión que espera el código TOTP
)

type principalKey struct{}

func withPrincipal(ctx context.Context, p auth.Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func principal(r *http.Request) auth.Principal {
	p, _ := r.Context().Value(principalKey{}).(auth.Principal)
	return p
}

// handle registra un endpoint autenticado.
func (s *Server) handle(pattern string, a access, h http.HandlerFunc) {
	s.mux.Handle(pattern, s.authenticate(a, h))
}

func (s *Server) authenticate(a access, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if bearer, ok := bearerToken(r); ok {
			if a != accessToken {
				writeError(w, http.StatusForbidden, "token_not_allowed", "los tokens de API no pueden usar este endpoint")
				return
			}
			p, err := s.auth.AuthenticateToken(ctx, bearer)
			if err != nil {
				s.fail(w, err)
				return
			}
			if !tokenInScope(p.Token, r) {
				writeError(w, http.StatusForbidden, "out_of_scope", "el token no tiene acceso a este recurso")
				return
			}
			next(w, r.WithContext(withPrincipal(ctx, p)))
			return
		}

		c, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "inicia sesión")
			return
		}
		p, err := s.auth.AuthenticateSession(ctx, c.Value)
		if err != nil {
			s.fail(w, err)
			return
		}
		switch {
		case p.MFAPending && a != accessMFAPending:
			writeError(w, http.StatusUnauthorized, "mfa_required", "falta el código de 2FA")
			return
		case !p.MFAPending && a == accessMFAPending:
			writeError(w, http.StatusBadRequest, "no_mfa_pending", "esta sesión no espera un código")
			return
		case p.NeedsTOTPSetup && a != accessSetup:
			writeError(w, http.StatusForbidden, "totp_setup_required", "configura 2FA para continuar")
			return
		}
		next(w, r.WithContext(withPrincipal(ctx, p)))
	})
}

// tokenInScope: el token debe ser del proyecto {p}. Si está limitado a un
// stage, solo puede tocar ese stage; en rutas sin {s} solo puede leer.
func tokenInScope(t *store.APIToken, r *http.Request) bool {
	if r.PathValue("p") != t.ProjectSlug {
		return false
	}
	if t.StageSlug == "" {
		return true
	}
	stage := r.PathValue("s")
	if stage == "" {
		return r.Method == http.MethodGet
	}
	return stage == t.StageSlug
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(h, "Bearer ")
	return token, ok && token != ""
}

// clientIP usa X-Real-IP, que pone Caddy (único que llega a muelle en producción, §8 del spec).
func clientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
```

`internal/api/server.go`:

```go
// Package api expone la API REST /api/v1 que usan el panel y GitHub Actions.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/CPU-commits/muelle/internal/auth"
	"github.com/CPU-commits/muelle/internal/envvars"
	"github.com/CPU-commits/muelle/internal/store"
)

const sessionCookie = "muelle_session"

type Deps struct {
	Store         *store.Store
	Auth          *auth.Service
	Env           *envvars.Service
	PanelOrigin   string // ej. "https://panel.tudominio.cl"
	SecureCookies bool
	Logger        *slog.Logger
}

type Server struct {
	st            *store.Store
	auth          *auth.Service
	env           *envvars.Service
	origin        string
	secureCookies bool
	log           *slog.Logger
	mux           *http.ServeMux
}

func New(d Deps) *Server {
	log := d.Logger
	if log == nil {
		log = slog.Default()
	}
	s := &Server{st: d.Store, auth: d.Auth, env: d.Env, origin: d.PanelOrigin,
		secureCookies: d.SecureCookies, log: log, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Cache-Control", "no-store")
	}
	if !s.originAllowed(r) {
		writeError(w, http.StatusForbidden, "bad_origin", "origen no permitido")
		return
	}
	s.mux.ServeHTTP(w, r)
}

// originAllowed protege contra CSRF: toda petición que modifica algo y no
// trae un token Bearer tiene que venir del origen exacto del panel.
func (s *Server) originAllowed(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	if _, ok := bearerToken(r); ok {
		return true
	}
	return r.Header.Get("Origin") == s.origin
}

func (s *Server) routes() {
	// Sesión (§10 del spec)
	s.mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /api/v1/auth/setup", s.handleSetup)
	s.mux.HandleFunc("POST /api/v1/auth/invite/{token}", s.handleAcceptInvite)
	s.handle("POST /api/v1/auth/totp", accessMFAPending, s.handleTOTPLogin)
	s.handle("POST /api/v1/auth/logout", accessSetup, s.handleLogout)
	s.handle("GET /api/v1/auth/me", accessSetup, s.handleMe)
	s.handle("POST /api/v1/auth/totp/setup", accessSetup, s.handleTOTPSetup)
	s.handle("POST /api/v1/auth/totp/enable", accessSetup, s.handleTOTPEnable)
}

// audit registra un evento a nombre de quien hace la petición. Nunca incluye valores secretos.
func (s *Server) audit(r *http.Request, action, target string, details any) {
	actorType, actorID := principal(r).Actor()
	var raw json.RawMessage
	if details != nil {
		raw, _ = json.Marshal(details)
	}
	err := s.st.AddAudit(r.Context(), store.AuditEvent{ActorType: actorType, ActorID: actorID,
		Action: action, Target: target, Details: raw})
	if err != nil {
		s.log.Error("no se pudo registrar la auditoría", "action", action, "err", err)
	}
}
```

`internal/api/auth_handlers.go`:

```go
package api

import (
	"net/http"

	"github.com/CPU-commits/muelle/internal/auth"
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type codeInput struct {
	Code string `json:"code"`
}

func (s *Server) setSessionCookie(w http.ResponseWriter, res auth.SessionResult) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: res.Token, Path: "/", Expires: res.ExpiresAt,
		HttpOnly: true, Secure: s.secureCookies, SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.secureCookies, SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !decodeJSON(w, r, &in, maxBodyBytes) {
		return
	}
	res, err := s.auth.Login(r.Context(), in.Username, in.Password, clientIP(r), r.UserAgent())
	if err != nil {
		s.fail(w, err)
		return
	}
	s.setSessionCookie(w, res)
	writeJSON(w, http.StatusOK, map[string]bool{"mfa_required": res.MFARequired})
}

func (s *Server) handleTOTPLogin(w http.ResponseWriter, r *http.Request) {
	var in codeInput
	if !decodeJSON(w, r, &in, maxBodyBytes) {
		return
	}
	c, _ := r.Cookie(sessionCookie) // authenticate ya verificó que existe
	res, err := s.auth.CompleteTOTP(r.Context(), c.Value, in.Code, clientIP(r), r.UserAgent())
	if err != nil {
		s.fail(w, err)
		return
	}
	s.setSessionCookie(w, res)
	writeJSON(w, http.StatusOK, map[string]bool{"mfa_required": false})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie(sessionCookie)
	if err := s.auth.Logout(r.Context(), c.Value); err != nil {
		s.fail(w, err)
		return
	}
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	u, err := s.st.GetUserByID(r.Context(), p.UserID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"username": u.Username, "totp_enabled": u.HasTOTP(), "needs_totp_setup": p.NeedsTOTPSetup,
	})
}

func (s *Server) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	secret, url, err := s.auth.BeginTOTPSetup(r.Context(), principal(r).UserID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret, "url": url})
}

func (s *Server) handleTOTPEnable(w http.ResponseWriter, r *http.Request) {
	var in codeInput
	if !decodeJSON(w, r, &in, maxBodyBytes) {
		return
	}
	p := principal(r)
	if err := s.auth.EnableTOTP(r.Context(), p.UserID, in.Code); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "user.totp_enable", "user:"+p.Username, nil)
	w.WriteHeader(http.StatusNoContent)
}

type setupInput struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleSetup crea el primer usuario con el link que imprime `muelle serve`.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var in setupInput
	if !decodeJSON(w, r, &in, maxBodyBytes) {
		return
	}
	n, err := s.st.CountUsers(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	if n > 0 {
		writeError(w, http.StatusConflict, "already_set_up", "muelle ya tiene usuarios; usa una invitación")
		return
	}
	s.acceptInvite(w, r, in.Token, in.Username, in.Password)
}

func (s *Server) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !decodeJSON(w, r, &in, maxBodyBytes) {
		return
	}
	s.acceptInvite(w, r, r.PathValue("token"), in.Username, in.Password)
}

func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request, token, username, password string) {
	res, err := s.auth.AcceptInvite(r.Context(), token, username, password, clientIP(r), r.UserAgent())
	if err != nil {
		s.fail(w, err)
		return
	}
	r = r.WithContext(withPrincipal(r.Context(), auth.Principal{UserID: res.UserID, Username: username}))
	s.audit(r, "user.create", "user:"+username, nil)
	s.setSessionCookie(w, res)
	writeJSON(w, http.StatusCreated, map[string]string{"username": username})
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/api/ && go test ./internal/api/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/api
```

- [ ] **Step 5: Commit**

```bash
git add internal/api/respond.go internal/api/middleware.go internal/api/server.go internal/api/auth_handlers.go internal/api/helpers_test.go internal/api/auth_handlers_test.go
git commit -m "feat(api): base HTTP, CSRF por Origin, sesiones y 2FA" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 19: API: proyectos y stages

CRUD de proyectos y stages. Los tokens pueden **leer** el proyecto y sus stages (`accessToken`); crear y borrar requiere sesión. Crear un stage solo lo guarda: la Fase 3 le agrega la red overlay y la conexión de Caddy.

**Files:**
- Create: `internal/api/projects_handlers.go`
- Modify: `internal/api/server.go` (una línea en `routes()`)
- Test: `internal/api/projects_handlers_test.go`

- [ ] **Step 1: Escribir el test**

`internal/api/projects_handlers_test.go`:

```go
package api

import (
	"context"
	"net/http"
	"testing"
)

func TestProjectsAndStagesAPI(t *testing.T) {
	e := newTestEnv(t)
	s := withSession(e.login())

	expect(t, e.do("GET", "/api/v1/projects", nil), http.StatusUnauthorized)

	w := e.do("POST", "/api/v1/projects", map[string]string{"slug": "Auditfy", "name": "Auditfy"}, s)
	expect(t, w, http.StatusUnprocessableEntity)
	w = e.do("POST", "/api/v1/projects", map[string]string{"slug": "auditfy", "name": ""}, s)
	expect(t, w, http.StatusUnprocessableEntity)
	w = e.do("POST", "/api/v1/projects", map[string]string{"slug": "auditfy", "name": "Auditfy"}, s)
	expect(t, w, http.StatusCreated)
	if p := decode[map[string]any](t, w); p["slug"] != "auditfy" || p["id"] != nil {
		t.Fatalf("project=%v (el id interno no se expone)", p)
	}
	expect(t, e.do("POST", "/api/v1/projects", map[string]string{"slug": "auditfy", "name": "X"}, s), http.StatusConflict)

	w = e.do("GET", "/api/v1/projects", nil, s)
	expect(t, w, http.StatusOK)
	if list := decode[[]map[string]any](t, w); len(list) != 1 {
		t.Fatalf("list=%v", list)
	}
	w = e.do("PATCH", "/api/v1/projects/auditfy", map[string]string{"name": "Auditfy SpA"}, s)
	expect(t, w, http.StatusOK)
	expect(t, e.do("GET", "/api/v1/projects/nope", nil, s), http.StatusNotFound)

	// Stages
	expect(t, e.do("POST", "/api/v1/projects/auditfy/stages", map[string]string{"slug": "staging_1"}, s), http.StatusUnprocessableEntity)
	w = e.do("POST", "/api/v1/projects/auditfy/stages", map[string]string{"slug": "staging"}, s)
	expect(t, w, http.StatusCreated)
	if st := decode[map[string]any](t, w); st["project"] != "auditfy" || st["slug"] != "staging" {
		t.Fatalf("stage=%v", st)
	}
	expect(t, e.do("POST", "/api/v1/projects/nope/stages", map[string]string{"slug": "staging"}, s), http.StatusNotFound)
	w = e.do("GET", "/api/v1/projects/auditfy/stages", nil, s)
	expect(t, w, http.StatusOK)
	expect(t, e.do("GET", "/api/v1/projects/auditfy/stages/staging", nil, s), http.StatusOK)

	// No se borra un proyecto con stages.
	w = e.do("DELETE", "/api/v1/projects/auditfy", nil, s)
	expect(t, w, http.StatusConflict)
	if code := errorCode(t, w); code != "not_empty" {
		t.Fatalf("code=%s", code)
	}
	expect(t, e.do("DELETE", "/api/v1/projects/auditfy/stages/staging", nil, s), http.StatusNoContent)
	expect(t, e.do("DELETE", "/api/v1/projects/auditfy", nil, s), http.StatusNoContent)

	// Todo quedó en la auditoría.
	events, _ := e.st.ListAudit(context.Background(), 0, 10)
	want := []string{"project.delete", "stage.delete", "stage.create", "project.update", "project.create"}
	if len(events) != len(want) {
		t.Fatalf("events=%+v", events)
	}
	for i, a := range want {
		if events[i].Action != a || events[i].ActorType != "user" {
			t.Errorf("evento %d = %s/%s, esperaba %s", i, events[i].ActorType, events[i].Action, a)
		}
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/api/
```

Expected: FAIL. La primera línea relevante es:

```
--- FAIL: TestProjectsAndStagesAPI (0.12s)
    projects_handlers_test.go:13: status 404, esperaba 401: 404 page not found
```

- [ ] **Step 3: Implementar**

`internal/api/projects_handlers.go`:

```go
package api

import (
	"net/http"

	"github.com/CPU-commits/muelle/internal/validate"
)

func (s *Server) projectRoutes() {
	s.handle("GET /api/v1/projects", accessSession, s.handleListProjects)
	s.handle("POST /api/v1/projects", accessSession, s.handleCreateProject)
	s.handle("GET /api/v1/projects/{p}", accessToken, s.handleGetProject)
	s.handle("PATCH /api/v1/projects/{p}", accessSession, s.handleUpdateProject)
	s.handle("DELETE /api/v1/projects/{p}", accessSession, s.handleDeleteProject)

	s.handle("GET /api/v1/projects/{p}/stages", accessToken, s.handleListStages)
	s.handle("POST /api/v1/projects/{p}/stages", accessSession, s.handleCreateStage)
	s.handle("GET /api/v1/projects/{p}/stages/{s}", accessToken, s.handleGetStage)
	s.handle("DELETE /api/v1/projects/{p}/stages/{s}", accessSession, s.handleDeleteStage)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	ps, err := s.st.ListProjects(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &in, maxBodyBytes) {
		return
	}
	if err := validate.ProjectSlug(in.Slug); err != nil {
		s.fail(w, err)
		return
	}
	if err := validate.Name("name", in.Name, 64); err != nil {
		s.fail(w, err)
		return
	}
	p, err := s.st.CreateProject(r.Context(), in.Slug, in.Name)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "project.create", "project:"+p.Slug, nil)
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	p, err := s.st.GetProject(r.Context(), r.PathValue("p"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &in, maxBodyBytes) {
		return
	}
	if err := validate.Name("name", in.Name, 64); err != nil {
		s.fail(w, err)
		return
	}
	p, err := s.st.UpdateProjectName(r.Context(), r.PathValue("p"), in.Name)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "project.update", "project:"+p.Slug, nil)
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("p")
	if err := s.st.DeleteProject(r.Context(), slug); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "project.delete", "project:"+slug, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListStages(w http.ResponseWriter, r *http.Request) {
	stages, err := s.st.ListStages(r.Context(), r.PathValue("p"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stages)
}

// handleCreateStage solo guarda el stage. La Fase 3 agrega la red overlay y conecta Caddy.
func (s *Server) handleCreateStage(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Slug string `json:"slug"`
	}
	if !decodeJSON(w, r, &in, maxBodyBytes) {
		return
	}
	if err := validate.StageSlug(in.Slug); err != nil {
		s.fail(w, err)
		return
	}
	st, err := s.st.CreateStage(r.Context(), r.PathValue("p"), in.Slug)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "stage.create", "stage:"+st.ProjectSlug+"/"+st.Slug, nil)
	writeJSON(w, http.StatusCreated, st)
}

func (s *Server) handleGetStage(w http.ResponseWriter, r *http.Request) {
	st, err := s.st.GetStage(r.Context(), r.PathValue("p"), r.PathValue("s"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleDeleteStage(w http.ResponseWriter, r *http.Request) {
	p, slug := r.PathValue("p"), r.PathValue("s")
	if err := s.st.DeleteStage(r.Context(), p, slug); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "stage.delete", "stage:"+p+"/"+slug, nil)
	w.WriteHeader(http.StatusNoContent)
}
```

En `internal/api/server.go`, al final de `routes()`, agregar `s.projectRoutes()` después de una línea en blanco. Queda así:

```go
func (s *Server) routes() {
	// …
	s.handle("POST /api/v1/auth/totp/enable", accessSetup, s.handleTOTPEnable)

	s.projectRoutes()
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/api/ && go test ./internal/api/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/api
```

- [ ] **Step 5: Commit**

```bash
git add internal/api/projects_handlers.go internal/api/projects_handlers_test.go internal/api/server.go
git commit -m "feat(api): proyectos y stages" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 20: API: apps

Crear y editar apps. Las de formulario pasan por `appspec.Parse` y se guarda el spec **normalizado**. Las compose, en esta fase, solo se validan como no vacías y de hasta 256 KiB; la Fase 2 agrega la validación con `compose-go`. La respuesta incluye `pending_changes`.

**Files:**
- Create: `internal/api/apps_handlers.go`
- Modify: `internal/api/server.go` (una línea en `routes()`)
- Test: `internal/api/apps_handlers_test.go`

- [ ] **Step 1: Escribir el test**

`internal/api/apps_handlers_test.go`:

```go
package api

import (
	"context"
	"net/http"
	"testing"
)

// seedStage crea auditfy/staging por el store y devuelve la opción de sesión.
func (e *testEnv) seedStage() opt {
	e.t.Helper()
	ctx := context.Background()
	if _, err := e.st.CreateProject(ctx, "auditfy", "Auditfy"); err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.st.CreateStage(ctx, "auditfy", "staging"); err != nil {
		e.t.Fatal(err)
	}
	return withSession(e.login())
}

const appsPath = "/api/v1/projects/auditfy/stages/staging/apps"

func TestCreateServiceApp(t *testing.T) {
	e := newTestEnv(t)
	s := e.seedStage()

	w := e.do("POST", appsPath, map[string]any{
		"slug": "api", "kind": "service",
		"spec": map[string]any{"image": "ghcr.io/aumi-app/backend-app:staging", "port": 3000,
			"healthcheck": map[string]any{"type": "http", "path": "/health"}},
	}, s)
	expect(t, w, http.StatusCreated)
	app := decode[map[string]any](t, w)
	spec := app["spec"].(map[string]any)
	if app["kind"] != "service" || spec["replicas"] != float64(1) || app["pending_changes"] != false || app["deploy_timeout_s"] != float64(300) {
		t.Fatalf("app=%v", app)
	}
	if spec["healthcheck"].(map[string]any)["interval_s"] != float64(10) {
		t.Fatalf("el spec guardado debe venir normalizado: %v", spec)
	}

	expect(t, e.do("POST", appsPath, map[string]any{"slug": "api", "kind": "service",
		"spec": map[string]any{"image": "x", "port": 1}}, s), http.StatusConflict)
	expect(t, e.do("POST", appsPath, map[string]any{"slug": "bad", "kind": "service",
		"spec": map[string]any{"image": "x", "port": 0}}, s), http.StatusUnprocessableEntity)
	expect(t, e.do("POST", appsPath, map[string]any{"slug": "bad", "kind": "service"}, s), http.StatusUnprocessableEntity)
	expect(t, e.do("POST", appsPath, map[string]any{"slug": "bad", "kind": "service", "compose_yaml": "x",
		"spec": map[string]any{"image": "x", "port": 1}}, s), http.StatusUnprocessableEntity)
	expect(t, e.do("POST", appsPath, map[string]any{"slug": "bad", "kind": "lambda"}, s), http.StatusUnprocessableEntity)
	expect(t, e.do("POST", "/api/v1/projects/auditfy/stages/nope/apps", map[string]any{"slug": "api", "kind": "service",
		"spec": map[string]any{"image": "x", "port": 1}}, s), http.StatusNotFound)
}

func TestCreateComposeApp(t *testing.T) {
	e := newTestEnv(t)
	s := e.seedStage()

	w := e.do("POST", appsPath, map[string]any{"slug": "nats", "kind": "compose",
		"compose_yaml": "services:\n  nats:\n    image: nats:2\n", "deploy_timeout_s": 60}, s)
	expect(t, w, http.StatusCreated)
	if app := decode[map[string]any](t, w); app["compose_yaml"] == "" || app["spec"] != nil || app["deploy_timeout_s"] != float64(60) {
		t.Fatalf("app=%v", app)
	}
	expect(t, e.do("POST", appsPath, map[string]any{"slug": "x", "kind": "compose", "compose_yaml": "  "}, s), http.StatusUnprocessableEntity)
	expect(t, e.do("POST", appsPath, map[string]any{"slug": "x", "kind": "compose", "compose_yaml": "a: 1",
		"spec": map[string]any{"image": "x"}}, s), http.StatusUnprocessableEntity)
	expect(t, e.do("POST", appsPath, map[string]any{"slug": "x", "kind": "compose", "compose_yaml": "a: 1",
		"deploy_timeout_s": 5}, s), http.StatusUnprocessableEntity)
}

func TestUpdateListDeleteApp(t *testing.T) {
	e := newTestEnv(t)
	s := e.seedStage()
	e.do("POST", appsPath, map[string]any{"slug": "api", "kind": "service", "spec": map[string]any{"image": "x:1", "port": 80}}, s)
	e.do("POST", appsPath, map[string]any{"slug": "nats", "kind": "compose", "compose_yaml": "a: 1"}, s)

	w := e.do("PATCH", appsPath+"/api", map[string]any{"spec": map[string]any{"image": "x:2", "port": 80}}, s)
	expect(t, w, http.StatusOK)
	if app := decode[map[string]any](t, w); app["spec"].(map[string]any)["image"] != "x:2" {
		t.Fatalf("app=%v", app)
	}
	w = e.do("PATCH", appsPath+"/api", map[string]any{"compose_yaml": "a: 1"}, s)
	expect(t, w, http.StatusUnprocessableEntity)
	if code := errorCode(t, w); code != "kind_mismatch" {
		t.Fatalf("code=%s", code)
	}
	expect(t, e.do("PATCH", appsPath+"/api", map[string]any{"spec": map[string]any{"image": "x", "port": -1}}, s), http.StatusUnprocessableEntity)

	w = e.do("GET", appsPath, nil, s)
	expect(t, w, http.StatusOK)
	if list := decode[[]map[string]any](t, w); len(list) != 2 || list[0]["slug"] != "api" {
		t.Fatalf("list=%v", list)
	}
	expect(t, e.do("DELETE", appsPath+"/nats", nil, s), http.StatusNoContent)
	expect(t, e.do("GET", appsPath+"/nats", nil, s), http.StatusNotFound)
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/api/
```

Expected: FAIL. La primera línea relevante es:

```
--- FAIL: TestCreateServiceApp (0.11s)
    apps_handlers_test.go:33: status 404, esperaba 201: 404 page not found
```

- [ ] **Step 3: Implementar**

`internal/api/apps_handlers.go`:

```go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/CPU-commits/muelle/internal/appspec"
	"github.com/CPU-commits/muelle/internal/store"
	"github.com/CPU-commits/muelle/internal/validate"
)

// En la Fase 1 el compose solo se guarda; la Fase 2 lo valida con compose-go.
const maxComposeBytes = 256 << 10

func (s *Server) appRoutes() {
	base := "/api/v1/projects/{p}/stages/{s}/apps"
	s.handle("GET "+base, accessToken, s.handleListApps)
	s.handle("POST "+base, accessSession, s.handleCreateApp)
	s.handle("GET "+base+"/{a}", accessToken, s.handleGetApp)
	s.handle("PATCH "+base+"/{a}", accessSession, s.handleUpdateApp)
	s.handle("DELETE "+base+"/{a}", accessSession, s.handleDeleteApp)
}

// appView agrega a la app el indicador de cambios pendientes.
type appView struct {
	store.App
	PendingChanges bool `json:"pending_changes"`
}

func viewApp(a store.App) appView { return appView{App: a, PendingChanges: a.PendingChanges()} }

type appInput struct {
	Slug           string          `json:"slug"`
	Kind           string          `json:"kind"`
	Spec           json.RawMessage `json:"spec"`
	ComposeYAML    *string         `json:"compose_yaml"`
	DeployTimeoutS *int            `json:"deploy_timeout_s"`
}

type appPatch struct {
	Spec           json.RawMessage `json:"spec"`
	ComposeYAML    *string         `json:"compose_yaml"`
	DeployTimeoutS *int            `json:"deploy_timeout_s"`
}

func hasJSON(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && string(t) != "null"
}

func validateCompose(y *string) error {
	if y == nil || strings.TrimSpace(*y) == "" {
		return validate.Errorf("compose_yaml", "es obligatorio")
	}
	if len(*y) > maxComposeBytes {
		return validate.Errorf("compose_yaml", "no puede superar 256 KiB")
	}
	return nil
}

func validateTimeout(v int) error {
	if v < 30 || v > 3600 {
		return validate.Errorf("deploy_timeout_s", "debe estar entre 30 y 3600")
	}
	return nil
}

func appTarget(p, st, a string) string { return "app:" + p + "/" + st + "/" + a }

func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
	apps, err := s.st.ListApps(r.Context(), r.PathValue("p"), r.PathValue("s"))
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]appView, 0, len(apps))
	for _, a := range apps {
		out = append(out, viewApp(a))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	var in appInput
	if !decodeJSON(w, r, &in, maxBodyBytes) {
		return
	}
	if err := validate.AppSlug(in.Slug); err != nil {
		s.fail(w, err)
		return
	}
	n := store.NewApp{ProjectSlug: r.PathValue("p"), StageSlug: r.PathValue("s"), Slug: in.Slug, Kind: in.Kind}
	switch in.Kind {
	case store.KindService:
		if in.ComposeYAML != nil {
			s.fail(w, validate.Errorf("compose_yaml", "no aplica a apps de tipo service"))
			return
		}
		spec, err := appspec.Parse(in.Spec)
		if err != nil {
			s.fail(w, err)
			return
		}
		n.Spec = spec.JSON()
	case store.KindCompose:
		if hasJSON(in.Spec) {
			s.fail(w, validate.Errorf("spec", "no aplica a apps de tipo compose"))
			return
		}
		if err := validateCompose(in.ComposeYAML); err != nil {
			s.fail(w, err)
			return
		}
		n.ComposeYAML = *in.ComposeYAML
	default:
		s.fail(w, validate.Errorf("kind", "debe ser service o compose"))
		return
	}
	if in.DeployTimeoutS != nil {
		if err := validateTimeout(*in.DeployTimeoutS); err != nil {
			s.fail(w, err)
			return
		}
		n.DeployTimeoutS = *in.DeployTimeoutS
	}
	app, err := s.st.CreateApp(r.Context(), n)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "app.create", appTarget(app.ProjectSlug, app.StageSlug, app.Slug), map[string]string{"kind": app.Kind})
	writeJSON(w, http.StatusCreated, viewApp(app))
}

func (s *Server) handleGetApp(w http.ResponseWriter, r *http.Request) {
	app, err := s.st.GetApp(r.Context(), r.PathValue("p"), r.PathValue("s"), r.PathValue("a"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, viewApp(app))
}

func (s *Server) handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	var in appPatch
	if !decodeJSON(w, r, &in, maxBodyBytes) {
		return
	}
	var u store.AppUpdate
	var fields []string
	if hasJSON(in.Spec) {
		spec, err := appspec.Parse(in.Spec)
		if err != nil {
			s.fail(w, err)
			return
		}
		raw := spec.JSON()
		u.Spec = &raw
		fields = append(fields, "spec")
	}
	if in.ComposeYAML != nil {
		if err := validateCompose(in.ComposeYAML); err != nil {
			s.fail(w, err)
			return
		}
		u.ComposeYAML = in.ComposeYAML
		fields = append(fields, "compose_yaml")
	}
	if in.DeployTimeoutS != nil {
		if err := validateTimeout(*in.DeployTimeoutS); err != nil {
			s.fail(w, err)
			return
		}
		u.DeployTimeoutS = in.DeployTimeoutS
		fields = append(fields, "deploy_timeout_s")
	}
	p, st, a := r.PathValue("p"), r.PathValue("s"), r.PathValue("a")
	app, err := s.st.UpdateApp(r.Context(), p, st, a, u)
	if err != nil {
		s.fail(w, err)
		return
	}
	if len(fields) > 0 {
		s.audit(r, "app.update", appTarget(p, st, a), map[string][]string{"fields": fields})
	}
	writeJSON(w, http.StatusOK, viewApp(app))
}

// handleDeleteApp solo borra el registro. La Fase 2 agrega `docker stack rm`.
func (s *Server) handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	p, st, a := r.PathValue("p"), r.PathValue("s"), r.PathValue("a")
	if err := s.st.DeleteApp(r.Context(), p, st, a); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "app.delete", appTarget(p, st, a), nil)
	w.WriteHeader(http.StatusNoContent)
}
```

En `internal/api/server.go`, al final de `routes()`, agregar `s.appRoutes()` después de `s.projectRoutes()`. Queda así:

```go
func (s *Server) routes() {
	// …
	s.handle("POST /api/v1/auth/totp/enable", accessSetup, s.handleTOTPEnable)

	s.projectRoutes()
	s.appRoutes()
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/api/ && go test ./internal/api/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/api
```

- [ ] **Step 5: Commit**

```bash
git add internal/api/apps_handlers.go internal/api/apps_handlers_test.go internal/api/server.go
git commit -m "feat(api): apps de formulario y compose" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 21: API: variables de stage y app

`GET`/`PUT`/`PATCH` de variables en `…/stages/{s}/env` y `…/apps/{a}/env`, con los mismos handlers para ambos dueños. `PUT` reemplaza todo el set, como el `sync-env` actual de GitHub Actions. `secret` es `true` si se omite. La auditoría guarda solo las claves.

**Files:**
- Create: `internal/api/env_handlers.go`
- Modify: `internal/api/server.go` (una línea en `routes()`)
- Test: `internal/api/env_handlers_test.go`

- [ ] **Step 1: Escribir el test**

`internal/api/env_handlers_test.go`:

```go
package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestEnvAPI(t *testing.T) {
	e := newTestEnv(t)
	s := e.seedStage()
	e.do("POST", appsPath, map[string]any{"slug": "api", "kind": "service", "spec": map[string]any{"image": "x", "port": 80}}, s)
	appEnv := appsPath + "/api/env"
	stageEnv := "/api/v1/projects/auditfy/stages/staging/env"

	w := e.do("PUT", appEnv, map[string]any{"vars": map[string]string{"DATABASE_URL": "postgres://x", "JWT_KEY": "k"}}, s)
	expect(t, w, http.StatusNoContent)
	w = e.do("PATCH", appEnv, map[string]any{"set": map[string]string{"LOG_LEVEL": "debug"}, "secret": false, "unset": []string{"JWT_KEY"}}, s)
	expect(t, w, http.StatusNoContent)

	w = e.do("GET", appEnv, nil, s)
	expect(t, w, http.StatusOK)
	body := w.Body.String()
	if strings.Contains(body, "postgres://x") {
		t.Fatalf("un secreto apareció en la respuesta: %s", body)
	}
	vars := decode[map[string][]map[string]any](t, w)["vars"]
	if len(vars) != 2 || vars[0]["key"] != "DATABASE_URL" || vars[0]["value"] != nil || vars[1]["value"] != "debug" {
		t.Fatalf("vars=%v", vars)
	}

	expect(t, e.do("PUT", appEnv, map[string]any{}, s), http.StatusUnprocessableEntity)
	expect(t, e.do("PUT", appEnv, map[string]any{"vars": map[string]string{"bad-key": "x"}}, s), http.StatusUnprocessableEntity)
	expect(t, e.do("PUT", stageEnv, map[string]any{"vars": map[string]string{"NATS_SERVERS": "nats://nats:4222"}, "secret": false}, s), http.StatusNoContent)
	expect(t, e.do("GET", appsPath+"/nope/env", nil, s), http.StatusNotFound)

	// La auditoría guarda claves, nunca valores.
	events, _ := e.st.ListAudit(context.Background(), 0, 10)
	for _, ev := range events {
		if strings.Contains(string(ev.Details), "postgres://x") {
			t.Fatalf("valor secreto en la auditoría: %s", ev.Details)
		}
	}
	if events[1].Action != "env.patch" || string(events[1].Details) != `{"set":["LOG_LEVEL"],"unset":["JWT_KEY"]}` {
		t.Fatalf("evento=%+v details=%s", events[1], events[1].Details)
	}
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/api/
```

Expected: FAIL. La primera línea relevante es:

```
--- FAIL: TestEnvAPI (0.11s)
    env_handlers_test.go:18: status 404, esperaba 204: 404 page not found
```

- [ ] **Step 3: Implementar**

`internal/api/env_handlers.go`:

```go
package api

import (
	"net/http"
	"sort"

	"github.com/CPU-commits/muelle/internal/store"
	"github.com/CPU-commits/muelle/internal/validate"
)

// 500 variables de hasta 64 KiB no caben en 1 MiB; el límite de env es mayor.
const maxEnvBodyBytes = 8 << 20

// ownerFunc resuelve el dueño de las variables desde la ruta y su "target" de auditoría.
type ownerFunc func(*http.Request) (store.EnvOwner, string, error)

func (s *Server) envRoutes() {
	stage := "/api/v1/projects/{p}/stages/{s}/env"
	app := "/api/v1/projects/{p}/stages/{s}/apps/{a}/env"
	for path, owner := range map[string]ownerFunc{stage: s.stageOwner, app: s.appOwner} {
		s.handle("GET "+path, accessToken, s.handleGetEnv(owner))
		s.handle("PUT "+path, accessToken, s.handlePutEnv(owner))
		s.handle("PATCH "+path, accessToken, s.handlePatchEnv(owner))
	}
}

func (s *Server) stageOwner(r *http.Request) (store.EnvOwner, string, error) {
	st, err := s.st.GetStage(r.Context(), r.PathValue("p"), r.PathValue("s"))
	if err != nil {
		return store.EnvOwner{}, "", err
	}
	return store.StageOwner(st.ID), "stage:" + st.ProjectSlug + "/" + st.Slug, nil
}

func (s *Server) appOwner(r *http.Request) (store.EnvOwner, string, error) {
	a, err := s.st.GetApp(r.Context(), r.PathValue("p"), r.PathValue("s"), r.PathValue("a"))
	if err != nil {
		return store.EnvOwner{}, "", err
	}
	return store.AppOwner(a.ID), appTarget(a.ProjectSlug, a.StageSlug, a.Slug), nil
}

func (s *Server) handleGetEnv(owner ownerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		o, _, err := owner(r)
		if err != nil {
			s.fail(w, err)
			return
		}
		vars, err := s.env.List(r.Context(), o)
		if err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"vars": vars})
	}
}

// handlePutEnv reemplaza todo el set, como el sync-env de GitHub Actions.
// Body: {"vars": {"K": "v"}, "secret": true}; secret es true si se omite.
func (s *Server) handlePutEnv(owner ownerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Vars   map[string]string `json:"vars"`
			Secret *bool             `json:"secret"`
		}
		if !decodeJSON(w, r, &in, maxEnvBodyBytes) {
			return
		}
		if in.Vars == nil {
			s.fail(w, validate.Errorf("vars", "es obligatorio"))
			return
		}
		o, target, err := owner(r)
		if err != nil {
			s.fail(w, err)
			return
		}
		if err := s.env.Replace(r.Context(), o, in.Vars, in.Secret == nil || *in.Secret); err != nil {
			s.fail(w, err)
			return
		}
		s.audit(r, "env.replace", target, map[string][]string{"keys": sortedKeys(in.Vars)})
		w.WriteHeader(http.StatusNoContent)
	}
}

// handlePatchEnv: {"set": {"K": "v"}, "unset": ["K2"], "secret": true}.
func (s *Server) handlePatchEnv(owner ownerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Set    map[string]string `json:"set"`
			Unset  []string          `json:"unset"`
			Secret *bool             `json:"secret"`
		}
		if !decodeJSON(w, r, &in, maxEnvBodyBytes) {
			return
		}
		o, target, err := owner(r)
		if err != nil {
			s.fail(w, err)
			return
		}
		if err := s.env.Patch(r.Context(), o, in.Set, in.Secret == nil || *in.Secret, in.Unset); err != nil {
			s.fail(w, err)
			return
		}
		unset := append([]string{}, in.Unset...)
		sort.Strings(unset)
		s.audit(r, "env.patch", target, map[string][]string{"set": sortedKeys(in.Set), "unset": unset})
		w.WriteHeader(http.StatusNoContent)
	}
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
```

En `internal/api/server.go`, al final de `routes()`, agregar `s.envRoutes()` después de `s.appRoutes()`. Queda así:

```go
func (s *Server) routes() {
	// …
	s.handle("POST /api/v1/auth/totp/enable", accessSetup, s.handleTOTPEnable)

	s.projectRoutes()
	s.appRoutes()
	s.envRoutes()
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/api/ && go test ./internal/api/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/api
```

- [ ] **Step 5: Commit**

```bash
git add internal/api/env_handlers.go internal/api/env_handlers_test.go internal/api/server.go
git commit -m "feat(api): variables de stage y app" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 22: API: usuarios, invitaciones, tokens, settings y auditoría

Endpoints de administración, todos solo con sesión. `TestTokenScopes` es la matriz de permisos de §10.1: si alguna fila cambia, cambió la seguridad. `PATCH /settings` no deja activar `require_2fa` a quien no tiene su propio 2FA configurado; sin esa guarda, quedaría encerrado en la pantalla de configuración.

**Files:**
- Create: `internal/api/admin_handlers.go`
- Modify: `internal/api/server.go` (una línea en `routes()`)
- Test: `internal/api/admin_handlers_test.go`

- [ ] **Step 1: Escribir el test**

`internal/api/admin_handlers_test.go`:

```go
package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestInviteFlow(t *testing.T) {
	e := newTestEnv(t)
	s := withSession(e.login())

	w := e.do("POST", "/api/v1/invites", nil, s)
	expect(t, w, http.StatusCreated)
	url := decode[map[string]any](t, w)["url"].(string)
	token, ok := strings.CutPrefix(url, testOrigin+"/invite/")
	if !ok {
		t.Fatalf("url=%s", url)
	}

	w = e.do("POST", "/api/v1/auth/invite/"+token, map[string]string{"username": "ana", "password": testPassword})
	expect(t, w, http.StatusCreated)
	ana := sessionFrom(t, w)
	expect(t, e.do("GET", "/api/v1/auth/me", nil, withSession(ana)), http.StatusOK)

	w = e.do("POST", "/api/v1/auth/invite/"+token, map[string]string{"username": "beto", "password": testPassword})
	expect(t, w, http.StatusBadRequest)
	if code := errorCode(t, w); code != "invalid_invite" {
		t.Fatalf("code=%s", code)
	}

	w = e.do("GET", "/api/v1/users", nil, s)
	expect(t, w, http.StatusOK)
	if users := decode[[]map[string]any](t, w); len(users) != 2 || users[0]["username"] != "admin" {
		t.Fatalf("users=%v", users)
	}
}

func TestDeleteUserAndResetTOTP(t *testing.T) {
	e := newTestEnv(t)
	s := withSession(e.login())
	e.user("ana")

	w := e.do("DELETE", "/api/v1/users/admin", nil, s)
	expect(t, w, http.StatusConflict)
	if code := errorCode(t, w); code != "cannot_delete_self" {
		t.Fatalf("code=%s", code)
	}
	expect(t, e.do("DELETE", "/api/v1/users/ana/totp", nil, s), http.StatusNoContent)
	expect(t, e.do("DELETE", "/api/v1/users/ana", nil, s), http.StatusNoContent)
	expect(t, e.do("DELETE", "/api/v1/users/ana", nil, s), http.StatusNotFound)
}

func TestSettingsAndAuditAPI(t *testing.T) {
	e := newTestEnv(t)
	s := withSession(e.login())

	w := e.do("GET", "/api/v1/settings", nil, s)
	expect(t, w, http.StatusOK)
	if decode[map[string]bool](t, w)["require_2fa"] {
		t.Fatal("require_2fa debe empezar en false")
	}
	w = e.do("PATCH", "/api/v1/settings", map[string]bool{"require_2fa": true}, s)
	expect(t, w, http.StatusConflict)
	if code := errorCode(t, w); code != "totp_required_first" {
		t.Fatalf("code=%s", code)
	}

	// Con su propio 2FA configurado, sí puede exigirlo.
	admin, _ := e.st.GetUserByUsername(context.Background(), "admin")
	secret, _, _ := e.auth.BeginTOTPSetup(context.Background(), admin.ID)
	code, _ := totp.GenerateCode(secret, time.Now())
	if err := e.auth.EnableTOTP(context.Background(), admin.ID, code); err != nil {
		t.Fatal(err)
	}
	w = e.do("PATCH", "/api/v1/settings", map[string]bool{"require_2fa": true}, s)
	expect(t, w, http.StatusOK)
	if !decode[map[string]bool](t, w)["require_2fa"] {
		t.Fatal("require_2fa no se guardó")
	}

	w = e.do("GET", "/api/v1/audit?limit=1", nil, s)
	expect(t, w, http.StatusOK)
	events := decode[[]map[string]any](t, w)
	if len(events) != 1 || events[0]["action"] != "settings.update" {
		t.Fatalf("events=%v", events)
	}
	expect(t, e.do("GET", "/api/v1/audit?limit=0", nil, s), http.StatusUnprocessableEntity)
}

// TestTokenScopes es la matriz de permisos de §10.1: qué puede hacer un token según su alcance.
func TestTokenScopes(t *testing.T) {
	e := newTestEnv(t)
	s := e.seedStage()
	e.do("POST", "/api/v1/projects/auditfy/stages", map[string]string{"slug": "production"}, s)
	e.do("POST", "/api/v1/projects", map[string]string{"slug": "otro", "name": "Otro"}, s)
	for _, stage := range []string{"staging", "production"} {
		e.do("POST", "/api/v1/projects/auditfy/stages/"+stage+"/apps",
			map[string]any{"slug": "api", "kind": "service", "spec": map[string]any{"image": "x", "port": 80}}, s)
	}

	newToken := func(stage string) string {
		w := e.do("POST", "/api/v1/tokens", map[string]any{"name": "ci", "project": "auditfy", "stage": stage}, s)
		expect(t, w, http.StatusCreated)
		tok := decode[map[string]any](t, w)["token"].(string)
		if !strings.HasPrefix(tok, "mk_") {
			t.Fatalf("token=%s", tok)
		}
		return tok
	}
	stagingTok := newToken("staging")
	projectTok := newToken("")

	env := map[string]any{"vars": map[string]string{"A": "1"}}
	cases := []struct {
		name   string
		token  string
		method string
		path   string
		body   any
		status int
	}{
		{"stage: lee su app", stagingTok, "GET", "/api/v1/projects/auditfy/stages/staging/apps/api", nil, 200},
		{"stage: reemplaza env de su app", stagingTok, "PUT", "/api/v1/projects/auditfy/stages/staging/apps/api/env", env, 204},
		{"stage: reemplaza env de su stage", stagingTok, "PUT", "/api/v1/projects/auditfy/stages/staging/env", env, 204},
		{"stage: lee el proyecto", stagingTok, "GET", "/api/v1/projects/auditfy", nil, 200},
		{"stage: no toca otro stage", stagingTok, "PUT", "/api/v1/projects/auditfy/stages/production/apps/api/env", env, 403},
		{"stage: no lee otro stage", stagingTok, "GET", "/api/v1/projects/auditfy/stages/production/apps/api", nil, 403},
		{"stage: no toca otro proyecto", stagingTok, "GET", "/api/v1/projects/otro", nil, 403},
		{"stage: no crea apps", stagingTok, "POST", "/api/v1/projects/auditfy/stages/staging/apps", map[string]any{"slug": "x"}, 403},
		{"stage: no lista proyectos", stagingTok, "GET", "/api/v1/projects", nil, 403},
		{"stage: no crea tokens", stagingTok, "POST", "/api/v1/tokens", map[string]any{"name": "x", "project": "auditfy"}, 403},
		{"stage: no usa auth/me", stagingTok, "GET", "/api/v1/auth/me", nil, 403},
		{"proyecto: toca cualquier stage", projectTok, "PUT", "/api/v1/projects/auditfy/stages/production/apps/api/env", env, 204},
		{"proyecto: no toca otro proyecto", projectTok, "GET", "/api/v1/projects/otro", nil, 403},
		{"proyecto: no borra stages", projectTok, "DELETE", "/api/v1/projects/auditfy/stages/production", nil, 403},
		{"token inventado", "mk_inventado", "GET", "/api/v1/projects/auditfy", nil, 401},
	}
	for _, c := range cases {
		// Los tokens no necesitan Origin: no hay cookies que robar.
		w := e.do(c.method, c.path, c.body, withBearer(c.token), withOrigin(""))
		if w.Code != c.status {
			t.Errorf("%s: status %d, esperaba %d (%s)", c.name, w.Code, c.status, w.Body.String())
		}
	}

	// Revocar un token lo invalida.
	w := e.do("GET", "/api/v1/tokens", nil, s)
	tokens := decode[[]map[string]any](t, w)
	id := strconv.Itoa(int(tokens[0]["id"].(float64)))
	expect(t, e.do("DELETE", "/api/v1/tokens/"+id, nil, s), http.StatusNoContent)
	expect(t, e.do("GET", "/api/v1/projects/auditfy", nil, withBearer(stagingTok)), http.StatusUnauthorized)
}
```

- [ ] **Step 2: Verificar que falla**

```bash
go test ./internal/api/
```

Expected: FAIL. La primera línea relevante es:

```
--- FAIL: TestInviteFlow (0.11s)
    admin_handlers_test.go:19: status 404, esperaba 201: 404 page not found
```

- [ ] **Step 3: Implementar**

`internal/api/admin_handlers.go`:

```go
package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/CPU-commits/muelle/internal/store"
	"github.com/CPU-commits/muelle/internal/validate"
)

func (s *Server) adminRoutes() {
	s.handle("GET /api/v1/users", accessSession, s.handleListUsers)
	s.handle("DELETE /api/v1/users/{username}", accessSession, s.handleDeleteUser)
	s.handle("DELETE /api/v1/users/{username}/totp", accessSession, s.handleResetUserTOTP)
	s.handle("POST /api/v1/invites", accessSession, s.handleCreateInvite)
	s.handle("GET /api/v1/tokens", accessSession, s.handleListTokens)
	s.handle("POST /api/v1/tokens", accessSession, s.handleCreateToken)
	s.handle("DELETE /api/v1/tokens/{id}", accessSession, s.handleDeleteToken)
	s.handle("GET /api/v1/settings", accessSession, s.handleGetSettings)
	s.handle("PATCH /api/v1/settings", accessSession, s.handlePatchSettings)
	s.handle("GET /api/v1/audit", accessSession, s.handleListAudit)
}

type userView struct {
	Username    string    `json:"username"`
	TOTPEnabled bool      `json:"totp_enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.st.ListUsers(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]userView, 0, len(users))
	for _, u := range users {
		out = append(out, userView{Username: u.Username, TOTPEnabled: u.HasTOTP(), CreatedAt: u.CreatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if username == principal(r).Username {
		writeError(w, http.StatusConflict, "cannot_delete_self", "no puedes borrar tu propio usuario")
		return
	}
	u, err := s.st.GetUserByUsername(r.Context(), username)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.st.DeleteUser(r.Context(), u.ID); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "user.delete", "user:"+username, nil)
	w.WriteHeader(http.StatusNoContent)
}

// handleResetUserTOTP sirve cuando alguien pierde su teléfono.
func (s *Server) handleResetUserTOTP(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	u, err := s.st.GetUserByUsername(r.Context(), username)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.auth.ResetTOTP(r.Context(), u.ID); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "user.totp_reset", "user:"+username, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	token, expires, err := s.auth.CreateInvite(r.Context(), &p.UserID)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "invite.create", "user:"+p.Username, nil)
	writeJSON(w, http.StatusCreated, map[string]any{"url": s.origin + "/invite/" + token, "expires_at": expires})
}

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := s.st.ListTokens(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tokens)
}

type tokenCreated struct {
	store.APIToken
	Token string `json:"token"` // en claro: se muestra una sola vez
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name          string `json:"name"`
		Project       string `json:"project"`
		Stage         string `json:"stage"`
		ExpiresInDays int    `json:"expires_in_days"`
	}
	if !decodeJSON(w, r, &in, maxBodyBytes) {
		return
	}
	plain, t, err := s.auth.CreateAPIToken(r.Context(), in.Name, in.Project, in.Stage, principal(r).UserID, in.ExpiresInDays)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "token.create", "token:"+strconv.FormatInt(t.ID, 10), map[string]string{"project": t.ProjectSlug, "stage": t.StageSlug})
	writeJSON(w, http.StatusCreated, tokenCreated{APIToken: t, Token: plain})
}

func (s *Server) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.fail(w, validate.Errorf("id", "debe ser un número"))
		return
	}
	if err := s.st.DeleteToken(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "token.delete", "token:"+r.PathValue("id"), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	required, err := s.auth.Require2FA(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"require_2fa": required})
}

// handlePatchSettings: en la Fase 1 solo existe require_2fa. Las fases
// siguientes agregan Cloudflare, IP pública, email ACME, etc.
func (s *Server) handlePatchSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Require2FA *bool `json:"require_2fa"`
	}
	if !decodeJSON(w, r, &in, maxBodyBytes) {
		return
	}
	if in.Require2FA != nil {
		if *in.Require2FA {
			// Sin esta guarda, quien activa la opción sin tener 2FA queda encerrado en la configuración.
			u, err := s.st.GetUserByID(r.Context(), principal(r).UserID)
			if err != nil {
				s.fail(w, err)
				return
			}
			if !u.HasTOTP() {
				writeError(w, http.StatusConflict, "totp_required_first", "configura tu propio 2FA antes de exigirlo a todos")
				return
			}
		}
		if err := s.auth.SetRequire2FA(r.Context(), *in.Require2FA); err != nil {
			s.fail(w, err)
			return
		}
		s.audit(r, "settings.update", "settings", map[string]bool{"require_2fa": *in.Require2FA})
	}
	s.handleGetSettings(w, r)
}

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	limit, before := 100, int64(0)
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			s.fail(w, validate.Errorf("limit", "debe estar entre 1 y 500"))
			return
		}
		limit = n
	}
	if v := r.URL.Query().Get("before"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			s.fail(w, validate.Errorf("before", "debe ser un id válido"))
			return
		}
		before = n
	}
	events, err := s.st.ListAudit(r.Context(), before, limit)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}
```

En `internal/api/server.go`, al final de `routes()`, agregar `s.adminRoutes()` después de `s.envRoutes()`. Queda así:

```go
func (s *Server) routes() {
	// …
	s.handle("POST /api/v1/auth/totp/enable", accessSetup, s.handleTOTPEnable)

	s.projectRoutes()
	s.appRoutes()
	s.envRoutes()
	s.adminRoutes()
}
```

- [ ] **Step 4: Verificar que pasa**

```bash
go vet ./internal/api/ && go test ./internal/api/
```

Expected:
```
ok  	github.com/CPU-commits/muelle/internal/api
```

- [ ] **Step 5: Commit**

```bash
git add internal/api/admin_handlers.go internal/api/admin_handlers_test.go internal/api/server.go
git commit -m "feat(api): usuarios, invitaciones, tokens, settings y auditor\u00eda" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 23: Comando `muelle serve` y prueba de punta a punta

Une todo:
- carga la configuración y la clave
- migra la base
- imprime el link de setup si no hay usuarios
- levanta el HTTP con apagado ordenado ante SIGINT/SIGTERM
- corre el mantenimiento cada hora: limpia sesiones, borra auditoría de más de 180 días y hace el backup diario (7 copias)

`muelle genkey` sirve para desarrollo local; en producción la clave la genera `muelle install` (Fase 5).

**Files:**
- Create: `cmd/muelle/main.go`

- [ ] **Step 1: Escribir `cmd/muelle/main.go`**

```go
// Comando muelle: `muelle serve` levanta el panel y la API; `muelle genkey`
// imprime una clave maestra nueva (para desarrollo local; en producción la
// genera `muelle install`, Fase 5).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/CPU-commits/muelle/internal/api"
	"github.com/CPU-commits/muelle/internal/auth"
	"github.com/CPU-commits/muelle/internal/config"
	"github.com/CPU-commits/muelle/internal/crypto"
	"github.com/CPU-commits/muelle/internal/envvars"
	"github.com/CPU-commits/muelle/internal/store"
)

const usage = `uso: muelle <comando>

comandos:
  serve    levanta el panel y la API (configuración por variables MUELLE_*)
  genkey   imprime una clave maestra nueva en base64`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve()
	case "genkey":
		fmt.Println(crypto.GenerateKey())
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		slog.Error("muelle terminó con error", "err", err)
		os.Exit(1)
	}
}

func serve() error {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	key, err := crypto.LoadKeyFile(cfg.MasterKeyFile)
	if err != nil {
		return err
	}
	cipher, err := crypto.New(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "muelle.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := st.Migrate(ctx); err != nil {
		return err
	}

	authSvc := auth.NewService(st, cipher)
	if token, created, err := authSvc.EnsureSetupInvite(ctx); err != nil {
		return err
	} else if created {
		log.Info("todavía no hay usuarios: abre este link para crear el primero (vence en 24 h)",
			"url", cfg.PanelOrigin+"/setup?token="+token)
	}

	srv := &http.Server{
		Addr: cfg.Listen,
		Handler: api.New(api.Deps{Store: st, Auth: authSvc, Env: envvars.NewService(st, cipher),
			PanelOrigin: cfg.PanelOrigin, SecureCookies: cfg.SecureCookies, Logger: log}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go maintenance(ctx, st, cfg.DataDir, log)

	errc := make(chan error, 1)
	go func() {
		log.Info("muelle escuchando", "addr", cfg.Listen)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		log.Info("apagando")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// maintenance corre cada hora: limpia sesiones vencidas, borra auditoría de
// más de 180 días y hace el backup diario de la base (§5.2 y §12 del spec).
func maintenance(ctx context.Context, st *store.Store, dataDir string, log *slog.Logger) {
	lastBackup := ""
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		now := st.Now()
		if _, err := st.DeleteExpiredSessions(ctx, now, auth.SessionIdle); err != nil {
			log.Error("limpiando sesiones", "err", err)
		}
		if _, err := st.DeleteAuditBefore(ctx, now.AddDate(0, 0, -180)); err != nil {
			log.Error("limpiando auditoría", "err", err)
		}
		if day := now.UTC().Format("2006-01-02"); day != lastBackup {
			if path, err := st.Backup(ctx, filepath.Join(dataDir, "backups"), 7); err != nil {
				log.Error("backup diario", "err", err)
			} else {
				lastBackup = day
				log.Info("backup diario", "path", path)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
```

- [ ] **Step 2: Verificar que compila estático**

```bash
go vet ./... && CGO_ENABLED=0 go build -o /tmp/muelle ./cmd/muelle && file /tmp/muelle
```

Expected: `ELF 64-bit LSB executable, x86-64, … statically linked`.

- [ ] **Step 3: Prueba de punta a punta con el binario real**

Desde la raíz del repo:

```bash
D=$(mktemp -d)
go build -o "$D/muelle" ./cmd/muelle
"$D/muelle" genkey > "$D/key"
export MUELLE_PANEL_ORIGIN=http://localhost:18080 MUELLE_LISTEN=127.0.0.1:18080 \
  MUELLE_DATA_DIR="$D/data" MUELLE_MASTER_KEY_FILE="$D/key" MUELLE_SECURE_COOKIES=false
"$D/muelle" serve > "$D/log.txt" 2>&1 &
PID=$!; sleep 1
TOKEN=$(grep -o 'setup?token=[^"]*' "$D/log.txt" | cut -d= -f2)
O="Origin: http://localhost:18080"; J="Content-Type: application/json"; B=http://localhost:18080/api/v1

curl -s -c "$D/cj" -H "$O" -H "$J" -d "{\"token\":\"$TOKEN\",\"username\":\"felipe\",\"password\":\"una-contraseña-larga\"}" $B/auth/setup; echo
curl -s -b "$D/cj" -H "$O" -H "$J" -d '{"slug":"auditfy","name":"Auditfy"}' $B/projects; echo
curl -s -b "$D/cj" -H "$O" -H "$J" -d '{"slug":"staging"}' $B/projects/auditfy/stages; echo
curl -s -b "$D/cj" -H "$O" -H "$J" -d '{"slug":"api","kind":"service","spec":{"image":"ghcr.io/aumi-app/backend-app:staging","port":3000,"healthcheck":{"type":"bun-http","path":"/health"}}}' \
  $B/projects/auditfy/stages/staging/apps > /dev/null
CI=$(curl -s -b "$D/cj" -H "$O" -H "$J" -d '{"name":"github-actions","project":"auditfy","stage":"staging"}' $B/tokens \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])')
curl -s -o /dev/null -w 'sync-env: %{http_code}\n' -X PUT -H "Authorization: Bearer $CI" -H "$J" \
  -d '{"vars":{"DATABASE_URL":"postgres://secreto","APP_ENV":"staging"}}' $B/projects/auditfy/stages/staging/apps/api/env
curl -s -H "Authorization: Bearer $CI" $B/projects/auditfy/stages/staging/apps/api/env; echo
curl -s -o /dev/null -w 'fuera de alcance: %{http_code}\n' -H "Authorization: Bearer $CI" $B/projects
ls "$D/data/backups"
kill -TERM $PID; sleep 1; tail -1 "$D/log.txt"
rm -rf "$D"
```

Expected (los `…` varían):

```
{"username":"felipe"}
{"slug":"auditfy","name":"Auditfy","created_at":"…"}
{"project":"auditfy","slug":"staging","created_at":"…"}
sync-env: 204
{"vars":[{"key":"APP_ENV","value":null,"secret":true,"updated_at":"…"},{"key":"DATABASE_URL","value":null,"secret":true,"updated_at":"…"}]}
fuera de alcance: 403
muelle-AAAAMMDD-HHMMSS.db
{"time":"…","level":"INFO","msg":"apagando"}
```

Si `sync-env` no da 204, si algún `value` de la respuesta no es `null` o si `fuera de alcance` no da 403, hay que detenerse y revisar las Tasks 17-21.

- [ ] **Step 4: Correr toda la suite con el detector de carreras**

```bash
gofmt -l . ; go vet ./... && go test -race ./...
```

Expected: `gofmt -l` no imprime nada y todos los paquetes salen `ok`.

- [ ] **Step 5: Commit**

```bash
git add cmd/muelle/main.go
git commit -m "feat: comando muelle serve con mantenimiento y backup diario" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


### Task 24: CI y README

GitHub Actions con gofmt, vet, tests con `-race` y build estático, más un README con cómo correrlo localmente.

**Files:**
- Create: `.github/workflows/ci.yaml`, `README.md`

- [ ] **Step 1: Crear `.github/workflows/ci.yaml`**

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: gofmt
        run: test -z "$(gofmt -l .)"
      - name: vet
        run: go vet ./...
      - name: test
        run: go test -race ./...
      - name: build
        run: CGO_ENABLED=0 go build -o muelle ./cmd/muelle
```

- [ ] **Step 2: Crear `README.md`**

````markdown
# muelle

Deploy manager ultraligero sobre Docker Swarm: panel web + API, secretos, stages y proxy con certificados automáticos.

- Diseño: [`docs/superpowers/specs/2026-10-09-muelle-design.md`](docs/superpowers/specs/2026-10-09-muelle-design.md)
- Planes: [`docs/superpowers/plans/`](docs/superpowers/plans/)

## Estado

Fase 1 (Base): API de usuarios, sesiones, 2FA, tokens de API, proyectos, stages, apps y variables cifradas. Todavía no despliega nada.

## Desarrollo local

```bash
go run ./cmd/muelle genkey > /tmp/muelle-key
MUELLE_PANEL_ORIGIN=http://localhost:8080 \
MUELLE_DATA_DIR=/tmp/muelle-data \
MUELLE_MASTER_KEY_FILE=/tmp/muelle-key \
MUELLE_SECURE_COOKIES=false \
go run ./cmd/muelle serve
```

En el primer arranque, el log imprime el link de setup (`…/setup?token=…`) para crear el primer usuario con `POST /api/v1/auth/setup`.

| Variable | Por defecto | Qué es |
|---|---|---|
| `MUELLE_PANEL_ORIGIN` | (obligatoria) | Origen exacto del panel, ej. `https://panel.tudominio.cl`. Se compara con el header `Origin` (CSRF) |
| `MUELLE_LISTEN` | `:8080` | Dirección HTTP |
| `MUELLE_DATA_DIR` | `/data` | Carpeta de `muelle.db` y `backups/` |
| `MUELLE_MASTER_KEY_FILE` | `/run/secrets/muelle_master_key` | Clave maestra en base64 (32 bytes) |
| `MUELLE_SECURE_COOKIES` | `true` | Usar `false` solo en desarrollo sin HTTPS |

## Tests

```bash
go test -race ./...
```
````

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yaml README.md
git commit -m "ci: gofmt, vet, tests y build; README de desarrollo" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```


## Después de la Fase 1

Cada fase se planifica en su propio documento en `docs/superpowers/plans/` cuando termina la anterior, para aprovechar lo aprendido. El código de la Fase 1 ya deja los puntos de enganche:

- **Fase 2 — Deploy** (§6, §7):
  - Paquetes nuevos:
    - `internal/docker`: SDK oficial de Docker.
    - `internal/render`: con `compose-go` y tests de golden files.
    - `internal/deployer`: cola por app, digests, `docker stack deploy`, convergencia y reconciliación.
  - Migración `0002` con `deploys` y `registries`.
  - Endpoints de deploys, rollback y registries.
  - Cambios sobre el código de la Fase 1:
    - `validateCompose` (en `api/apps_handlers.go`) se reemplaza por la validación real con `compose-go`, que incluye los nombres de servicio únicos por stage.
    - `handleDeleteApp` agrega `docker stack rm`.
    - El deploy usa `envvars.Service.Resolve` y llena `apps.last_deployed_at` y `apps.storage_node_id`.
- **Fase 3 — Proxy y DNS** (§3, §8, §9):
  - Paquetes nuevos:
    - `internal/proxy`: el contenedor muelle-caddy y su configuración.
    - `internal/dns`: Cloudflare.
  - Migración `0003` con `domains`.
  - Settings cifrados (la columna `settings.encrypted` ya existe), con el token de Cloudflare.
  - `handleCreateStage`/`DeleteStage` crean y borran la red overlay y conectan Caddy.
  - Endpoints de estado y logs en vivo (SSE).
  - Verificación de certificados.
- **Fase 4 — Panel** (§11): SPA en Svelte, embebida con `go:embed` y servida por el mismo `api.Server`.
- **Fase 5 — Distribución** (§3.3, §14, §16):
  - `muelle install`.
  - Imágenes de muelle y de Caddy.
  - GitHub Action composite.
  - El origen del panel pasa a salir de settings.
  - Migración de auditfy desde Dokploy.
