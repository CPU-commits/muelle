# Módulo `ingress` (Fase 3): el contrato que usa el panel

La fuente es el contrato HTTP de las Fases 2 y 3:
`docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md` en `fase-23-base` (§0 y §3),
con la Fase 3 terminada.
Este archivo resume lo que la pantalla Dominios lee y manda, y cómo se comporta con cada
respuesta. Si el contrato cambia, se cambian este README, `data/dtos/domainDto.ts` y los
mocks (`src/mocks/{db,handlers}/ingress.ts`).

## Convenciones

- Base `/api/v1`, JSON en snake_case, fechas RFC 3339 en UTC.
- Errores: `{"error": {"code": "…", "message": "…"}}`; el panel decide por el `code`.
- **Solo con sesión**: un token de API recibe `403 token_not_allowed`.
- `{app}` es `/projects/{p}/stages/{s}/apps/{a}`. `{hostname}` va url-encoded y tiene que
  ser de esa app: si no, `404 not_found`, aunque exista en otra.
- Crear, editar y borrar aplican al instante el DNS y la ruta de Caddy, sin deploy. Un fallo
  de Caddy no hace fallar la petición (se ve en `GET /proxy`).

## Errores

| Respuesta                                | Qué ve el panel                                                                                                                                                               |
| ---------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `404 no_route`, `405 method_not_allowed` | La Fase 3 todavía no está: `<ComingSoon phase={3}>`                                                                                                                           |
| `404 not_found` (un dominio)             | `DomainNotFound`. Al borrar: otra persona ya lo borró, se cierra el diálogo y se pide la lista                                                                                |
| `409 conflict` (crear)                   | `HostnameTaken`: "Ese hostname ya está en uso en muelle", en el campo                                                                                                         |
| `409 nothing_to_adopt`                   | `NothingToAdopt`: "No hay un registro que adoptar"                                                                                                                            |
| `422 invalid`                            | En el campo: `hostname`, o `target_service` / `target_port` (el campo "destino"), o `proxied`                                                                                 |
| `502 cloudflare_unavailable`             | `CloudflareUnavailable` (en el diálogo de borrar): Cloudflare no respondió o rechazó el borrado, no se borró nada; para borrarlo solo en muelle, se quita el token en Ajustes |
| `503 docker_unavailable`                 | `DockerUnavailable`: "Docker no responde en el servidor de muelle."                                                                                                           |

## El dominio

```json
{
	"hostname": "api.tudominio.cl",
	"target_service": "api",
	"target_port": 3000,
	"proxied": true,
	"tls_mode": "cloudflare",
	"cf_zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
	"cf_record_id": "372e67954025e0ba6aaa6d586b9e0b59",
	"status": "pending",
	"last_error_code": null,
	"last_error": null,
	"cert_issuer": null,
	"cert_expires_at": null,
	"records": [
		{ "type": "A", "name": "api.tudominio.cl", "content": "203.0.113.24" },
		{ "type": "AAAA", "name": "api.tudominio.cl", "content": "2001:db8::24" }
	],
	"created_at": "2026-10-01T12:00:00Z",
	"updated_at": "2026-10-01T12:00:00Z"
}
```

| Campo                              | Qué                                                                                                                                                                                                                                                                          |
| ---------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `hostname`                         | Único en todo muelle, en forma canónica (minúsculas, sin punto final, sin comodines)                                                                                                                                                                                         |
| `target_service`, `target_port`    | El servicio (nombre corto) y el puerto al que va el tráfico                                                                                                                                                                                                                  |
| `proxied`                          | Detrás del proxy de Cloudflare. Solo tiene efecto si muelle maneja el registro: fuera de toda zona, o sin token, queda `false` aunque el `POST` mande `true`                                                                                                                 |
| `tls_mode`                         | `cloudflare` (DNS-01) · `http` (HTTP-01, fuera de Cloudflare)                                                                                                                                                                                                                |
| `cf_zone_id?`                      | La zona; null si no cae en ninguna o no se pudo consultar                                                                                                                                                                                                                    |
| `cf_record_id?`                    | El registro `A` que maneja muelle; **null si no lo maneja**. El del `AAAA` no se expone                                                                                                                                                                                      |
| `status`                           | `pending` · `active` · `error`                                                                                                                                                                                                                                               |
| `last_error_code?`                 | Solo con `error`, y **siempre** con `error` (la migración 0003 lo garantiza): `record_exists` → Adoptar; `cloudflare` o `certificate` → Reintentar. El panel decide **solo por el código**, nunca por el texto. Uno desconocido es un error cualquiera                       |
| `last_error?`                      | El mensaje en español, que el panel muestra                                                                                                                                                                                                                                  |
| `cert_issuer?`, `cert_expires_at?` | Lo último que vio la verificación en un certificado **válido** (la organización del emisor, o su CN): "Let's Encrypt · vence 8 ene 2027"; sin los dos, "Certificado pendiente"                                                                                               |
| `records`                          | **Calculados con los ajustes actuales** (no se guardan): un `A` con la IP pública y un `AAAA` con la IPv6, si las hay; `[]` sin IP pública. `name` es el hostname. Con `cf_record_id` son los que maneja muelle; sin zona, los que hay que crear a mano (tabla con "copiar") |
| `created_at`, `updated_at`         | `updated_at` cambia con cada cambio del dominio (crearlo, un `PATCH` que cambia algo, adoptarlo, un reintento que rehace el DNS y cada cambio de `status`); una verificación que solo renueva el certificado no lo mueve                                                     |

### Estados

- **Crear**, con token y el hostname en una zona: se crean `A` (y `AAAA`) con el `proxied`
  pedido y queda `pending`. Si ya hay un registro de ese nombre que muelle no creó (un `A`,
  un `AAAA` o un `CNAME`): `error` + `record_exists`, sin `cf_record_id` pero **con**
  `cf_zone_id` y `tls_mode: cloudflare` (el certificado sale por DNS-01 igual). Si
  Cloudflare falla, o no hay IP pública: `error` + `cloudflare` (el `POST` igual responde
  `201`).
- **Sin token o sin zona**: `tls_mode: http`, `pending`, `proxied: false`; el registro se
  crea a mano.
- **`pending` → `active`**: cuando la verificación ve un certificado válido que vence en más
  de 14 días. **Corre 30 s después de crear, adoptar o reintentar; cada minuto mientras
  sigue `pending`; y cada hora para todos.**
- **Un `pending` sin certificado válido 10 min después** de su último cambio (`updated_at`)
  pasa a `error` + `certificate`. **`active` → `error`** + `certificate` si el certificado es
  inválido o vence en menos de 14 días; **`error` + `certificate` → `active`** cuando la
  verificación de cada hora ve uno válido.
- Si muelle no llega a Caddy, o el proxy no está `synced`, la verificación no cambia nada.

| Estado del panel      | Cuándo                                           | Acción                                                          |
| --------------------- | ------------------------------------------------ | --------------------------------------------------------------- |
| Válido                | `active`                                         | Proxied (si `cf_record_id`), Editar, Reintentar en el menú      |
| Emitiendo certificado | `pending` y muelle lo resuelve (no es manual)    | La lista se refresca cada 5 s mientras haya uno                 |
| Esperando DNS         | `pending` fuera de Cloudflare (`tls_mode: http`) | **Verificar** (= retry: verifica ahora) y la tabla de `records` |
| Registro existente    | `error` + `record_exists`, sin `cf_record_id`    | **Adoptar registro**; "Volver a mirar el DNS" en el menú        |
| Error                 | otro `error`                                     | `last_error` y **Reintentar**                                   |

Un dominio "Esperando DNS" se refresca cada 30 s: muelle lo verifica cada minuto mientras
siga `pending`, así que pasa solo a `active` (o a `error` + `certificate` a los 10 min).
**Verificar** lo mira ahora.

## Endpoints

| Endpoint                              | Cuerpo                                                            | Respuesta                     |
| ------------------------------------- | ----------------------------------------------------------------- | ----------------------------- |
| `GET {app}/domains`                   | —                                                                 | `200 [dominio]`, por hostname |
| `POST {app}/domains`                  | `{hostname, target_service, target_port, proxied}`                | `201 dominio`                 |
| `PATCH {app}/domains/{hostname}`      | `{proxied}` o `{target_service, target_port}`: solo lo que cambia | `200 dominio`                 |
| `DELETE {app}/domains/{hostname}`     | —                                                                 | `204`                         |
| `POST {app}/domains/{hostname}/adopt` | — (vacío o `{}`; otra cosa es `400 invalid_json`)                 | `200 dominio`                 |
| `POST {app}/domains/{hostname}/retry` | — (vacío o `{}`; otra cosa es `400 invalid_json`)                 | `200 dominio`                 |

- **Crear**: el panel normaliza el hostname (minúsculas, sin punto final) y lo valida
  antes. En una app de formulario, `target_service` tiene que ser el slug de la app (en una
  compose lo verifica el render del deploy); si no, `422 target_service`. `proxied` va siempre, y **en `false` por defecto** (como el contrato): el
  formulario tiene el switch "Proxied" al lado de "Agregar". Un hostname que ya usa
  cualquier app: `409 conflict`. Tampoco puede ser el dominio del panel (`422 hostname`).
- **Editar**: `proxied` sobre un dominio sin `cf_record_id` es `422` (el panel no ofrece el
  switch en ese caso). Si Cloudflare falla (o ya no hay token), el valor se guarda, el
  dominio queda en `error` + `cloudflare` y responde `200`; si sale bien y estaba en
  `error`/`cloudflare`, vuelve a `pending`. El switch es optimista: se mueve enseguida y
  vuelve si el backend falla.
- **Borrar**: con `cf_record_id`, borra primero los registros en Cloudflare (`A` y `AAAA`).
  Si Cloudflare no responde **o rechaza el borrado** (un token sin permiso):
  `502 cloudflare_unavailable` y **no se borra nada**; el diálogo de confirmación queda
  abierto con el error, que dice cómo borrarlo solo en muelle (quitar el token en Ajustes).
  Sin token, borra solo en muelle.
- **Adoptar**: solo con `error` + `record_exists` (si no, `409 nothing_to_adopt`). Apunta el
  registro existente a la IP pública, **conserva su `proxied` y lo copia al dominio**
  (adoptar no cambia hacia dónde va el tráfico; para cambiarlo, un `PATCH` después) y deja
  el dominio `pending`. Un `CNAME` pasa a ser un `A`. Sin token o sin IP pública, con
  varios `A`/`CNAME` de ese nombre, o si Cloudflare falla: `200` con el dominio en `error` +
  `cloudflare` y el motivo (el aviso de Adoptar lo muestra). Sin IP pública, la tarjeta ya
  dice que falta configurarla en Ajustes.
- **Reintentar** (sincrónico, hasta 20 s): con `cloudflare` rehace el DNS; con
  `record_exists` **vuelve a mirar y, si el registro ajeno ya no está, lo crea**; con
  `pending`, `certificate` o `active` verifica el certificado ahora (un `certificate` que
  sigue sin certificado vuelve a `pending` con otros 10 min, y muelle fuerza a Caddy a
  reintentar la emisión); sin zona, la vuelve a buscar. Responde con el estado resultante.

## `GET /proxy` → 200 (solo con sesión)

El estado de muelle-caddy, para el aviso "proxy desincronizado" (spec §8 y §13). Lo muestra
Dominios, arriba del formulario.

```json
{
	"status": "synced",
	"error": null,
	"synced_at": "2026-10-10T14:20:41Z",
	"caddy": {
		"image": "ghcr.io/cpu-commits/muelle-caddy:v0.1.0",
		"running": true,
		"http_port": 80,
		"https_port": 443
	}
}
```

| `status` (y `caddy.running`) | Aviso de Dominios                                                                         |
| ---------------------------- | ----------------------------------------------------------------------------------------- |
| `synced`, corriendo          | Ninguno                                                                                   |
| `synced`, `running: false`   | "muelle-caddy no está corriendo"                                                          |
| `out_of_sync`                | "El proxy está desincronizado", con el `error` y un link a Ajustes; muelle reintenta solo |
| `recreating`                 | "Se está recreando el proxy": cambiaron los puertos en Ajustes (nunca la imagen, §3.5)    |
| `disabled`                   | "El proxy de muelle está apagado": falta el dominio del panel o la imagen de Caddy        |

- `error` viene solo con `out_of_sync`: el último fallo del contenedor (un puerto ocupado, la
  imagen que no baja, el registry sin acceso a la imagen de Caddy) o, si no hay, el de
  Caddy al cargar la configuración (con el token como `***`). Lo que lo arregla está en
  Ajustes (puertos, registries, email, token): el aviso trae el link. Al arrancar muelle,
  hasta el primer envío a Caddy, dice `out_of_sync` con "muelle todavía no le mandó la
  configuración a Caddy".
- `synced_at` es null hasta el primer envío; sin contenedor, `caddy.image` es null y
  `running` false. Con `disabled` no se consulta Docker (también responde sin Docker).
- Mientras dice `out_of_sync` o `recreating` se pregunta de nuevo cada 10 s (con la
  pestaña visible). Cada cambio de un dominio (crear, editar, borrar, adoptar,
  reintentar) lo vuelve a pedir: una ruta nueva puede dejarlo desincronizado.
- Sin la Fase 3 (`no_route`), con Docker caído (`503 docker_unavailable`) o mientras carga,
  no se muestra nada: ni `<ComingSoon>` ni un error (los dominios tienen sus propios avisos).
- Ajustes no es de este módulo, pero usa `proxyQuery` y `proxyNotice` del barrel: después
  de cambiar los puertos sin mudarse de origen (solo el HTTP, o se entró por otra dirección)
  sigue `GET /proxy` mientras dice `recreating`, y muestra si muelle-caddy quedó escuchando o
  el `error` si no arrancó (contrato §3.5).

## Refresco y avisos

- La lista se refresca cada 5 s mientras muelle está resolviendo algo ("Emitiendo
  certificado") y cada 30 s mientras hay uno "Esperando DNS"; nunca con la pestaña oculta.
- Cambiar la IP pública o el token en Ajustes cambia los `records` y puede sacar dominios
  de `error` + `cloudflare`: Ajustes invalida estas queries (`ingressKeys.all`).
- Los dominios con error se anuncian a un lector de pantalla en **una** región para toda la
  pantalla ("2 dominios tienen errores: …"), que cambia solo si cambia cuáles fallan.

## Superficie pública (barrel)

- `DomainsScreen` (props `project`, `stage`, `app` y, opcional, `targets`: los destinos
  posibles como `"api:3000"`). Sin `targets`, los pide al catálogo (`appTargetsQuery` del
  barrel de `catalog`): un `servicio:puerto` por cada puerto que declara cada servicio de
  la app (`services` de `GET {app}`, contrato §2.12). Siempre se suman los que ya usan los
  dominios.
- `domainsQuery`, `proxyQuery`, `ingressKeys`, `ingressUseCases`, `domainState`,
  `proxyNotice` y los tipos `AppDomain`, `DomainState`, `Hostname`, `ProxyStatus`,
  `ProxyNotice`.

## Mocks (`pnpm dev:mock`)

La primera vez que se abre una app se siembran los tres del diseño:
`{app}-{stage}.tudominio.cl` (válido), `webhooks-{stage}.tudominio.cl` (registro existente,
con Proxied prendido en Cloudflare: Adoptar lo copia) y `{app}.{stage}.otro-dominio.com`
(fuera de Cloudflare, esperando DNS). Las IP públicas son `203.0.113.24` y `2001:db8::24`.

- Un hostname bajo `tudominio.cl` cae en la zona: `pending` y a los ~3 s `active`.
    - Con `existe` en la primera etiqueta: el registro ya existía (Adoptar). Con
      `existe-tmp`, el registro ajeno desaparece a los 5 s y "Volver a mirar el DNS" lo
      crea.
    - Con `cf-error`: Cloudflare responde con error al crearlo (Reintentar).
    - Con `cf-intermitente`: Cloudflare falla al cambiar Proxied o al adoptar (`200` con el
      dominio en `error` + `cloudflare`).
    - Con `cf-caido`: Cloudflare no responde al borrarlo (`502`, no se borra).
    - Con `caddy-rechaza`: Caddy no acepta la configuración y `GET /proxy` dice
      `out_of_sync`.
- En una app de formulario (según el catálogo del mock, aunque nunca se haya desplegado),
  el destino tiene que ser el servicio de la app: otro es `422 target_service`. En una
  compose no se verifica al guardar.
- Cualquier otro hostname queda fuera de Cloudflare (y su `proxied` en `false`) hasta que se
  toca Verificar.
- El token de Cloudflare, las IP públicas y el dominio del panel son los de Ajustes del mock
  (`db/admin.ts`): sin token todo queda fuera de Cloudflare; `records` sigue a las IP de
  ahora (sin IPv6 no hay `AAAA`, sin IP pública `[]`); adoptar sin token o sin IP queda en
  `error` + `cloudflare`; sin dominio del panel, `GET /proxy` dice `disabled`.
- Cambiar los puertos en Ajustes recrea muelle-caddy: `GET /proxy` dice `recreating` unos 4 s
  (sin contenedor: `image: null`, `running: false`) y después `synced`. El puerto `8081`
  está ocupado en el host del mock: el `PATCH` lo prueba antes de guardar y responde `422`
  en ese campo, sin guardar nada (§3.5).
- Un `PATCH` de Proxied que sale bien saca un `error` + `cloudflare` (vuelve a `pending`).
  Sin token (se quitó en Ajustes), se guarda y el dominio queda en `error` + `cloudflare`
  con "agrega el token de Cloudflare en Ajustes", como en el backend.

## Diferencias con el contrato

Ninguna en estos endpoints: lo de arriba es lo que dice el contrato. Dos cosas de ingress
que el panel sigue del código porque el contrato no las dice (propuestas como cambios):

- `409 network_conflict` al crear un stage (ya hay una red de Docker con su nombre que no
  creó muelle; `ErrNotStageNetwork` en `ingress/infra/in/http/errors.go`): catalog lo
  muestra como `StageNetworkConflict`. Falta en la tabla de §0.2.
- Un email ACME o un token de Cloudflare con `{` o `}`: §3.4 dice que muelle lo guarda y
  deja de mandarle la configuración a Caddy (`out_of_sync`). Ajustes los frena antes de
  mandarlos (`InvalidAcmeEmail('placeholder')`, `InvalidCloudflareToken('placeholder')`);
  lo propuesto es que `PATCH /settings` responda `422` con su campo.
