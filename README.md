# muelle

Deploy manager ultraligero sobre Docker Swarm: panel web + API, secretos, stages y proxy con certificados automáticos. Go (`net/http` de la biblioteca estándar, SQLite en Go puro) con arquitectura hexagonal/DDD.

- Diseño: [`docs/superpowers/specs/2026-10-09-muelle-design.md`](docs/superpowers/specs/2026-10-09-muelle-design.md)
- Plan de la Fase 1: [`docs/superpowers/plans/2026-10-10-muelle-fase1-hexagonal.md`](docs/superpowers/plans/2026-10-10-muelle-fase1-hexagonal.md)

## Estado

Fase 1 (Base): API REST de usuarios, sesiones, 2FA, tokens de API, proyectos, stages, apps, variables cifradas, settings, auditoría y backups. Todavía no despliega nada.

## Estructura (spec §4.1)

```
main.go             subcomandos serve y genkey
src/
├── cmd/http/       ServeMux, middleware y errores → HTTP
├── cmd/cron/       ejecuta las tareas periódicas de los módulos
├── internal/
│   ├── identity/   usuarios, sesiones, 2FA, invitaciones, tokens de API
│   ├── catalog/    proyectos, stages y apps
│   ├── secrets/    variables cifradas de stage y app
│   └── admin/      settings, auditoría, backup y limpieza periódica
├── shared/         dominio común y container.New, que arma todo
├── package/        interfaces finas: cipher, clock, audit
└── settings/       carga y validación de MUELLE_*
```

Cada módulo tiene `domain/`, `app/` e `infra/` (in/out). Los módulos no se importan entre sí.

## Desarrollo local

```bash
go run . genkey > /tmp/muelle.key
MUELLE_PANEL_ORIGIN=http://127.0.0.1:18080 \
MUELLE_SECURE_COOKIES=false \
MUELLE_LISTEN=127.0.0.1:18080 \
MUELLE_DATA_DIR=/tmp/muelle-data \
MUELLE_MASTER_KEY_FILE=/tmp/muelle.key \
go run . serve
```

El link de setup se imprime al arrancar.

| Variable | Por defecto | Qué es |
|---|---|---|
| `MUELLE_PANEL_ORIGIN` | (obligatoria) | Origen exacto del panel; se compara con el header `Origin` (CSRF) |
| `MUELLE_LISTEN` | `:8080` | Dirección HTTP |
| `MUELLE_DATA_DIR` | `/data` | Carpeta de `muelle.db` y `backups/` |
| `MUELLE_MASTER_KEY_FILE` | `/run/secrets/muelle_master_key` | Clave maestra en base64 (32 bytes) |
| `MUELLE_SECURE_COOKIES` | `true` | `false` solo en desarrollo sin HTTPS |

## Tests

```bash
go vet ./...
go test -race ./...
```

## Convenciones

Comentarios y mensajes de error en español; identificadores en inglés.

## Notas para las fases siguientes

Hallazgos de la revisión de código que las próximas fases deben respetar.

**Fase 2, render**
- Escapar `$` como `$$` en todo string que venga del spec de formulario (`command`, `healthcheck.command`, imagen, mounts, valores de env).
- Generar el healthcheck `http` en forma exec (sin shell).
- Fijar el nombre del proyecto compose explícitamente y usar variables de entorno solo para interpolación (las claves `COMPOSE_*` están reservadas).
- Verificar que los nombres de `secrets:`/`configs:` de compose (`<stack>_<name>`) no pasen de 64 caracteres.
- Rechazar volúmenes externos, o con `name:` explícito que apunte a `muelle_*`.
- Comprobar que el tamaño del env mezclado stage+app cabe en el `ARG_MAX` de Linux.
- Conectar `secrets.ResolveEnv` (recibe un `AppID`).
- Reemplazar `validateCompose` por la validación real de compose-go, incluyendo nombres de servicio únicos por stage.
- El borrado de una app agrega `docker stack rm`.

**Fase 2, worker de deploy**
- Las goroutines nunca deben usar el ctx de una transacción después de que `InTx` retorna; derivar de un ctx externo (`context.WithoutCancel`).

**Fase 3**
- Crear un stage agrega la red overlay y conecta Caddy.
- El pool de direcciones de la overlay debe ser privado (RFC1918/ULA) para que se pueda confiar en `X-Real-IP`.

**Fase 5 / install**
- Considerar el prefijo `__Host-` en el nombre de la cookie de sesión cuando las cookies son Secure.
