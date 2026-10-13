# Módulo `ingress` (Fase 3): contrato que el panel supone

El backend de la Fase 3 todavía no existe. La pantalla Dominios está hecha contra el spec
(`docs/superpowers/specs/2026-10-09-muelle-design.md`, §5.1 tabla `domains`, §5.2 regla 4,
§8, §9 y §10), la migración `0003_ingress.sql` de `fase-23-base` y las suposiciones de este
archivo. **Lo que dice aquí es lo que el panel lee y manda hoy**: si la Fase 3 decide otra cosa, hay que cambiar este README,
`data/dtos/domainDto.ts` y los mocks (`src/mocks/{db,handlers}/ingress.ts`).

Lo que el spec no fija y se supuso está marcado con **(supuesto)**.

## Convenciones

- Base `/api/v1`, JSON, snake_case, fechas RFC 3339 con zona.
- Errores con el sobre de siempre: `{"error": {"code": "…", "message": "…"}}`.
- Solo con sesión (un token de API no maneja dominios, spec §10.1).
- `{app}` abajo es `/projects/{p}/stages/{s}/apps/{a}`. **(supuesto)** Las rutas de un
  dominio (`…/domains/{hostname}`) van bajo la app, igual que la lista: el spec las
  escribe `PATCH/DELETE …/domains/{hostname}`. `{hostname}` va con `encodeURIComponent`.
- Crear, editar o borrar un dominio aplica al instante el DNS y la ruta de Caddy, sin
  deploy (spec §5.2 regla 4).
- Los campos marcados `?` pueden faltar o venir `null`. Un campo desconocido se ignora.

## Detección de "todavía no existe"

Contra un backend sin la Fase 3: `404 {"error":{"code":"no_route"}}` o
`405 method_not_allowed` → `UnavailableError` con `phase: 3` → `<ComingSoon phase={3}>`.
Un `404 not_found` en las rutas de **un** dominio es `DomainNotFound` (se mapea por
código, gana siempre).

## El dominio

```json
{
	"hostname": "webhooks-staging.tudominio.cl",
	"target_service": "api",
	"target_port": 3000,
	"proxied": true,
	"tls_mode": "cloudflare",
	"cf_zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
	"cf_record_id": null,
	"status": "error",
	"last_error": "el registro ya existe",
	"last_error_code": "record_exists",
	"cert_issuer": null,
	"cert_expires_at": null,
	"records": [
		{ "type": "A", "name": "webhooks-staging.tudominio.cl", "content": "203.0.113.24" },
		{ "type": "AAAA", "name": "webhooks-staging.tudominio.cl", "content": "2001:db8::24" }
	],
	"created_at": "2026-10-01T12:00:00Z"
}
```

| Campo                              | Tipo                                           | Qué                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| ---------------------------------- | ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `hostname`                         | string                                         | Único en todo muelle (spec §5.1), en minúsculas y sin punto final.                                                                                                                                                                                                                                                                                                                                                                                      |
| `target_service`                   | string                                         | El servicio de la app al que va el tráfico (slug, spec §5.2).                                                                                                                                                                                                                                                                                                                                                                                           |
| `target_port`                      | int 1–65535                                    |                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `proxied`                          | bool                                           | Detrás del proxy de Cloudflare (`proxied INTEGER NOT NULL DEFAULT 0`). Solo tiene efecto si muelle maneja el registro.                                                                                                                                                                                                                                                                                                                                  |
| `tls_mode`                         | `cloudflare` · `http`                          | Spec §5.1 y §8: desafío DNS de Cloudflare, o HTTP-01 fuera de Cloudflare.                                                                                                                                                                                                                                                                                                                                                                               |
| `cf_zone_id?`                      | string                                         | La zona de Cloudflare del hostname; null si no cae en ninguna.                                                                                                                                                                                                                                                                                                                                                                                          |
| `cf_record_id?`                    | string                                         | El registro que creó o adoptó muelle; **null si muelle no lo maneja** (spec §9).                                                                                                                                                                                                                                                                                                                                                                        |
| `status`                           | `pending` · `active` · `error`                 | Spec §5.1.                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `last_error?`                      | string                                         | El mensaje del error (spec §9: "el registro ya existe", la respuesta de Cloudflare; §8: certificado inválido o por vencer).                                                                                                                                                                                                                                                                                                                             |
| `last_error_code?`                 | `record_exists` · `cloudflare` · `certificate` | El motivo de `error`, para elegir la acción: `record_exists` → **Adoptar**; `cloudflare` y `certificate` → **Reintentar**. La tabla lo garantiza (`CHECK`: `status = 'error'` ⇔ `last_error_code` no nulo), así que el panel decide **solo por el código**, nunca por el texto de `last_error`. Un valor desconocido se trata como un error cualquiera (Reintentar).                                                                                    |
| `cert_issuer?`, `cert_expires_at?` | string, RFC 3339                               | Lo que ve la verificación de §8 (emisor y vencimiento del certificado servido). El panel muestra "Let's Encrypt · vence 8 ene 2027" si vienen los dos, y "Certificado pendiente" si no.                                                                                                                                                                                                                                                                 |
| `records`                          | array                                          | **(supuesto, campo nuevo)** Los registros DNS que este dominio necesita, ya con la IP pública: `{type: "A" \| "AAAA", name, content}`, el `AAAA` cuando el nodo tiene IPv6 (`cf_record_id_aaaa` en la tabla). Con `cf_record_id` son los que maneja muelle; sin zona (`tls_mode: http`) son los que hay que crear a mano, y el panel los muestra en una tabla con "copiar". La IP del texto de Adoptar sale del `A`. `name` es el FQDN. Si falta, `[]`. |
| `created_at?`                      | RFC 3339                                       |                                                                                                                                                                                                                                                                                                                                                                                                                                                         |

Cómo lo muestra el panel:

| Estado del panel      | Cuándo                                                       | Acción                                          |
| --------------------- | ------------------------------------------------------------ | ----------------------------------------------- |
| Válido                | `status: active`                                             | Proxied (si `cf_record_id`), Editar             |
| Esperando DNS         | `pending` y fuera de Cloudflare (`tls_mode: http`, sin zona) | **Verificar** (= retry) y la tabla de `records` |
| Emitiendo certificado | `pending` en Cloudflare                                      | la lista se refresca cada 5 s mientras haya uno |
| Registro existente    | `error` con `record_exists` y sin `cf_record_id`             | **Adoptar registro**                            |
| Error                 | cualquier otro `error`                                       | `last_error` y **Reintentar**                   |

## Endpoints

### `GET {app}/domains` → 200

**(supuesto)** Array plano de dominios (como `GET /projects`). El panel los ordena por
hostname.

### `POST {app}/domains` → 201

```json
{ "hostname": "api.tudominio.cl", "target_service": "api", "target_port": 3000, "proxied": true }
```

El panel normaliza el hostname (minúsculas, sin punto final) y lo valida antes: al menos
dos etiquetas, cada una `[a-z0-9-]` sin guion al principio ni al final, TLD que empieza
con letra, ≤ 253, sin `https://` ni ruta, sin comodines. `proxied` va siempre, y en
**`false` por defecto** (como la columna): el formulario tiene el switch "Proxied" al lado
de "Agregar". **(supuesto)** Fuera de Cloudflare no tiene efecto y queda `false`.
Respuesta: el dominio creado (spec §9: con zona → registro y `pending`; sin zona →
`tls_mode: http`; registro ajeno → `error` con `record_exists`).

Errores: `409 conflict` (**(supuesto)** o `hostname_taken`; el `conflict` genérico se
acepta mientras tanto, con un TODO para sacarlo) → "Ese hostname ya está en uso en
muelle"; `422 invalid` con `"hostname: …"`, `"target_service: …"` o `"target_port: …"` →
al campo del formulario; `400 invalid_json`.

### `PATCH {app}/domains/{hostname}` → 200

Solo lo que cambia, uno o los dos grupos:

```json
{ "proxied": false }
{ "target_service": "web", "target_port": 8080 }
```

Respuesta: el dominio actualizado. Cambiar `proxied` actualiza los registros que maneja
muelle (spec §9). **(supuesto)** `proxied` en un dominio sin `cf_record_id` →
`422 invalid` con `"proxied: …"` (el panel no ofrece el switch en ese caso). El switch es
optimista: se mueve enseguida y vuelve si el backend falla.

### `DELETE {app}/domains/{hostname}` → 204

Borra la ruta y, solo si `cf_record_id` no es nulo, el registro en Cloudflare (spec §9).
Un error se muestra dentro del diálogo de confirmación, que queda abierto; un
`404 not_found` (otra persona ya lo borró) cierra el diálogo y pide la lista de nuevo.

### `POST {app}/domains/{hostname}/adopt` → 200

Sin cuerpo. Spec §9: actualiza el registro existente a la IP pública y guarda el
`cf_record_id`. Respuesta: el dominio (típicamente `pending` hasta el certificado).
**(supuesto)** Si no hay nada que adoptar: `409 nothing_to_adopt`.

### `POST {app}/domains/{hostname}/retry` → 200

Sin cuerpo. Reintenta lo que falló: el DNS en Cloudflare (spec §9) o, fuera de
Cloudflare, vuelve a verificar que el registro resuelva y el certificado (§8, "Verificar"
en el panel). Respuesta: el dominio, con el estado nuevo.

## Refresco y avisos

- La lista se refresca sola cada 5 s **solo** mientras muelle está resolviendo algo
  ("Emitiendo certificado": `pending` en Cloudflare), y nunca con la pestaña oculta. Uno
  que espera un registro manual no cambia solo: se resuelve con "Verificar".
- Los dominios con error se anuncian a un lector de pantalla en **una** región para toda
  la pantalla ("2 dominios tienen errores: …"), que cambia solo si cambia cuáles fallan.

## Superficie pública (barrel)

- `DomainsScreen` (props `project`, `stage`, `app` y, del catálogo, `targets`: los
  destinos posibles como `"api:3000"`; los conecta F2).
- `domainsQuery`, `ingressKeys`, `ingressUseCases`, `domainState` y los tipos `AppDomain`,
  `DomainState`, `Hostname`.

## Mocks (`pnpm dev:mock`)

La primera vez que se abre una app se siembran los tres del diseño: `{app}-{stage}.tudominio.cl`
(válido), `webhooks-{stage}.tudominio.cl` (registro existente → Adoptar) y
`{app}.{stage}.otro-dominio.com` (fuera de Cloudflare, esperando DNS). Las IP públicas
son `203.0.113.24` y `2001:db8::24` (cada dominio lleva su `A` y su `AAAA`).

- Un hostname bajo `tudominio.cl` cae en la zona: `pending` y a los ~3 s `active`.
    - Con `existe` en la primera etiqueta: el registro ya existía (Adoptar).
    - Con `cf-error`: Cloudflare responde con error (Reintentar).
- Cualquier otro hostname queda fuera de Cloudflare hasta que se toca Verificar (y su
  `proxied` queda en `false`).
