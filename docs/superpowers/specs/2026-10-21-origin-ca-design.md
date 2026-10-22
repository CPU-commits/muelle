# muelle — Certificados de Cloudflare (Origin CA)

> Fecha: 2026-10-21 · Estado: aprobado, para implementar con TDD · Base: `main` @ a1238d9 (se escribió contra 58f7236 y se adaptó después de los menores de la integración, §15 R15).
> Fuentes de verdad: el diseño (`docs/superpowers/specs/2026-10-09-muelle-design.md`, §5.1, §8, §9, §12, §13 y §15) y el contrato (`docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md`, §1.1, §3, §3.7, §4 y §6). El texto del contrato y del diseño que sale de acá (§10.2 y §10.4) ya está en esos dos documentos (el Paso 0a del plan, §14), y **donde difieran, mandan ellos**. Este documento guarda la investigación, las decisiones, los riesgos y el plan.

La decisión del dueño, que es final:
- Let's Encrypt sigue siendo lo normal.
- En Ajustes hay una opción nueva, apagada por defecto: "Certificados de Cloudflare (Origin CA)".
- Con la opción encendida, y con un token de Cloudflare que tiene Zone → SSL and Certificates → Edit, cada dominio de Cloudflare con el proxy (nube naranja) usa un certificado de Cloudflare Origin CA en vez de Let's Encrypt. Esto incluye los dominios de las apps y el del panel.
- Los dominios en DNS only y los que están fuera de Cloudflare siguen con Let's Encrypt.
- Si un dominio pierde el proxy, o se apaga la opción, ese dominio vuelve a Let's Encrypt.
- El objetivo es que el modo SSL "Full (strict)" de Cloudflare use certificados de Cloudflare.
- Solo Cloudflare confía en un certificado de Origin CA; los navegadores no. El panel y la documentación lo dicen.

---

## 0. Resumen de las decisiones

| Tema | Decisión |
|---|---|
| Ajuste | `cloudflare_origin_ca` (bool, por defecto `false`), en la tabla `settings` y en `shared.ProxySettings`. Va en `GET /settings` siempre y se cambia con `PATCH /settings` |
| Validación | Encenderla sin token, o quitar el token sin apagarla, da `422`. Si el token no tiene el permiso en alguna de las zonas activas que ve, `422` con un mensaje que nombra el permiso y las zonas. Si Cloudflare no responde, `502 cloudflare_unavailable`. Para comprobar el permiso se usa primero la pista `permissions` de `GET /zones` (no tiene efectos secundarios) y, si no alcanza, un certificado de prueba de 7 días que se revoca enseguida (§3.3) |
| Qué hostnames califican | Un dominio de una app califica si tiene `tls_mode: cloudflare`, muelle maneja su registro (`cf_record_id`), está `proxied` en muelle **y** en Cloudflare todos sus A/AAAA/CNAME tienen el proxy (muelle lo lee cada 5 min). El panel califica si cae en una zona (`panel_cf_zone_id`) y todos sus A/AAAA/CNAME tienen el proxy, aunque muelle no haya creado el registro (solo lo lee). Un registro de una app que muelle no maneja no califica |
| Emisión | ECDSA P-256 y CSR con `crypto/x509`, `request_type: origin-ecc`, **90 días**, un hostname por certificado. Se renueva cuando faltan 30 días. Se deja de servir cuando faltan 14 días: ahí se vuelve a Let's Encrypt |
| Dónde se guarda | Migración `0006_origin_ca.sql` con dos tablas: `origin_ca_certs` (los certificados, con la clave cifrada con la clave maestra y la AAD `origin_ca_certs:private_key:<id>`) y `origin_ca_hosts` (lo que se vio en Cloudflare, y las fallas con su backoff) |
| Revocación | Un certificado se revoca cuando la opción se apaga, el dominio pierde el proxy, se borra el dominio, cambia el dominio del panel, cambia la zona o se renueva. **Solo después** de que Caddy cargó una configuración sin él, y con 1 min de margen: revocar uno que todavía se sirve rompe Full (strict). Cambiar el token no revoca nada por sí solo |
| Caddy | El certificado va en `apps.tls.certificates.load_pem`. El host va en `automatic_https.skip_certificates` y sale de las políticas de automatización. Va `apps.tls.disable_ocsp_stapling: true`. ACME (DNS-01 y HTTP-01) sigue igual para los demás hosts |
| Fallas | Si la emisión falla, se sirve Let's Encrypt mientras tanto. El dominio **no** pasa a `error`, porque funciona. El aviso va en un campo nuevo, `origin_ca.error_code` (`permission`, `cloudflare` o `rejected`), con su texto. El backoff es de 1 min a 1 h si Cloudflare no responde, y de 6 h si lo rechaza |
| API | En el dominio se suman `cert_source` (`letsencrypt` o `cloudflare_origin_ca`) y `origin_ca` (null con la opción apagada, sin token o con el proxy deshabilitado). `GET /proxy` suma `panel_certificate`. `cert_issuer` no cambia de sentido: sigue siendo el emisor que vio la verificación. Las claves nuevas están siempre, y el panel las trata como obligatorias (§11) |
| Verificación | `TLSProbe` acepta también un certificado "fijado": para un host con Origin CA, la hoja que sirve Caddy tiene que ser idéntica, byte a byte, a la que guardó muelle. No se embeben las raíces de Origin CA |

---

## 1. Lo que dice la investigación

### 1.1 Cloudflare Origin CA

Verificado en la documentación oficial, en el OpenAPI (`cloudflare/api-schemas`) y en el código de cloudflare-go y de `origin-ca-issuer`. Lo marcado **NO VERIFICADO** no está documentado.

**Crear: `POST https://api.cloudflare.com/client/v4/certificates`** ([create](https://developers.cloudflare.com/api/resources/origin_ca_certificates/methods/create/))
- Campos obligatorios: `hostnames`, `request_type` y `csr`. Con un CSR vacío Cloudflare responde `1007 CSR parsed as empty`. El CSR va como PEM con saltos de línea.
- `hostnames` son FQDN de zonas de la cuenta. Admiten `*.` en un solo nivel y no admiten IPs. El OpenAPI dice `maxItems: 100` y la página de Origin CA dice 200 SANs. muelle manda siempre uno.
- `request_type` es `origin-rsa`, `origin-ecc` o `keyless-certificate`.
- `requested_validity` es opcional. Admite `7, 30, 90, 365, 730, 1095, 5475` días, y por defecto es `5475` (15 años).
- Respuesta (`result`):
  - `id`: el serial en decimal, un string;
  - `certificate`: el PEM;
  - `hostnames`, `request_type`, `requested_validity` y `csr`, tal como se mandaron;
  - `expires_on`: un string sin formato declarado (`"2014-01-01 05:20:00 +0000 UTC"`). **muelle no lo usa:** toma el vencimiento del `NotAfter` del certificado.
- La cadena es una sola hoja firmada por la raíz de Origin CA, sin intermedios (blog de Cloudflare; no se confirmó con un certificado real).
- Las raíces RSA y ECC son autofirmadas y **vencen el 15 de agosto de 2029**. Un certificado de 15 años emitido hoy vencería después que su raíz.

**Autenticación**
- Un token de API con Zone → SSL and Certificates → Edit ([Origin CA](https://developers.cloudflare.com/ssl/origin-configuration/origin-ca/)).
- La Origin CA Key (`X-Auth-User-Service-Key`) se dio de baja el 30 de septiembre de 2026 ([ca-keys](https://developers.cloudflare.com/fundamentals/api/get-started/ca-keys/)). Hoy solo sirve un token, como supuso el dueño.
- El usuario dueño del token tiene que tener "API Access" activado. Si no, Cloudflare responde 1010 "This zone is either not part of your account…" ([troubleshooting](https://developers.cloudflare.com/ssl/origin-configuration/origin-ca/troubleshooting/)).
- Las operaciones de `/certificates` no traen la sección "Accepted Permissions" en la documentación, ni la anotación `x-api-token-group` en el OpenAPI. Lo único documentado es "SSL and Certificates · Edit". **NO VERIFICADO:** si `SSL and Certificates · Read` alcanza para listar.

**Listar y revocar**
- `GET /certificates?zone_id=…`: `zone_id` es obligatorio, y pagina con `page` y `per_page` (de 5 a 50). muelle no lo necesita: guarda el `id` de cada certificado.
- `DELETE /certificates/{id}` responde `{"result": {"id": "…", "revoked_at": "2024-09-06T18:43:47.928893Z"}}` ([delete](https://developers.cloudflare.com/api/resources/origin_ca_certificates/methods/delete/)). No se puede deshacer.
- **Revocar uno que el origen todavía sirve rompe el tráfico:** "our edge servers will refuse to communicate with origins serving the revoked certificate" ([blog](https://blog.cloudflare.com/cloudflare-ca-encryption-origin/)). Por eso el orden de §6.

**Comprobar el permiso sin efectos secundarios: no hay una forma documentada.**
- `GET /user/tokens/verify` solo trae `{id, status, expires_on, not_before}`.
- `GET /user/tokens/{id}` trae las políticas, pero pide "API Tokens Read", que el token de muelle no tiene.
- Ningún `GET` distingue Edit de Read: todos los de SSL aceptan Read.
- El objeto zona (`GET /zones`) trae `permissions` (`"#ssl:edit"`, `"#dns_records:edit"`…), pero está **deprecado** ("Legacy permissions based on legacy user membership information", [zones](https://developers.cloudflare.com/api/resources/zones/methods/get/)). Un log real de 2023 (terraform-provider-cloudflare#2465), con un token, mostró justo los permisos del token. **NO VERIFICADO** que hoy siga así.
- Un `POST /certificates` con un CSR inválido **no sirve** de prueba. Lo más probable es que Cloudflare revise el CSR antes que la zona: en ese log, `1007` vino con `X-Envoy-Upstream-Service-Time: 0`. Un token sin el permiso recibiría el mismo `1007` o `1011`.
- **Cómo responde un token sin el permiso: NO VERIFICADO.** Los casos reportados son **HTTP 400** (no 403):
  - `10000 Authentication error` (terraform#1361, origin-ca-issuer#159);
  - `1010 Failed to validate requested hostname …: This zone is either not part of your account, or you do not have access to it` (terraform#1524).

**Errores y límites**
- El sobre es el v4 de siempre: `{success, errors[{code, message}], messages, result}`.
- Códigos de Origin CA, de un esquema oficial viejo:
  - `1002`: request_type no soportado;
  - `1005`: demasiados hostnames;
  - `1006`: hostnames repetidos;
  - `1007`: CSR vacío;
  - `1009`: requested_validity inválido;
  - `1010`: no se pudo validar un SAN o la zona;
  - `1011`: no se pudo leer el CSR;
  - `1012`: falta el zone id;
  - `1014`: ya revocado;
  - `1100` y `1101`: falla de la base de Cloudflare;
  - `1200` y `1201`: falla de la CA al firmar.
- Límite: **1200 peticiones cada 5 minutos por usuario**, sumando todas las llamadas (también las del DNS). Si se pasa, Cloudflare responde **429 y bloquea todas las llamadas por 5 minutos** ([limits](https://developers.cloudflare.com/fundamentals/api/reference/limits/)). No hay un límite propio de Origin CA documentado, ni un máximo de certificados por zona.
- **NO VERIFICADO**, aunque no hay nada que diga lo contrario: un certificado se puede emitir para un hostname que todavía no tiene registro DNS.
- Crear un certificado no cambia nada en el edge. Revocarlo sí.

### 1.2 Caddy

Verificado en el código de Caddy **v2.11.7** (la versión de `deploy/caddy/Dockerfile`, compilada con `caddy-dns/cloudflare v0.2.4`) y de certmagic v0.25.6. `load_pem` es parte del núcleo de Caddy: la imagen no cambia.

- **`apps.tls.certificates.load_pem`:** `[{"certificate": "<PEM>", "key": "<PEM>", "tags": ["…"]}]` (`modules/caddytls/pemloader.go`).
  - Se lee con `tls.X509KeyPair`, así que acepta la cadena completa con la hoja primero.
  - La clave puede ser `PRIVATE KEY` (PKCS#8), `EC PRIVATE KEY` o `RSA PRIVATE KEY`.
  - Los `tags` solo los lee `certificate_selection`.
  - `certificate`, `key` y `tags` pasan por `ReplaceKnown`, que expande `{env.*}` y `{file.*}`. Un PEM no trae llaves.
- **Automatic HTTPS y los certificados cargados** (`modules/caddyhttp/autohttps.go:202-223`): un host que tiene un certificado cargado no se maneja por ACME, salvo con `automatic_https.ignore_loaded_certificates`.
  - El TLS app carga los PEM en su Provision, antes de que el HTTP app junte los nombres, así que el orden es seguro.
  - Los redirects a HTTPS se arman igual.
  - Un comodín cargado frena ACME para todos sus subdominios. No aplica: muelle carga un hostname por certificado.
  - `skip_certificates` saca hosts de la emisión, pero no de los redirects. `skip` los saca de las dos.
- **Las políticas de automatización:** `subjects` "is a filter, not a command" (`modules/caddytls/automation.go`). Un host con `load_pem` que sigue en una política no se maneja igual. Lo más limpio es sacarlo de los `subjects` y nombrarlo en `skip_certificates`: así queda dicho en la configuración, aunque Caddy cambie su heurística de los certificados cargados.
- **Qué certificado se sirve:**
  - Los cargados y los manejados comparten un caché, sin prioridad entre ellos: decide el orden del índice.
  - `certificate_selection` tiene una trampa: si no hay candidatos para el nombre, filtra el caché entero (#5933).
  - muelle no la usa, porque un host con Origin CA nunca tiene un certificado de ACME en esa configuración (ver la alternativa descartada en §7.4).
- **Recargar con `POST /load`** es gradual. El `Cleanup` de la configuración vieja saca del caché los certificados que ya no están cargados, y conviven solo milisegundos. Una configuración idéntica byte a byte no hace nada, salvo con `Cache-Control: must-revalidate`.
- **Autosave:** `caddy.go:391` escribe el JSON tal cual en `/config/caddy/autosave.json` (0600, volumen `muelle_caddy_config`). **La clave privada queda ahí en claro**, como ya pasa con el token de Cloudflare (§8 del diseño), y también la devuelve `GET /config/`.
  - `load_files`, `load_storage` o `{file.…}` la sacarían del JSON, pero necesitan archivos dentro del contenedor de Caddy.
  - muelle solo comparte con Caddy el volumen del socket, y en disco la clave quedaría en claro igual. Se acepta, como el token (§15, R6).
- **OCSP:**
  - Al cargar un PEM con URL de OCSP, Caddy hace un pedido que bloquea el Provision hasta 30 s si el responder no contesta.
  - **NO VERIFICADO:** un análisis de SSL Labs muestra que los de Origin CA traen `http://ocsp.cloudflare.com/origin_ca`.
  - `apps.tls.disable_ocsp_stapling: true` lo apaga para los certificados cargados, que son justo estos.
  - Sin eso, un `POST /load` podría tardar más que el plazo de 30 s de muelle.
- **Caddy no renueva ni vigila los certificados cargados.** Solo avisa al cargarlos si vencieron o vencen en un día, y sirve igual uno vencido. La renovación y la vigilancia son de muelle.
- **`.test` es un nombre público para Caddy:** `SubjectIsInternal` solo cubre localhost, `.local`, `.internal` y `.home.arpa`. La integración ya lo resuelve con la política interna de `InternalCA`.

### 1.3 Lo que contradice o precisa lo que supuso el dueño

1. **"Verificar el permiso sin efectos secundarios" no se puede asegurar.** Cloudflare no documenta una forma, y un CSR inválido no prueba nada (§1.1). Propuesta (§3.3):
   - primero la pista `#ssl:edit` del objeto zona, sin efectos;
   - si falta, un certificado de 7 días para el apex de la zona, que se revoca enseguida;
   - el **Paso 0** del plan (§14) confirma con dos tokens reales qué responde Cloudflare.
2. **Revocar no es gratis:** con Full (strict), revocar un certificado que Caddy todavía sirve corta el sitio. La revocación va siempre después de una carga sin él (§6).
3. **`cert_issuer` no es un enum:** el contrato lo define como "lo que vio la verificación: la organización del emisor". Ponerle valores como `cloudflare_origin_ca` cambiaría su sentido y rompería el panel. Va un campo nuevo, `cert_source` (§10).
4. **`last_error_code` no sirve para avisar de una emisión que falló:** el dominio sigue sirviendo Let's Encrypt, así que no está en `error`. Ponerlo en `error` lo mostraría en rojo y lo sacaría de la verificación. Además, `last_error_code` tiene un `CHECK` en la tabla, que en SQLite obliga a reconstruirla. El aviso va en `origin_ca.error_code` y `origin_ca.error` (§9).
5. **15 años, el valor por defecto, no conviene** (§5.2).
6. **Un token sin el permiso recibe HTTP 400 (10000 o 1010), no 403.** El cliente actual lo trata como rechazo, que está bien, pero hay que clasificarlo por código (§9.1).
7. **El 429 de Cloudflare bloquea 5 minutos todo el token,** también el DNS de los dominios. El reconciliador corta la pasada apenas recibe un 429 (§9.2).

---

## 2. Lo que hay hoy en el código (`main` @ a1238d9)

Entre 58f7236 y a1238d9 entraron los menores de la integración. Para este diseño cambian dos cosas: `NoPlaceholders` también en install y en `MUELLE_INIT_*` (contrato §3.4), que no toca nada de acá, y que el panel ya no tiene caminos para un backend más viejo (`web/CLAUDE.md`, "Las fases 2, 3 y 5"): los campos nuevos son obligatorios en sus DTOs (§11). Los ajustes, las secciones del contrato y las migraciones siguen como abajo.

- **La configuración de Caddy:**
  - `ingress/domain/service/proxy_config.go` (`BuildProxyConfig`, pura) arma `ProxyConfig{Panel, Routes, ACMEEmail, CloudflareToken, InternalCA}`.
  - Cada `Route` lleva un `TLSMode`, que pasa a `http` si no hay token.
  - `infra/out/service/caddy/config.go` (`Render`) arma hasta dos políticas en `apps.tls.automation.policies`: `cloudflare` (ACME con DNS-01 y el token) y `http` (ACME). Con `InternalCA`, el emisor es `internal`.
  - El servidor es `:443` y no tiene `automatic_https`. Los golden están en `caddy/testdata/*.golden.json`.
- **Mandar la configuración:**
  - `usecases/sync.go` (`Syncer.Sync`, con un mutex y 30 s) lee los ajustes, `PanelDNS`, los dominios y las apps, y manda con `caddy.Admin.Load`.
  - `Admin.Load` (`caddy/admin.go`) reemplaza el token por `***` en los rechazos de Caddy (`redact`).
- **Los dominios:**
  - `entity.Domain` guarda `Proxied` (lo que pidió muelle), `TLSMode`, `CFZoneID`, `CFRecordID`, `Status`, `ErrorCode`, `CertIssuer` y `CertExpiresAt`.
  - `dnsOps` (`usecases/dns.go`) aplica `proxied` al A y al AAAA propios.
  - Un dominio de una zona cuyo registro muelle no maneja queda en `error`, con `record_exists` o `cloudflare`. Uno fuera de toda zona (o creado sin token) tampoco tiene `cf_record_id`, pero es `tls_mode: http` y puede estar `pending` o `active`.
  - Si alguien cambia el `proxied` en el dashboard de Cloudflare, muelle no se entera: no lo vuelve a leer.
- **El panel:**
  - `EnsurePanelDNS` (`usecases/panel_dns.go`) crea el A del panel con `proxied: false` y nunca cambia su `proxied`. Guarda `panel_cf_zone_id`, `panel_cf_record_id` y `_aaaa`.
  - El TLS del panel es `cloudflare` si hay token y zona.
- **Los ajustes:**
  - `shared/domain/entity/proxy_settings.go` (`ProxySettings`, `Encode`, `DecodeProxySettings`, `ChangedKeys`) es el contrato entre admin e ingress.
  - El token va cifrado con la AAD `settings:cloudflare_token`.
  - `admin/app/usecases/settings.go` (`PatchSettings.Execute`) valida (`applyProxy`), llama a `hook.CheckCloudflareToken` y a `hook.CheckProxyPorts` fuera de la transacción, guarda, audita y llama a `hook.ProxySettingsChanged`.
- **Verificar el token:**
  - `ingress/app/usecases/token.go` usa `GET /user/tokens/verify` y `GET /zones`.
  - Clasifica con `isRejection` en 422 o 502 (`unavailableErr`).
  - El cliente está en `infra/out/service/cloudflare/client.go` (sin SDK), y el falso en `cftest`.
- **Reconciliar:**
  - El job "proxy" (`usecases/proxy_job.go`) corre cada 5 min y al arrancar: redes, Caddy, huérfanas, Sync y, con token, `EnsurePanelDNS` (y otro Sync si cambió la zona del panel).
  - El job "certificados" corre cada hora.
  - `CertChecker` verifica a los 30 s, cada minuto los `pending`, y aplica `service.ApplyCert` (14 días y un plazo de 10 min).
  - La sonda es `TLSProbe` (`infra/out/service/certs.go`), que verifica contra `Roots` (nil son las del sistema).
- **El panel web:**
  - Ajustes → tarjeta "Cloudflare", en `web/src/modules/admin/ui/features/proxySettings/ProxySettings.svelte` y su viewModel.
  - Dominios → `web/src/modules/ingress/ui/components/domainCard/DomainCard.svelte`, con los textos de `labels/domainLabels.ts`.
  - El refresco lo decide `hasProvisioning`, en `data/queries/domainQueries.ts`.
- **Migraciones:**
  - La más alta es `0005_deploy_routes_warning.sql`.
  - La 0006 es de esta función (contrato §6), y la 0007 está reservada para el servidor MCP, que va en otra rama y suma su línea al contrato §6 cuando entra. El orden en que entran importa (§5.4).
  - Los ids que van en una AAD usan `AUTOINCREMENT`, y la fila se inserta primero con un valor vacío (patrón de `deploy/infra/out/repository/sqlite/registries.go`).

---

## 3. El ajuste `cloudflare_origin_ca`

### 3.1 Dónde vive

- `shared/domain/entity/proxy_settings.go`:
  - `SettingCloudflareOriginCA = "cloudflare_origin_ca"`, que entra en `ProxySettingKeys`;
  - el campo `CloudflareOriginCA bool` en `ProxySettings`;
  - `Encode` con `FormatBool` y `DecodeProxySettings` con `ParseBool`: `""` es `false`.
  - `ChangedKeys` la incluye sola.
- Se guarda en texto (`"true"`/`"false"`), sin cifrar.
- No hay `MUELLE_INIT_*` para esto: es opt-in, desde Ajustes.
- La lee el adaptador de ingress (`ProxySettingsReader`) sin cambios, porque recorre `ProxySettingKeys`.

### 3.2 `GET /settings` y `PATCH /settings`

- `GET` (y la respuesta del `PATCH`) suma `"cloudflare_origin_ca": false`, siempre presente.
- `PATCH` acepta `cloudflare_origin_ca` (bool). Cualquier otro tipo es `400 invalid_json`.
- En `admin/app/dto/dto.go`: `SettingsOutput.CloudflareOriginCA bool` y `PatchSettingsInput.CloudflareOriginCA *bool`.
- Las reglas, en este orden, dentro de `PatchSettings.Execute` (el texto exacto de cada mensaje está en el contrato, §4.1):

| # | Regla | Respuesta |
|---|---|---|
| 1 | Se enciende en este `PATCH` (de `false` a `true`) y, con los valores resultantes, no hay token. Va al final de `applyProxy`, después de la forma del token | `422`: `cloudflare_origin_ca: necesita el token de Cloudflare: configúralo antes de activarla`. No se guarda nada |
| 1b | Se quita en este `PATCH` un token guardado (`cloudflare_token: ""` con un token guardado; sin uno, `""` no quita nada) y la opción queda encendida (no viene `cloudflare_origin_ca: false`). Va junto con la regla 1 | `422`: `cloudflare_origin_ca: sin token no se pueden usar los certificados de Cloudflare: mándala en false en el mismo cambio para quitar el token`. No se guarda nada |
| 2 | Token nuevo: `CheckCloudflareToken`, igual que hoy (después del `409 would_lock_out`) | `422 cloudflare_token` / `502` |
| 3 | Con los valores resultantes la opción queda encendida, y o se enciende ahora o cambió el token: `hook.CheckCloudflareOriginCA(ctx, token, field)` (§3.3). `field` es `cloudflare_origin_ca` si se encendió en este `PATCH`, y si no `cloudflare_token` (el token reemplazado con la opción ya encendida) | `422` en `field`: `el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona auditfy.cl (lo necesitan los certificados de Cloudflare): agrégalo al token en Cloudflare, o deja la opción apagada` (en `cloudflare_token`, el final es `…, o apaga antes los certificados de Cloudflare`). `502 cloudflare_unavailable` si Cloudflare no responde (red, 5xx, 429) |
| 4 | `CheckProxyPorts`, igual que hoy, y después `409 totp_required_first`, dentro de la transacción | — |

- Las reglas 1 y 1b miran solo lo que cambia en este `PATCH`, no el estado guardado: un estado "encendida sin token" que llegó por un `UPDATE` a mano no frena un `PATCH` de otra cosa, ni uno con `cloudflare_token: ""`, que ahí no quita nada (y el reconciliador, sin token, no tiene candidatos, §5.5).
- Los tres chequeos van fuera de la transacción, como hoy. Si alguno falla, no se guarda nada del `PATCH`.
- `cloudflare_origin_ca: false` nunca llama a Cloudflare.
- Mandar el mismo valor que ya tiene no es un cambio.
- **Auditoría:** `settings.update` con `"cloudflare_origin_ca": true|false`. `auditValue` la devuelve como bool.
- **El hook:** `ProxySettingsChanged` aplica el cambio (§3.4).

### 3.3 `CheckCloudflareOriginCA` (ingress)

- Es un puerto nuevo en `admin/domain/service.ProxyHook`, `CheckCloudflareOriginCA(ctx, token, field string) error`.
- Lo implementan `ingress.Module` y el caso de uso `usecases.CheckOriginCAPermission`.
- Tiene un plazo de 30 s sobre el ctx de la petición: si el cliente se va, no se guarda nada.

Qué hace:
1. **Lista las zonas** con `dns.Zones(ctx, token)`, todas las páginas.
   - `service.Zone` suma `Permissions []string` y `Status string`, que el cliente lee de `permissions` y de `status` de cada zona (`initializing`, `pending`, `active` o `moved`, verificado en la referencia de `GET /zones`).
   - Un rechazo es `422` en `field`: `el token no puede listar las zonas (<mensaje de Cloudflare>): dale los permisos Zone:Read y DNS:Edit sobre las zonas de tus dominios`.
   - Si no ve ninguna zona: `422`, `el token no ve ninguna zona: dale los permisos Zone:Read y DNS:Edit sobre las zonas de tus dominios` (los mismos textos que `CheckCloudflareToken`, `msgTokenPermissions`).
   - Si Cloudflare no responde: `502`.
2. **Revisa cada zona activa que ve el token** (`status: active`).
   - Por qué todas, y no solo las que usan los dominios de hoy: en Cloudflare, un permiso de una política del token aplica a todas las zonas de esa política. Un token bien armado tiene SSL Edit donde tiene DNS Edit. Así, un dominio nuevo en cualquiera de esas zonas funciona sin sorpresas.
   - Por qué solo las activas (un cambio sobre la propuesta de 58f7236, que revisaba todas las que cuenta `cloudflare_zones`): en una zona `pending` (agregada a la cuenta sin cambiar los nameservers) Cloudflare todavía no hace de proxy, y lo más probable es que rechace la emisión con `1010` (R11). Revisarla frenaría la opción con un "falta el permiso" que no es cierto. Si el token no ve ninguna activa, no hay nada que revisar y pasa.
   - Va con a lo más 4 zonas a la vez.
   - a. Si `Permissions` trae `"#ssl:edit"`, la zona pasa. No hay efectos secundarios.
   - b. Si no (el campo falta o no lo trae), muelle hace una **prueba definitiva**:
     - genera una clave y un CSR (§5.1) para el apex de la zona (`zone.Name`);
     - `POST /certificates` con `{"hostnames": ["<zona>"], "request_type": "origin-ecc", "requested_validity": 7, "csr": "…"}`;
     - si sale bien, `DELETE /certificates/{id}` enseguida. Ese certificado nunca lo sirvió nadie, así que revocarlo no toca el edge.
     - Si el `DELETE` falla, el certificado se guarda como `retired` en `origin_ca_certs` (hostname = la zona, con la clave borrada) y el reconciliador lo revoca después (§6).
     - La clasificación del `POST` es la de §9.1:
       - `permission`: la zona queda en la lista de las que faltan;
       - `cloudflare`: `502`;
       - `rejected`: `422`, `Cloudflare rechazó la prueba del permiso en la zona <z> (<mensaje>)`.
3. Si faltan zonas: `422` en `field` con el texto de la regla 3. Las zonas van ordenadas por nombre: "en la zona a.cl", "en las zonas a.cl y b.cl", y con tres o más, las dos primeras y cuántas faltan: "en las zonas a.cl, b.cl y 3 más".
4. Ningún mensaje trae el token. El `<mensaje de Cloudflare>` que citan algunos se corta a 300 runas y pasa por el mismo borrado del token que `rejected()` del cliente, también en el camino nuevo de `/certificates`.

**El Paso 0 del plan (§14) confirma la pista y los códigos.** Lo hace el dueño a mano, con un token que tiene el permiso y otro que no:
- si `permissions` no refleja el token (el que no tiene el permiso igual muestra `#ssl:edit`), se saca el paso 2a y siempre se hace la prueba 2b;
- los códigos que devuelve el `POST` del token sin permiso quedan como fixtures de `cftest` y del cliente.

Que la pista dé un falso positivo no es grave: la emisión falla con `permission`, el dominio sigue con Let's Encrypt y avisa (§9).

### 3.4 Lo que aplica `ProxySettingsChanged`

Va dentro de `usecases/settings_changed.go`:
- Si cambió `cloudflare_origin_ca`, o cambió el token con la opción encendida, se reinicia el backoff de todos los hosts (`origin.ResetBackoff`) y se despierta al reconciliador (`origin.Kick()`, sin esperarlo).
- `cloudflare_origin_ca` se suma a las claves que hacen `syncer.Sync`. Al apagarla, la configuración vuelve a Let's Encrypt **antes** de que responda el `PATCH`. Al encenderla, el Sync no cambia nada: todavía no hay certificados, y la emisión sigue en segundo plano.
- Al apagarla, además, se borran las filas de `origin_ca_hosts` (`origin.Forget()`, sin el mutex de la pasada): lo observado y las fallas. Los `retired` de `origin_ca_certs` no se tocan, y se revocan igual. Así, al volver a encenderla, la API dice `issuing`, `dns_only` o `not_eligible`, y no una falla vieja.
  - La carrera: `Forget` no toma el mutex de la pasada, para no esperar hasta 5 min antes de responder. Si una pasada que empezó con la opción encendida escribe una fila después, esa fila queda hasta que la borra el paso 7 de una pasada con la opción apagada, y ese paso espera a que el host no tenga certificados, tampoco retirados sin revocar (§5.5).
  - Si la opción se vuelve a encender antes, la API puede mostrar lo que vio esa pasada (también un `failed`) hasta que otra pasada vuelve a mirar el host. Es solo lo que se ve: encenderla reinicia el backoff de todos (§9.2), así que la pasada que despierta lo reintenta enseguida. El contrato (§3.7) lo dice así.

---

## 4. Qué hostnames califican

### 4.1 Candidato, observación y "se sirve"

Son tres pasos, y los dos primeros son funciones puras en `ingress/domain/service/origin_ca.go`. Las usan, con la misma regla, `BuildProxyConfig`, la salida de la API y el reconciliador, así la API nunca dice algo distinto de lo que se le manda a Caddy.

**Candidato** (sin preguntar a Cloudflare). La opción tiene que estar encendida, tiene que haber token y el proxy tiene que estar habilitado (`ProxySettings.Enabled`, §4.3). Además:
- **Un dominio de una app:** es **elegible** si `TLSMode == cloudflare`, `CFZoneID != ""` y `Managed()` (muelle maneja el A), y es candidato si además tiene `Proxied` (lo que pidió muelle). Uno elegible sin `Proxied` es el `dns_only` de la API (§10.1).
  - El `status` del dominio no cuenta: uno en `error` + `cloudflare` (por ejemplo, porque falló el cambio de IP) sigue siendo candidato, y lo que decide es la observación.
- **El panel:** `panel_domain != ""` y `panel_cf_zone_id != ""` (elegible y candidato son lo mismo: el panel no tiene un `proxied` en muelle).

**Observación**, en cada pasada del reconciliador: `dns.Records(ctx, token, zone, hostname)`, una llamada por candidato, que trae todos los registros de ese nombre. El resultado:
- `proxied`: hay al menos un A, AAAA o CNAME y **todos** tienen `proxied: true`. En un dominio de una app, además, el A que maneja muelle (`CFRecordID`) está entre ellos;
- `dns_only`: alguno no tiene el proxy;
- `missing`: no hay ningún A, AAAA ni CNAME de ese nombre (el panel sin registro todavía) o, en un dominio de una app, todos tienen el proxy pero el A que maneja muelle no está entre ellos (alguien lo borró en Cloudflare);
- `unreadable`: Cloudflare rechazó la lectura (un 4xx, por ejemplo una zona que el token ya no ve).
- Si Cloudflare no responde (red, 5xx o 429), **no se cambia** lo observado antes. Con 429 se corta la pasada (§9.2).

Lo observado se guarda en `origin_ca_hosts.observed`, con `observed_at`. Un `PATCH` que cambia el `proxied` de un dominio lo vuelve a NULL (`origin.ForgetObserved(hostname)`, en el mismo caso de uso, antes del Kick): la API dice `issuing` hasta que la pasada lo lee, y no un `dns_only` viejo. `ForgetObserved` no borra la falla: con una guardada, la API dice `failed` (§10.1, paso 7), y se reintenta con su backoff o con `retry`.

**Se sirve con Origin CA** si se cumplen las cuatro cosas:
- el host es candidato;
- `observed == proxied`;
- hay un certificado `active` de ese host y de esa zona (`zone_id == CFZoneID`, o `panel_cf_zone_id`);
- y vence en más de 14 días (`service.RenewMargin`).

Si no, Let's Encrypt, como hoy.

Por qué se pide el proxy en muelle **y** en Cloudflare:
- Si el usuario pide `proxied: false` en el panel, Caddy vuelve a Let's Encrypt en el Sync de ese `PATCH`, aunque Cloudflare todavía no lo aplicó.
- Si alguien pone DNS only en el dashboard de Cloudflare, muelle lo ve en a lo más 5 min (el job "proxy") y vuelve a Let's Encrypt.
- Ante la duda (Cloudflare no responde), se queda con lo último que vio. Así un corte de la API no mueve los certificados.

### 4.2 Registros que muelle no maneja

- **Un dominio de una app sin `cf_record_id` no califica.** Fuera de toda zona (o creado sin token) es `tls_mode: http`, así que ni siquiera es elegible. En una zona (`cf_zone_id`), por qué:
  - un dominio de una zona sin `cf_record_id` siempre está en `error` (`record_exists`, o `cloudflare` porque falló la creación). No tiene sentido emitir para un dominio que todavía no funciona;
  - el registro ajeno puede apuntar a otro servidor. Es el caso de la migración desde Dokploy (§16 del diseño): el dominio se prepara en muelle con DNS-01 mientras Dokploy sigue sirviendo. Un certificado de origen ahí no lo usa nadie;
  - el contrato ya dice que `proxied` "solo tiene efecto si muelle maneja el registro", y el panel solo muestra el switch de Proxied en ese caso;
  - después de Adoptar pasa a calificar, sin hacer nada más.
- **El panel sí califica con un registro ajeno**, solo leyéndolo. Por qué:
  - en la v1, el registro ajeno del panel es un estado normal (no se adopta, §3.4 del contrato), no un error;
  - el panel lo sirve este muelle, así que no hay otro servidor detrás;
  - muelle nunca cambia el `proxied` del panel, ni siquiera el del registro que creó (lo crea con `false`). Para el panel, lo único que dice si hay proxy es lo que se ve en Cloudflare, y es lo mismo si el registro lo creó muelle o no;
  - muelle no toca el registro: solo lo lee.

### 4.3 Reglas del panel

- Sin `panel_domain` (desarrollo) o con el proxy deshabilitado (`ProxySettings.Enabled` falso), el reconciliador no hace nada. Con el proxy deshabilitado no hay Caddy al que darle un certificado, así que la API dice `origin_ca: null` en los dominios y en el panel, como con la opción apagada (§10.1).
- Si cambia `panel_domain` (`MUELLE_PANEL_DOMAIN` al arrancar), el host viejo deja de ser candidato: su certificado se retira y se revoca (§6).
- `PANEL_DOMAIN` nunca coincide con el de una app (`hostname: es el dominio del panel`), así que el hostname es único en las tablas nuevas.
- El estado del panel se ve en `GET /proxy` (`panel_certificate`, §10.3).
- **Ojo:** con el panel en Origin CA, quien entra directo al servidor (sin pasar por Cloudflare, por ejemplo con `/etc/hosts`) ve un certificado en el que su navegador no confía. Hay que pasar por Cloudflare, o apagar el proxy: en a lo más 5 min muelle vuelve a Let's Encrypt. La documentación lo dice.

---

## 5. Emisión

### 5.1 La clave y el CSR

- Es un puerto nuevo, `service.OriginKeys`, con el adaptador en `ingress/infra/out/service/origin_keys.go`. La aleatoriedad es infra.
- `Generate(hostname) (keyPEM, csrPEM string, err)`:
  - `ecdsa.GenerateKey(elliptic.P256(), rand.Reader)`;
  - `x509.CreateCertificateRequest` con `Subject.CommonName = hostname` y `DNSNames = [hostname]`;
  - la clave en PKCS#8 (`x509.MarshalPKCS8PrivateKey`), como `PRIVATE KEY`.
- `request_type: origin-ecc`.
  - Por qué ECC: con Full (strict), el edge de Cloudflare acepta las dos. ECC da claves y handshakes más chicos, y es lo que ya usan los certificados de Caddy.
- `Check(certPEM, keyPEM, hostname, now) (notAfter time.Time, err)` revisa lo que devolvió Cloudflare antes de guardarlo:
  - `tls.X509KeyPair` (lo mismo que hace Caddy);
  - la hoja se lee bien;
  - `VerifyHostname(hostname)`;
  - `NotBefore <= now+5min < NotAfter`.
  - Si falla, la emisión cuenta como `rejected` ("Cloudflare devolvió un certificado que no sirve: …") y no se guarda nada. Así, un PEM de muelle nunca puede hacer que Caddy rechace la configuración entera.
- El PEM del certificado se vuelve a codificar desde el DER, para normalizar los saltos de línea.

### 5.2 Validez: 90 días

`OriginCAValidityDays = 90` (una constante, no un ajuste). Por qué:
- **Es el mismo ciclo que Let's Encrypt:** el margen de 14 días del contrato (`RenewMargin`) y los 30 días de renovación siguen valiendo igual para todos los certificados de muelle.
- **El camino de la renovación se ejerce cada 2 meses.** Un error ahí se ve enseguida, no al año o a los 15 años.
- **La clave en claro tiene menos tiempo de vida.** Queda en `autosave.json` y en `GET /config/` (§1.2). Un certificado que no se pudo revocar (por ejemplo, porque cambió la cuenta del token) vence solo.
- **Las raíces de Origin CA vencen en 2029:** un certificado de 15 años emitido hoy viviría más que su raíz.
- **El costo es mínimo:** unas 6 llamadas por host al año, contra el límite de 1200 cada 5 min.
- **Riesgo aceptado:** la renovación depende de que muelle corra, porque Caddy no renueva los certificados cargados (§1.2). Con muelle caído, Caddy sigue sirviendo el certificado de su autosave hasta que vence, sin volver a Let's Encrypt (eso también lo hace muelle). Con 90 días y la renovación a los 30 que faltan, muelle tendría que estar caído más de 30 días, justo en esa ventana, para que el certificado venza.
- **365 días también sería defendible** (menos llamadas, y aguanta más tiempo con muelle caído). Se elige 90 por la uniformidad y por ejercer la renovación seguido.

### 5.3 Cuándo se emite y cuándo se renueva

En cada pasada (§5.5), para cada candidato con `observed == proxied` cuyo backoff ya venció (`next_attempt_at <= now`, o NULL: sin una falla no espera):
- **Se emite** si no tiene un certificado `active`, si el que tiene es de otra zona, o si su hostname no coincide;
- **se renueva** si el `active` vence en menos de 30 días (`OriginCARenewBefore = 30 * 24h`). Cada renovación lleva una clave nueva.
- Hay un certificado por hostname (`hostnames: [hostname]`). Así el ciclo de vida y la revocación van uno a uno con el dominio.
- Al guardar el nuevo, el anterior del mismo host pasa a `retired`, en la misma transacción, y se revoca después de la carga (§6).
- Entre los 30 y los 14 días se sigue sirviendo el viejo. Si la renovación falla, el aviso lo dice (§9.3).
- Cuando faltan menos de 14 días deja de servirse, y Caddy pide uno de Let's Encrypt.

### 5.4 Dónde se guarda: migración `0006_origin_ca.sql`

- En `main` @ a1238d9 la más alta sigue siendo `0005_deploy_routes_warning.sql`. La 0006 es de esta función (contrato §6), y la 0007 está reservada para el servidor MCP, que va en otra rama. Los números no se cambian: el runner aplica en orden las versiones mayores que `MAX(version)`, y en los tests, sobre una base nueva, un hueco (la 0006 sin la 0007, o al revés) no molesta.
- **El orden en que entran a `main` sí importa.** `sqlite.Migrate` salta toda versión menor o igual que `MAX(version)`. Si la 0007 llega primero a una base que ya existe (por ejemplo, la del servidor de desarrollo del dueño con un binario que ya trae la del MCP), la 0006 no se aplica nunca en esa base, y todo lo que usa `origin_ca_*` falla en ejecución con `no such table`. Por eso:
  - la línea 1 (con la 0006) entra a `main` antes que la 0007 del MCP;
  - si el MCP tiene que entrar antes, la salida no se decide en una rama: se acuerda antes (contrato §1.3) renumerar o cambiar el runner para que aplique cada versión que falta, no solo las mayores que `MAX(version)`;
  - una base de desarrollo que ya corrió la 0007 sin la 0006 se recrea.
- Es una migración nueva, no un cambio a `0003_ingress.sql`, por lo mismo que la 0004 y la 0005: para que corra sobre una base que ya tiene dominios.

```sql
-- Certificados de Cloudflare (Origin CA), opcionales (contrato §3.7): los que
-- emitió muelle y lo que vio de cada hostname en Cloudflare. Es una migración
-- nueva, y no un cambio a 0003_ingress.sql, para que corra también sobre una
-- base que ya tiene dominios.

-- AUTOINCREMENT: el id va en la AAD de la clave,
-- origin_ca_certs:private_key:<id> (§12 del diseño).
CREATE TABLE origin_ca_certs (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  -- El de un dominio o el del panel. Sin FK: el panel no está en domains, y
  -- un certificado retirado sobrevive a su dominio hasta que se revoca.
  hostname        TEXT    NOT NULL,
  zone_id         TEXT    NOT NULL,
  -- El id de Cloudflare (el serial en decimal), para revocarlo.
  cf_cert_id      TEXT    NOT NULL UNIQUE,
  certificate     TEXT    NOT NULL,  -- PEM de la hoja
  -- PKCS#8 en PEM, cifrado con la clave maestra. x'' en uno retirado: ya no
  -- se necesita.
  private_key     BLOB    NOT NULL,
  expires_at      INTEGER NOT NULL,  -- NotAfter, en ms
  state           TEXT    NOT NULL CHECK (state IN ('active', 'retired')),
  created_at      INTEGER NOT NULL,
  retired_at      INTEGER,
  revoke_failures INTEGER NOT NULL DEFAULT 0,
  next_revoke_at  INTEGER,
  CHECK ((state = 'retired') = (retired_at IS NOT NULL))
);
-- A lo más un certificado activo por hostname.
CREATE UNIQUE INDEX origin_ca_certs_active ON origin_ca_certs(hostname) WHERE state = 'active';

CREATE TABLE origin_ca_hosts (
  hostname        TEXT    PRIMARY KEY,
  -- Lo último que se vio en Cloudflare (contrato §3.7); NULL: todavía no se miró.
  observed        TEXT    CHECK (observed IN ('proxied', 'dns_only', 'missing', 'unreadable')),
  observed_at     INTEGER,
  -- La última emisión o renovación que falló, y cuándo se reintenta.
  failures        INTEGER NOT NULL DEFAULT 0,
  last_error_code TEXT    CHECK (last_error_code IN ('permission', 'cloudflare', 'rejected')),
  -- Solo la causa; el mensaje entero se arma al leer, según el status (§9.3).
  last_error      TEXT,
  next_attempt_at INTEGER,
  updated_at      INTEGER NOT NULL,
  CHECK ((last_error_code IS NULL) = (last_error IS NULL)),
  CHECK ((failures = 0) = (last_error_code IS NULL)),
  -- Con una falla, siempre hay un reintento: retry_at nunca es null con error_code.
  CHECK ((failures = 0) = (next_attempt_at IS NULL))
);
```

- **El cifrado:**
  - AES-256-GCM con la clave maestra (`package/cipher`, la de `Deps.Cipher` de ingress), con la AAD `origin_ca_certs:private_key:<id>`.
  - Se suma a la lista canónica de `shared/infra/out/crypto/crypto.go` y a §12 del diseño.
  - El repositorio inserta con `private_key = x''` y la reemplaza en la misma transacción, como `registries`.
  - Al retirar uno: `state = 'retired'`, `retired_at` y `private_key = x''`.
- **La clave nunca sale:**
  - no va al log, ni a un error, ni a la API, ni a la auditoría;
  - `ProxyConfig` y los structs nuevos (`OriginPEM`, `OriginCert`) la ocultan en `String`, `GoString`, `LogValue` y `MarshalJSON`, como hoy el token;
  - `caddy.Admin.Load` también reemplaza por `***`, en un rechazo de Caddy, cada clave PEM y cada línea base64 de 16 caracteres o más de esas claves (se extiende `redact`/`tokenForms`).
  - El log de una emisión trae el hostname, `cf_cert_id` y el vencimiento, nada más.
- El respaldo diario (`VACUUM INTO`) lleva las claves cifradas, como los demás secretos.

### 5.5 El reconciliador `EnsureOriginCA`

Va en `ingress/app/usecases/origin_ca.go`. Tiene su propio mutex: una pasada a la vez.

- **Cuándo corre:**
  - lo despierta un `Kick()` que no bloquea, con un canal de 1;
  - su goroutine es parte de `Module.Run`.
  - Lo despiertan:
    - el job "proxy", al final, cada 5 min y al arrancar;
    - `ProxySettingsChanged` (§3.4);
    - crear, editar, adoptar, reintentar o borrar un dominio, siempre después de su Sync.
- **Plazos:** cada pasada tiene 5 min (`Timing.OriginPass`), y cada llamada a Cloudflare el timeout del cliente (15 s). Escribe solo en sus tablas, sin el lock de `Ops`: no toca `domains`.

Una pasada:
1. Lee los ajustes y `PanelDNS`.
   - Con el proxy deshabilitado, no hace nada.
   - Con la opción apagada, o sin token, el conjunto de candidatos queda vacío.
2. Lee los candidatos (§4.1), los certificados `active` y `origin_ca_hosts`.
3. Para cada candidato, ordenado por hostname: lo observa y guarda `observed`.
   - Si queda `proxied`, hace falta un certificado (§5.3) y el backoff ya venció, emite: genera la clave y el CSR, llama a `POST /certificates` con 90 días, hace el `Check` y guarda el nuevo `active` (y retira el anterior).
   - Si la emisión sale bien, borra la falla del host. Si falla, la registra con su backoff (§9.2).
   - Un 429 corta la pasada (pasos 3 y 6).
4. Retira los `active` que ya no se van a servir: la opción apagada o sin token; el host que ya no es candidato (dominio borrado, sin proxy en muelle, registro que ya no maneja, otra zona, otro `panel_domain`); lo observado en `dns_only`, `missing` o `unreadable`; o un certificado vencido.
   - **No** retira por un `observed` sin cambios porque Cloudflare no respondió.
   - Un certificado de otra zona se retira y, en el paso 3, se emite uno nuevo para la zona nueva.
5. Si algo cambió (un `active` nuevo, uno retirado, un `observed` distinto), llama a `syncer.Sync(ctx, false)`. Si falla, la pasada termina sin revocar: el error ya quedó en `GET /proxy`, y lo reintenta el Retrier.
6. Revoca los `retired` que ya se pueden revocar (§6).
7. Borra las filas de `origin_ca_hosts` de los hostnames que no son candidatos y no tienen certificados.

---

## 6. Revocación

**Cuándo:**
- la opción se apaga;
- el dominio pierde el proxy (en muelle, o se ve DNS only en Cloudflare);
- se borra el dominio;
- cambia `panel_domain`;
- cambia la zona del dominio;
- se renueva (el viejo);
- un `DELETE` de la prueba de §3.3 que falló.

**Sin token**, no se puede revocar. Los `retired` quedan hasta que vuelva un token, o hasta que venzan: ahí se borran sin revocar.

**Cambiar el token no revoca por sí solo.** El certificado y la clave no dependen del token. Revocar y reemitir en cada rotación sería churn, con riesgo de corte.
- Con la opción encendida, el token nuevo ya pasó `CheckCloudflareOriginCA` en todas sus zonas.
- Lo que deje de calificar con el token nuevo (una zona que ya no ve: `unreadable`) se retira por la regla de siempre.
- Un certificado que el token nuevo no puede revocar (otra cuenta) se olvida cuando vence, en a lo más 90 días.

**El orden: esto es lo que no se puede romper.** Revocar uno que Caddy todavía sirve corta el sitio con Full (strict) (§1.1). Un `retired` se revoca solo si se cumplen dos cosas:
1. `retired_at < syncer.LastGoodBuild()`. `LastGoodBuild` es un valor nuevo del `Syncer`: el momento en que **empezó a leer la base** el último `Sync` cuyo `POST /load` salió bien.
   - Se toma antes de leer los ajustes, con el mutex tomado.
   - No sirve `synced_at` (el fin del `Load`): un Sync que leyó la base antes del retiro y terminó después mandó una configuración que todavía lo trae.
   - `retired_at` es la hora en que **se escribe** el retiro, no la del comienzo de la pasada (que pudo ser minutos antes, con las llamadas a Cloudflare del paso 3): un Sync que empezó en el medio todavía lo mandó. Vale para el paso 4 y para el viejo de una renovación (el `created_at` del nuevo).
   - El retiro se escribe con el mutex del `Syncer` tomado (`Syncer.Retiring`): ningún Sync lee la base mientras tanto. La hora sale de la misma marca que el comienzo de un Sync, que nunca va hacia atrás aunque el reloj lo haga.
2. Además, pasó `Timing.OriginRevokeGrace` (1 min) desde `retired_at`. Cubre el instante en que conviven la configuración vieja y la nueva en el caché de Caddy (§1.2), y los handshakes en curso.

**Resultado de `DELETE /certificates/{id}`:**
- sale bien, `404` o `1014` (ya revocado): se borra la fila;
- Cloudflare no responde: `revoke_failures++` y `next_revoke_at` con backoff (5 min, doblando, hasta 24 h);
- rechazo de permiso: lo mismo, con 24 h;
- vencido (`expires_at < now`): se borra sin llamar a Cloudflare.

Se registra en el log (Info), sin auditoría: es una acción del sistema, como las verificaciones.

---

## 7. La configuración de Caddy

### 7.1 Datos

- `service.Route` y `service.PanelRoute` suman `OriginCA *OriginPEM`, donde `OriginPEM{CertPEM, KeyPEM string}` oculta la clave.
- Es nil si el host no se sirve con Origin CA (§4.1).
- `BuildProxyConfig` recibe además `origin service.OriginInputs{Certs map[string]OriginCert, Hosts map[string]OriginHost, Now time.Time}`.
  - El `Syncer.build` lo arma: lee los `active`, descifra las claves y lee `origin_ca_hosts`.
  - `Now` sale del reloj del `Syncer`.
- Sigue siendo pura.

### 7.2 El JSON

Para los hosts con `OriginCA != nil`:
- un elemento en `apps.tls.certificates.load_pem`, ordenado por hostname, con los tags `["muelle_origin_ca", "muelle_origin_ca:<hostname>"]`. Los tags no los usa nadie; sirven para identificarlos en `GET /config/`;
- el hostname en `apps.http.servers.muelle.automatic_https.skip_certificates`, ordenado;
- el hostname **no** va en ninguna política de `apps.tls.automation.policies`;
- `apps.tls.disable_ocsp_stapling: true`, solo si hay al menos un `load_pem`. Así los golden actuales no cambian.

El resto no cambia: las rutas, `trusted_proxies`, el panel y las políticas `cloudflare` y `http` de los demás hosts.
- Sin políticas, no va `automation`. Si no hay ni políticas ni certificados, no va `apps.tls`, como hoy.
- Los redirects HTTP→HTTPS los sigue armando automatic HTTPS para todos los hosts.

Ejemplo (golden `origin_ca.golden.json`): el panel y `api.auditfy.cl` con Origin CA, `web.auditfy.cl` en DNS only (DNS-01) y `grafana.ejemplo.com` fuera de Cloudflare (HTTP-01). Se abrevian los rangos y las rutas, que son iguales que en `full.golden.json`:

```json
{
  "admin": { "listen": "unix//run/caddy-admin/admin.sock" },
  "apps": {
    "http": {
      "servers": {
        "muelle": {
          "listen": [":443"],
          "trusted_proxies": { "source": "static", "ranges": ["173.245.48.0/20", "…"] },
          "client_ip_headers": ["CF-Connecting-IP", "X-Forwarded-For"],
          "trusted_proxies_strict": 1,
          "routes": ["…igual que hoy…"],
          "automatic_https": {
            "skip_certificates": ["api.auditfy.cl", "panel.auditfy.cl"]
          }
        }
      }
    },
    "tls": {
      "certificates": {
        "load_pem": [
          {
            "certificate": "-----BEGIN CERTIFICATE-----\nMIIB…\n-----END CERTIFICATE-----\n",
            "key": "-----BEGIN PRIVATE KEY-----\nMIGH…\n-----END PRIVATE KEY-----\n",
            "tags": ["muelle_origin_ca", "muelle_origin_ca:api.auditfy.cl"]
          },
          {
            "certificate": "-----BEGIN CERTIFICATE-----\nMIIC…\n-----END CERTIFICATE-----\n",
            "key": "-----BEGIN PRIVATE KEY-----\nMIGI…\n-----END PRIVATE KEY-----\n",
            "tags": ["muelle_origin_ca", "muelle_origin_ca:panel.auditfy.cl"]
          }
        ]
      },
      "automation": {
        "policies": [
          {
            "subjects": ["web.auditfy.cl"],
            "issuers": [{
              "module": "acme",
              "email": "ops@auditfy.cl",
              "challenges": { "dns": {
                "provider": { "name": "cloudflare", "api_token": "cf-token-1234567890abcdef" },
                "resolvers": ["1.1.1.1:53", "1.0.0.1:53"]
              } }
            }]
          },
          {
            "subjects": ["grafana.ejemplo.com"],
            "issuers": [{ "module": "acme", "email": "ops@auditfy.cl" }]
          }
        ]
      },
      "disable_ocsp_stapling": true
    }
  }
}
```

Los structs, en el orden de su JSON, en `caddy/config.go`:
- `server` suma `AutomaticHTTPS *autoHTTPS \`json:"automatic_https,omitzero"\``, con `autoHTTPS{SkipCertificates []string \`json:"skip_certificates"\`}`;
- `tlsApp` queda `{Certificates *tlsCertificates \`json:"certificates,omitzero"\`; Automation *automation \`json:"automation,omitzero"\`; DisableOCSPStapling bool \`json:"disable_ocsp_stapling,omitzero"\`}`, con `tlsCertificates{LoadPEM []pemPair \`json:"load_pem"\`}` y `pemPair{Certificate, Key string; Tags []string}`.

**Placeholders:** `Render` rechaza, con `ErrPlaceholderInSettings`, un PEM o un tag que traiga `{` o `}`. No pasa (es base64 y un hostname validado), pero Caddy los expandiría.

### 7.3 Cómo sigue funcionando ACME para los demás

- Automatic HTTPS junta los hosts de las rutas.
- Saca los de `skip_certificates` y los que tienen un certificado cargado.
- Al resto le aplica la política cuyo `subjects` los nombra, igual que hoy.
- Al pasar un host de Origin CA a Let's Encrypt, sale de `load_pem` y de `skip_certificates` y vuelve a su política (la `cloudflare`, DNS-01). El `Cleanup` de la configuración vieja saca el certificado cargado del caché, y Caddy emite.
- Si Caddy todavía tiene en `/data` un certificado de Let's Encrypt válido de ese host (de antes del cambio, si pasaron menos de unos 60 días), lo usa enseguida.
- Si no, hay una ventana mientras emite. Con DNS-01 suele ser menos de un minuto, y mientras tanto el edge puede responder 525 (§15, R3).

### 7.4 Alternativas descartadas

- **Mantener Let's Encrypt "tibio"** para los hosts con Origin CA, para que la vuelta no tenga ventana:
  - el host en `apps.tls.certificates.automate`;
  - `tls_connection_policies` con `match.sni: [host]` y `certificate_selection.any_tag: ["muelle_origin_ca:<host>"]`, más una `{}` al final.
  - Se descarta en la v1 por tres razones:
    - con dos certificados del mismo nombre en el caché, el que se sirve depende del orden del índice, que sobrevive a las recargas;
    - la trampa #5933 de `certificate_selection`;
    - una emisión y renovación de Let's Encrypt por host que nadie usa.
  - Queda como mejora, si R3 molesta en la práctica.
- **`load_files`/`load_storage`/`{file.…}` para sacar la clave del JSON:** muelle no puede escribir en el sistema de archivos de Caddy, salvo en el volumen del socket, y en disco quedaría en claro igual (§1.2).

---

## 8. Verificación de certificados

- El puerto cambia: `CertProbe.Probe(ctx, hostname string, pinned [][]byte)`. `pinned` son los DER de las hojas de Origin CA que Caddy puede estar sirviendo para ese host: ninguna, una, o dos en una renovación.
- `TLSProbe` (`infra/out/service/certs.go`): con `pinned`, una hoja servida que es **idéntica** a una de `pinned`, para ese hostname (`VerifyHostname`) y vigente, es `Valid`, sin verificar la cadena contra `Roots`.
- Una hoja que verifica contra `Roots` sigue siendo `Valid`, con o sin `pinned`. Así, durante un cambio de origen (Let's Encrypt ↔ Origin CA), los dos son buenos, y un `active` no pasa a `error` por una carrera entre la sonda y la recarga.
- `Issuer` y `NotAfter` salen de la hoja, igual que hoy.
  - En uno de Origin CA, `cert_issuer` es la organización de la raíz de Cloudflare (probablemente `CloudFlare, Inc.`; NO VERIFICADO).
  - El panel no depende de ese texto: usa `cert_source`.
- `CertChecker.check` y `checkLocked` pasan en `pinned`:
  - la hoja del `active` cuando, con los datos de ese momento, el host se sirve con Origin CA. Usan la misma función de §4.1;
  - y las de sus `retired` que ningún Sync bueno dejó afuera todavía (`retired_at >= LastGoodBuild`, la regla de §6). En una renovación, `Insert` deja `active` la nueva en el paso 3, pero Caddy sirve la vieja hasta el Sync del paso 5: sin esto, el job "certificados" dejaría el dominio en `error` en ese rato.
- No se embeben las raíces de Origin CA: fijar la hoja es más estricto y no depende de que Cloudflare rote sus raíces (vencen en 2029).
- La regla de `ApplyCert` no cambia: 14 días y 10 min.
- El panel no tiene fila en `domains`, así que no se verifica (igual que hoy). Lo que se sabe de él lo dice `panel_certificate` (§10.3).

---

## 9. Fallas, reintentos y backoff

### 9.1 Cómo se clasifica un error de `POST /certificates`

`service.ClassifyOriginError(err) OriginErrorCode`, pura:

| Error | Código |
|---|---|
| Envuelve `entity.ErrCloudflareUnavailable` (sin red, 5xx, 429, respuesta ilegible) | `cloudflare` |
| `*DNSError` con HTTP 401 o 403, o código `10000`, o código `1010` | `permission` (incluye "API Access" apagado y la zona de otra cuenta) |
| `*DNSError` con código `1100`, `1101`, `1200` o `1201` (fallas internas de Cloudflare, aunque vengan como 4xx) | `cloudflare` |
| Otro `*DNSError` | `rejected` |
| El certificado devuelto no pasa `OriginKeys.Check` | `rejected` |

### 9.2 Backoff

`service.OriginBackoff(failures, code)`, por host:
- `cloudflare`: 1 min, 2, 4… hasta 1 h;
- `permission` y `rejected`: 6 h, fijo. No sirve insistir hasta que alguien cambie el token o la zona.

Lo reinician:
- el `POST …/domains/{hostname}/retry` de ese dominio (además, despierta al reconciliador);
- un `PATCH /settings` que enciende la opción o cambia el token;
- una emisión que sale bien.

Reiniciarlo (`ResetBackoff`, en los dos primeros casos) pone `next_attempt_at` en la hora del pedido, solo en los hosts con una falla (sin falla queda NULL, por el `CHECK` de §5.4), y deja `failures`, `last_error_code` y `last_error`: el aviso se sigue viendo hasta que la pasada que se despierta emite o vuelve a fallar. Solo una emisión que sale bien borra la falla, con `next_attempt_at`. Mientras tanto, `retry_at` es la hora de ese pedido (en el pasado).

**El panel no tiene `retry`** (no está en `domains`): el suyo lo reinicia solo un `PATCH /settings` que cambia el token (encender la opción, para el panel, es apagarla y encenderla, que revoca los de todos los dominios). Por eso su causa `permission` no nombra Reintentar (§9.3), y Ajustes no tiene un botón para él (§11.1). Mandar otra vez `cloudflare_origin_ca: true`, o el mismo token, no es un cambio (§3.2) y no reinicia nada.

**Tampoco hay un reinicio de todos a la vez.** Después de arreglar en Cloudflare el permiso del mismo token, cada dominio tiene su Reintentar, y el panel y los demás esperan su reintento solo, a lo más 6 h. Mientras tanto sirven Let's Encrypt, que con Full (strict) también vale. Se deja para después (R18): sería un cambio de contrato, por ejemplo que mandar `true` con la opción encendida compruebe el permiso y reinicie el backoff de todos.

**429:** "bloquea todas las llamadas por 5 minutos". La pasada que lo recibe termina ahí, sin más llamadas, y la siguiente es la del próximo job "proxy". Cuenta para el backoff del host que lo recibió, como `cloudflare`.

**Presupuesto:** cada pasada hace, como mucho:
- una lectura por candidato;
- una emisión por host que la necesita;
- una revocación por cada certificado retirado.

Con 100 dominios con proxy son unas 100 llamadas cada 5 min, contra 1200. Al encender la opción, unas 200 de una vez.

### 9.3 Qué se ve

- **El dominio no pasa a `error`.** Sigue con su estado de siempre, que decide la verificación del certificado que se sirve: Let's Encrypt mientras tanto.
- El aviso va en `origin_ca`, en `GET …/domains` y en `panel_certificate`:
  - `status` es `failed` si no hay un Origin CA que se pueda servir;
  - es `active`, con `error`, si falló una renovación pero el certificado actual todavía sirve.
- `error_code` es `permission`, `cloudflare` o `rejected`.
- `error` es una oración en español, que el panel muestra tal cual.
  - La plantilla sigue al `status` de la API, no a lo que falló: `OriginCAView` (§10.1) arma el mensaje al leer, con `last_error` (que guarda solo la `<causa>`) y el `active` de ese momento. Con `failed` va la de emisión, también cuando falló una renovación y el actual ya no se sirve (menos de 14 días, aunque la fila siga `active`); la de renovación, solo con `active`. Así el mensaje nunca promete una vuelta a Let's Encrypt que ya pasó.
  - emisión (con `failed`): `muelle no pudo emitir el certificado de Cloudflare: <causa>; mientras tanto usa Let's Encrypt`;
  - renovación (con `active`): `muelle no pudo renovar el certificado de Cloudflare, que vence el <fecha>: <causa>; si no lo renueva antes del <fecha − 14 días>, vuelve a Let's Encrypt`.
  - `<causa>` según el código:
    - `permission`, en un dominio: `el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona de <hostname> (Cloudflare: <mensaje>); agrégalo al token y pulsa Reintentar`;
    - `permission`, en el panel, que no tiene `retry` (§9.2): `el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona de <hostname> (Cloudflare: <mensaje>); agrégalo al token: muelle lo reintenta solo cada 6 h`;
    - `cloudflare`: `Cloudflare no respondió (<detalle>); se reintenta solo`;
    - `rejected`: `Cloudflare lo rechazó (<mensaje>)`, o `Cloudflare devolvió un certificado que no sirve (<motivo>)`.
  - Las fechas van con `shortDate` ("2 ene 2027"), y los mensajes de Cloudflare se cortan a 300 runas y nunca traen el token.
- `retry_at` es `next_attempt_at`, que con una falla guardada nunca es null (§5.4).
- `error_code`, `error` y `retry_at` van solo con `failed` y con `active` (una renovación que falló). Con los demás estados son null, aunque `origin_ca_hosts` todavía tenga la falla (un dominio que pasó a DNS only después de fallar).
- Una emisión que falla queda en el log (Warn) con el hostname y el código.

---

## 10. Cambios de la API

### 10.1 El dominio (contrato §3.1)

- `dto.DomainOutput` suma `CertSource string \`json:"cert_source"\`` y `OriginCA *OriginCAOutput \`json:"origin_ca"\``, sin omitempty.
- `toOutput` recibe además la vista de Origin CA del host: los ajustes, su `active` y su fila de `origin_ca_hosts`.
- `ListDomains` la lee una vez para todos; las demás operaciones, para su hostname.

`origin_ca` y `cert_source` salen de una función pura, `service.OriginCAView(host, settings, enabled, active, hostRow, now)`, la misma que decide `OriginCA` en `BuildProxyConfig` (§4.1). En este orden:
1. La opción apagada, sin token o con el proxy deshabilitado (§4.3): `origin_ca: null`.
2. No es elegible (§4.1: `tls_mode` no es `cloudflare`, sin `cf_zone_id` o sin `cf_record_id`; el panel sin `panel_cf_zone_id`): `not_eligible`.
3. Un dominio de una app elegible con `proxied: false` en muelle: `dns_only`.
4. Lo observado es `dns_only`: `dns_only`.
5. Lo observado es `missing` o `unreadable`: `not_eligible` (muelle no ve un registro con el proxy que pueda usar).
6. Se sirve con Origin CA (§4.1): `active`, con `error_code`/`error`/`retry_at` si la última renovación falló.
7. Tiene una falla guardada (`failures > 0`): `failed`.
8. Si no: `issuing` (también con `observed` todavía NULL: no se miró Cloudflare).

`cert_source` es `cloudflare_origin_ca` si y solo si `origin_ca.status` es `active`; si no, `letsencrypt`. `expires_at` es el `NotAfter` del `active` de ese hostname (null si no hay), con cualquier estado.

### 10.2 El texto del contrato

Está en el contrato, que manda: §3.1 (`cert_source` y `origin_ca` en el dominio), §3.2 (`retry`), §3.3 (`panel_certificate` en `GET /proxy`), §3.4 (TLS, el panel y los errores de Caddy), §3.5 (`CheckCloudflareOriginCA` y `ProxySettingsChanged`), §3.7 (la sección nueva: qué hostnames, los estados en el orden de §10.1, los textos y los ejemplos), §4 y §4.1 (`cloudflare_origin_ca` en `GET`/`PATCH /settings`, con los mensajes exactos de los `422`), §1.1, §1.2, §6 (la 0006) y §7 (las filas de los cambios).

Lo que cambió al pasar el texto al contrato, sobre la propuesta escrita contra 58f7236:
- las reglas 1 y 1b de §3.2 miran solo lo que cambia en el `PATCH`;
- el permiso se revisa solo en las zonas activas (§3.3);
- `origin_ca` es null también con el proxy deshabilitado, `missing` incluye el A de muelle que ya no está, y los estados van en el orden de §10.1, con `error_code`/`error`/`retry_at` solo con `failed` y `active`;
- `retry_at` después de un reinicio del backoff (§9.2);
- los campos nuevos son obligatorios para el panel (§11).

### 10.3 `GET /proxy`

- `dto.ProxyOutput` suma `PanelCertificate *PanelCertificateOutput \`json:"panel_certificate"\``, con `{Hostname, CertSource string; OriginCA *OriginCAOutput}`.
- Lo arma `GetProxyStatus`, que ahora también lee `PanelDNS`, el `active` del panel y su fila de `origin_ca_hosts`.
- Con `disabled`, también va (no consulta Docker).

```json
"panel_certificate": {
  "hostname": "panel.tudominio.cl",
  "cert_source": "cloudflare_origin_ca",
  "origin_ca": { "status": "active", "error_code": null, "error": null, "retry_at": null, "expires_at": "2027-01-19T12:00:00Z" }
}
```

### 10.4 Lo que se agrega al diseño (`2026-10-09-muelle-design.md`)

Está en el diseño: §5.1 (`cloudflare_origin_ca` en `settings`, las tablas `origin_ca_certs` y `origin_ca_hosts`, y la 0006), §8 (TLS, el autosave y la verificación con la hoja fijada), §9 (la lectura del `proxied` de los registros), §11 (Ajustes), §12 (la AAD `origin_ca_certs:private_key:<id>`), §13 (la fila de una emisión que falla) y §15 (los golden, el Cloudflare falso y la integración).

---

## 11. El panel (`web/`)

### 11.1 Ajustes → tarjeta "Cloudflare"

**Datos:**
Los campos nuevos son **obligatorios** en los DTOs, como los demás que sumaron las Fases 2 y 3 (`web/CLAUDE.md`, "Las fases 2, 3 y 5"): el panel va embebido con el backend, y una respuesta sin ellos es un `ContractError`. (La propuesta escrita contra 58f7236 los hacía opcionales para un backend sin esta función; ese camino ya no existe en el panel.)
- `adminDto.ts` suma `cloudflare_origin_ca: z.boolean()`.
- `CloudflareStatus` (`domain/entities/settings.ts`) suma `readonly originCa: boolean`.
- `SettingsPatch.cloudflareOriginCa?: boolean` se mapea a `cloudflare_origin_ca` en `settingsPatchPayload` (`SettingsPatchPayload.cloudflare_origin_ca?: boolean`).
- `proxyDto` (ingress) suma `panel_certificate: panelCertificateDtoSchema.nullable()` (null sin `panel_domain`), y `ProxyStatus` suma `panelCertificate: PanelCertificate | null`. Va en el barrel de ingress.

**Dominio y casos de uso:**
- `setCloudflareOriginCa(enabled)`: encenderla sin `tokenSet` rechaza con `OriginCaNeedsToken` (`ValidationError` en `cloudflareOriginCa`), sin llamar al backend.
- `removeCloudflareToken`: con `originCa: true`, manda `{cloudflare_token: "", cloudflare_origin_ca: false}`.
- El repositorio suma `cloudflare_origin_ca: 'cloudflareOriginCa'` en `fields` (`httpAdminRepositories.ts`). El `502` ya es `CloudflareUnavailable`.

**La vista** (`ProxySettings.svelte` + viewModel):
- Debajo del estado del token va una fila con `Switch` y el título "Certificados de Cloudflare (Origin CA)", con su ayuda debajo.
- Sin token, el switch está deshabilitado y la ayuda suma "Configura el token de Cloudflare para activarlos".
- Mientras guarda, el switch está deshabilitado con `loading`. El switch muestra siempre lo guardado: no es optimista.
- Encender no pide confirmación. Apagar abre un `ConfirmDialog` (la vuelta a Let's Encrypt puede dar `525` un momento).
- Si sale bien:
  - toast;
  - se invalidan `ingressKeys.all`: los dominios recalculan `cert_source` y `origin_ca`, y cambia `GET /proxy`.
- Los errores:
  - `ValidationError` en `cloudflareOriginCa` (el `422` del backend): `Banner` de error en la tarjeta, con el mensaje del backend (`sentence`), y el switch vuelve a lo guardado;
  - `CloudflareUnavailable`: Banner "Cloudflare no respondió: no se cambió nada. Vuelve a intentarlo en un momento.";
  - lo demás: `describeError`.
- En el modal del token, con la opción encendida, la descripción suma el permiso. Un `422` en `cloudflare_token` ya se muestra en el campo.
- **El panel:** con `panelCertificate` y la opción encendida, una línea "Panel: Cloudflare Origin CA · vence {date}", o "Panel: Let's Encrypt (DNS only en Cloudflare)", o el aviso de `failed` (o de una renovación que falló) con `originCa.error`. El aviso del panel no lleva botón de Reintentar: el panel no tiene `retry`, y su mensaje dice que se reintenta solo (§9.2).
- La nota de la tarjeta: "El token, y con los certificados de Cloudflare también sus claves, quedan en el autosave de Caddy".

**Textos** (`messages/admin/{es,en}.json`):

| Clave | es | en |
|---|---|---|
| `admin_cf_origin_ca_label` | Certificados de Cloudflare (Origin CA) | Cloudflare certificates (Origin CA) |
| `admin_cf_origin_ca_help` | Los dominios en Cloudflare con el proxy activado (nube naranja), también el del panel, usan un certificado de Cloudflare Origin CA en vez de Let's Encrypt. Solo Cloudflare confía en ese certificado, los navegadores no: sirve para el modo SSL Full (strict). Los dominios en DNS only y los que están fuera de Cloudflare siguen con Let's Encrypt. | Cloudflare domains with the proxy on (orange cloud), the panel's included, use a Cloudflare Origin CA certificate instead of Let's Encrypt. Only Cloudflare trusts that certificate, browsers don't: it's for the Full (strict) SSL mode. DNS-only domains and domains outside Cloudflare keep Let's Encrypt. |
| `admin_cf_origin_ca_permission` | El token necesita además el permiso Zone → SSL and Certificates → Edit en sus zonas. | The token also needs the Zone → SSL and Certificates → Edit permission on its zones. |
| `admin_cf_origin_ca_needs_token` | Configura el token de Cloudflare para activarlos. | Set the Cloudflare token to turn them on. |
| `admin_cf_origin_ca_enabled` | Certificados de Cloudflare activados: muelle los emite en unos minutos | Cloudflare certificates on: muelle issues them in a few minutes |
| `admin_cf_origin_ca_disabled` | Certificados de Cloudflare desactivados: los dominios vuelven a Let's Encrypt | Cloudflare certificates off: domains go back to Let's Encrypt |
| `admin_cf_origin_ca_off_title` | ¿Volver a Let's Encrypt? | Go back to Let's Encrypt? |
| `admin_cf_origin_ca_off_body` | Caddy pide un certificado de Let's Encrypt para cada dominio, normalmente en menos de un minuto. Mientras tanto, Cloudflare puede responder con error 525. muelle revoca después los certificados de Cloudflare. | Caddy requests a Let's Encrypt certificate for each domain, usually in under a minute. Meanwhile Cloudflare may answer with a 525 error. muelle revokes the Cloudflare certificates afterwards. |
| `admin_cf_origin_ca_off_confirm` | Desactivar | Turn off |
| `admin_cf_origin_ca_unavailable` | Cloudflare no respondió: no se cambió nada. Vuelve a intentarlo en un momento. | Cloudflare didn't respond: nothing changed. Try again in a moment. |
| `admin_cf_origin_ca_panel` | Panel: {source} | Panel: {source} |
| `admin_cf_remove_body_origin_ca` | También se desactivan los certificados de Cloudflare: los dominios vuelven a Let's Encrypt. | Cloudflare certificates are turned off too: domains go back to Let's Encrypt. |
| `admin_cf_modal_origin_ca` | Con los certificados de Cloudflare activados, también Zone → SSL and Certificates → Edit. | With Cloudflare certificates on, also Zone → SSL and Certificates → Edit. |
| `admin_cf_note` (cambia) | El token, y con los certificados de Cloudflare también sus claves, quedan en el autosave de Caddy. | The token, and with Cloudflare certificates their keys too, are kept in Caddy's autosave. |

### 11.2 Dominios

**Datos:**
- `domainDto.ts` suma, obligatorios (§11.1):
  - `cert_source: z.enum(CERT_SOURCES)`;
  - `origin_ca: originCaDtoSchema.nullable()`, con `originCaDtoSchema = z.object({status: z.enum(ORIGIN_CA_STATUSES), error_code: z.enum(ORIGIN_CA_ERROR_CODES).nullable().catch(null), error: z.string().nullable(), retry_at: timestamp.nullable(), expires_at: timestamp.nullable()})`. `error_code` es tolerante como `last_error_code`: un código que el panel no conoce se muestra como una falla cualquiera, con su `error`. Lo usa también `panel_certificate`.
- `domainStatus.ts` suma `CERT_SOURCES = ['letsencrypt', 'cloudflare_origin_ca']`, `ORIGIN_CA_STATUSES = ['active', 'issuing', 'failed', 'dns_only', 'not_eligible']` y `ORIGIN_CA_ERROR_CODES = ['permission', 'cloudflare', 'rejected']`.
- `AppDomain` suma `certSource: CertSource` y `originCa: OriginCaState | null` (null: la opción apagada, sin token o con el proxy deshabilitado).

**Funciones del dominio** (`appDomain.ts`):
- `originCaNotice(domain): 'issuing' | 'failed' | 'renew_failed' | 'dns_only' | 'dns_only_in_cloudflare' | null`. `dns_only_in_cloudflare` es `dns_only` con `proxied: true` en muelle: el proxy se apagó en el dashboard de Cloudflare;
- `hasProvisioning` también cuenta `originCa.status === 'issuing'`, así la lista se refresca seguido mientras emite.

**La vista** (`DomainCard.svelte`, `domainLabels.ts`):
- `certificateText`: con `certSource === 'cloudflare_origin_ca'`, "Cloudflare Origin CA · vence {date}" en vez de `{issuer}`, con la fecha de `originCa.expiresAt` (la que guardó muelle; `cert_expires_at` es la de la verificación, que puede ir hasta una hora atrasada después del cambio). Con `letsencrypt`, igual que hoy.
- Debajo, según `originCaNotice`:
  - `issuing` → meta, en tono neutro;
  - `failed` y `renew_failed` → pie en tono `warn`, con `originCa.error` y el botón Reintentar (el `retry` de siempre);
  - `dns_only` y `dns_only_in_cloudflare` → la pista.
  - No hay nada para `not_eligible`, ni con la opción apagada.
- Con `active`, un `title` o texto chico "Solo Cloudflare confía en este certificado".

**Textos** (`messages/ingress/{es,en}.json`):

| Clave | es | en |
|---|---|---|
| `ingress_cert_origin_ca` | Cloudflare Origin CA · vence {date} | Cloudflare Origin CA · expires {date} |
| `ingress_cert_origin_ca_expiring` | Cloudflare Origin CA · vence {date}, pronto | Cloudflare Origin CA · expires {date}, soon |
| `ingress_origin_ca_trust` | Solo Cloudflare confía en este certificado | Only Cloudflare trusts this certificate |
| `ingress_origin_ca_issuing` | Emitiendo el certificado de Cloudflare; mientras tanto, Let's Encrypt | Issuing the Cloudflare certificate; Let's Encrypt meanwhile |
| `ingress_origin_ca_failed` | No se pudo emitir el certificado de Cloudflare. Mientras tanto, Let's Encrypt. | The Cloudflare certificate couldn't be issued. Let's Encrypt meanwhile. |
| `ingress_origin_ca_renew_failed` | No se pudo renovar el certificado de Cloudflare | The Cloudflare certificate couldn't be renewed |
| `ingress_origin_ca_dns_only` | DNS only: usa Let's Encrypt. Activa Proxied para usar el certificado de Cloudflare. | DNS only: uses Let's Encrypt. Turn on Proxied to use the Cloudflare certificate. |
| `ingress_origin_ca_dns_only_cloudflare` | En Cloudflare el registro está en DNS only (nube gris): usa Let's Encrypt. | The record is DNS only in Cloudflare (grey cloud): uses Let's Encrypt. |

- `src/modules/ingress/README.md` suma los campos y los estados.
- `web/CLAUDE.md`, en "Lo que un módulo usa de otro", suma: Ajustes lee `panel_certificate` de `GET /proxy` (`proxyFollowQuery` del barrel de ingress).

### 11.3 MSW

- `db/admin.ts` guarda `cloudflare_origin_ca` (por defecto `false`).
- `handlers/admin.ts`:
  - encenderla sin token es `422`;
  - un `cloudflare_token: ""` que quita el token guardado, con la opción encendida y sin `false`, es `422`;
  - un token que contiene `sin-ssl` es `422` con el mensaje del permiso (en `cloudflare_origin_ca`, o en `cloudflare_token` si cambió el token);
  - un token que contiene `ssl-lento` es `502` al comprobar el permiso (el `caido` de hoy ya es `502` al guardar el token, así que no serviría para un token guardado);
  - los mensajes son los del contrato (§4.1), letra por letra, como los demás del mock.
- `db/ingress.ts` calcula `cert_source` y `origin_ca` desde `db/admin.ts` y cada dominio:
  - manejado + proxied → `active`, con `expires_at` a 90 días;
  - manejado sin proxy → `dns_only`;
  - manual → `not_eligible`;
  - un hostname que empieza con `falla-` → `failed`, con un `error` de ejemplo (`permission`).
  - `GET /proxy` suma `panel_certificate`.

---

## 12. Documentación a actualizar

- **README:**
  - La línea de "El token de Cloudflare" (hoy la 335) queda: "…necesita los permisos Zone:Read y DNS:Edit sobre las zonas de tus dominios y, si activas los certificados de Cloudflare, también Zone → SSL and Certificates → Edit en esas zonas. El usuario dueño del token tiene que tener API Access."
  - Un párrafo nuevo, "Certificados de Cloudflare (Origin CA)", que dice:
    - que es opcional y para Full (strict), y que muelle no cambia el modo SSL de la zona;
    - que solo Cloudflare confía en ellos;
    - qué dominios califican (con el proxy, y el panel también);
    - que los de DNS only y los de fuera de Cloudflare siguen con Let's Encrypt;
    - que son de 90 días y se renuevan solos;
    - que se revocan al dejar de usarse;
    - que las claves quedan cifradas en la base y en claro en el autosave de Caddy;
    - que con el panel en Origin CA hay que entrar por Cloudflare;
    - que al apagarlos puede haber un `525` mientras Caddy emite el de Let's Encrypt.
- **Diseño y contrato:** lo de §10.4 y §10.2, ya hecho (Paso 0a). Si una de las líneas encuentra algo que no se puede cumplir, el cambio se acuerda antes (contrato §1.3) y va a los tres documentos.
- **`shared/infra/out/crypto/crypto.go`:** la AAD nueva en la lista canónica.
- **El comentario del paquete `ingress` (`module.go`):** suma "los certificados de Cloudflare (Origin CA)".
- **`deploy/caddy/Dockerfile`:** sin cambios (`load_pem` es del núcleo). Opcional: una línea en el comentario que diga que la imagen no necesita nada más para Origin CA.

---

## 13. Tests

### 13.1 Unitarios (Go)

- **`shared/domain/entity`:**
  - `cloudflare_origin_ca` en `Encode`/`Decode` (un `""` es `false`, y lo que no es `"true"` también);
  - `ChangedKeys` la incluye;
  - `String` y `LogValue` no cambian.
- **La migración:** `migrations_origin_ca_test.go`:
  - las tablas;
  - los `CHECK` (`retired` sin `retired_at`, `failures > 0` sin código o sin `next_attempt_at`…);
  - el índice único parcial: dos `active` del mismo host fallan, y un `active` y un `retired` pasan;
  - la migración corre sobre una base con dominios.
- **`ingress/domain/service/origin_ca_test.go`** (tablas, puras):
  - candidato (cada condición de §4.1, el panel con un registro ajeno);
  - "se sirve" (los 14 días, otra zona, `observed` distinto de `proxied`);
  - `status` de §10.1 en su orden;
  - `NeedsCert` (sin certificado, menos de 30 días, otra zona);
  - `ClassifyOriginError` (cada fila de §9.1);
  - `OriginBackoff`;
  - los mensajes de §9.3, con las fechas, y la causa `permission` del panel, que no nombra Reintentar;
  - la plantilla según el `status`: una renovación que falló va con la de renovación mientras el actual se sirve, y con la de emisión (`failed`) cuando le quedan menos de 14 días.
- **`cftest`** (el Cloudflare falso, en `cftest/origin_ca.go`):
  - `POST /certificates` firma el CSR con una CA de prueba en memoria (`OriginRoot()` la expone para los tests);
  - comprueba el permiso por token y zona (`GrantSSL(token, zoneID)`); sin él responde **400 con `10000`**, o lo que diga el Paso 0, y `FailNext` imita `1010` o `403`;
  - valida el CSR (`1007`, `1011`), `requested_validity` (`1009`) y que el hostname caiga en una zona del token (`1010`);
  - `DELETE /certificates/{id}`: `revoked_at`; uno desconocido da `404`, y uno ya revocado `1014`;
  - `permissions` en las zonas de `GET /zones` (con `#ssl:edit` según `GrantSSL`), y un modo sin el campo (`OmitZonePermissions()`);
  - `Issued()` y `Revoked(id)` para las aserciones.
  - Con sus propios tests, en `server_test.go`.
- **El cliente de Cloudflare** (`cloudflare/client_test.go`, contra `cftest` y un `httptest` propio):
  - `CreateOriginCert` (el cuerpo exacto: `csr`, `hostnames`, `request_type`, `requested_validity`) y `RevokeOriginCert`;
  - `Zones` con `Permissions`;
  - 429 y 5xx como `ErrCloudflareUnavailable`, y 400 con `10000`/`1010`/`1011` como `*DNSError` con su código;
  - el token nunca aparece en un error.
- **`OriginKeys`:** el CSR se lee y su firma verifica (`CheckSignature`); es P-256 con CN y SAN; `Check` rechaza otra clave, otro hostname, uno vencido o uno que todavía no vale.
- **El repositorio SQLite:**
  - ida y vuelta con la clave cifrada;
  - la AAD está ligada al id: una clave copiada a otra fila no descifra;
  - `Insert` retira el anterior en la misma transacción;
  - `Retire` borra la clave;
  - `Active` y `RetiredDue(now, lastGoodBuild, grace)`.
- **`BuildProxyConfig`:** cada caso de §4.1 decide `OriginCA` en `Route` y en `PanelRoute`. `String`, `LogValue` y `MarshalJSON` de `ProxyConfig` y de `OriginPEM` muestran `***` en vez de la clave.
- **Los golden de Caddy** (`caddy/config_test.go`, `testdata/`):
  - `origin_ca.golden.json` (el de §7.2);
  - `origin_ca_all.golden.json`: todos con Origin CA, así que no va `automation` y van `certificates`, `disable_ocsp_stapling` y `skip_certificates`;
  - `origin_ca_internal.golden.json`: con `InternalCA`, lo que manda la integración;
  - los golden de hoy no cambian;
  - la verificación de que no quede ningún atajo del Caddyfile cubre también los nuevos;
  - un PEM con `{` da `ErrPlaceholderInSettings`;
  - `Admin.Load` oculta la clave (el PEM entero, y una línea base64 suelta) en un rechazo de `caddytest`.
- **`TLSProbe`** (`certs_test.go`, con un servidor TLS de `httptest`):
  - con `pinned` igual a la hoja, `Valid` sin raíces; también si es una de dos;
  - con `pinned` distinto, y una hoja que no verifica, es inválido;
  - un `pinned` vencido, o de otro hostname, es inválido;
  - una hoja que verifica contra `Roots` vale aunque `pinned` sea otro.
- **`CheckOriginCAPermission`** (`usecases/origin_ca_check_test.go`, contra `cftest`):
  - todas con `#ssl:edit`: ninguna llamada a `/certificates`;
  - una zona `pending` sin `#ssl:edit` no se revisa (ningún `POST`), y un token que solo ve zonas que no están activas pasa;
  - una sin el campo: un `POST` y un `DELETE`, y pasa;
  - una sin permiso: `422` que nombra la zona, en el `field` que se pidió;
  - tres o más sin permiso: el texto "y N más";
  - `502` con 5xx y con 429;
  - el `DELETE` de la prueba que falla deja un `retired`;
  - el plazo de 30 s (`cftest.Hang`).
- **`EnsureOriginCA`** (`usecases/origin_ca_test.go`, con SQLite real, el reloj de `package/clock`, `cftest` y `caddytest`):
  - apagada: no llama a Cloudflare;
  - encendida: emite para el manejado con proxy y no para el DNS only, el manual ni el ajeno;
  - DNS only visto en Cloudflare: no emite, y si había uno, lo retira, hace el Sync y lo revoca después del margen;
  - Cloudflare caído al observar: mantiene lo último visto y no retira;
  - renueva a los 30 días y revoca el viejo; a los 14 días sin renovar, deja de servirlo;
  - una falla registra el código, el mensaje y el backoff (1, 2, 4… min; 6 h);
  - un 429 corta la pasada;
  - **no revoca** si el Sync falló, ni si `retired_at >= LastGoodBuild`, ni antes del margen (carrera: un Sync que empezó antes del retiro y terminó después);
  - `retired_at` es la hora en que se escribe el retiro (DNS only y renovación): un Sync que empieza durante la pasada, antes del retiro, y sale bien, seguido de rechazos de Caddy, no deja revocarlo;
  - revocar con 404 o 1014 borra la fila;
  - sin token, conserva los retirados y los borra al vencer;
  - un dominio borrado se revoca;
  - otra zona emite uno nuevo y revoca el viejo;
  - el panel con un registro ajeno con proxy emite, y con DNS only no;
  - otro `panel_domain` revoca el viejo;
  - `ResetBackoff`.
- **`Syncer`:** `LastGoodBuild` es el momento de la lectura, no el del fin, y no avanza si falla el Load. `Retiring` espera al Sync en curso, y su hora no queda antes del comienzo de un Sync anterior con el reloj hacia atrás.
- **`CertChecker`:** en una renovación fija la nueva y la vieja hasta un Sync bueno que empezó después del retiro.
- **`ProxySettingsChanged`:** `cloudflare_origin_ca` hace Sync y Kick; el token con la opción encendida hace Kick; apagarla hace `Forget` (las filas de `origin_ca_hosts`, no los `retired`).
- **`UpdateDomain`:** cambiar `proxied` vuelve `observed` a NULL y hace Kick.
- **`RetryDomain`:** reinicia el backoff y hace Kick.
- **La salida de la API:**
  - `toOutput` y `GetProxyStatus` traen todas las claves siempre (`cert_source`, `origin_ca`, `panel_certificate`);
  - `origin_ca` es null con la opción apagada, sin token y con el proxy deshabilitado;
  - cada paso de §10.1 en su orden, `cert_source` igual a `cloudflare_origin_ca` solo con `active`, y `error_code`/`error`/`retry_at` en null fuera de `failed` y `active`;
  - en `ingress/infra/in/http`, los JSON exactos.
- **admin:**
  - `proxy_settings_test.go` (el `fakeHook` suma `CheckCloudflareOriginCA`), con la matriz de §3.2: sin token, quitar el token sin apagarla, encenderla y quitar el token a la vez (el mensaje de la regla 1), un estado "encendida sin token" guardado a mano que no frena un `PATCH` de otra cosa ni un `cloudflare_token: ""`, encenderla (el hook con `field` igual a `cloudflare_origin_ca`), cambiar el token con la opción encendida (`cloudflare_token`), un `false` que no llama al hook, un `502` que no guarda nada, y la auditoría con el bool;
  - `dto_test.go`: la clave siempre presente.
- **El contenedor** (`shared/infra/container`): `PATCH /settings` de punta a punta con `cftest`, para el `422`, el `200` y `GET /settings`.

### 13.2 Integración (`//go:build integration`, prefijo `it3`, puertos 21000–21999)

Va en `src/internal/ingress/integration_origin_ca_test.go`, con las ranuras 4 y 5 (las 1–3 ya están tomadas). La ranura 4 son los puertos 21401–21406, y la 5 son 21501–21506. Usa `caddy:2-alpine` y `alpine/socat:latest`, como los demás; si faltan, se salta.

1. **`TestIntegrationCaddyLoadPEM`** (`newITEnv(t, 4)`): Caddy real sirve un PEM cargado para un host y la CA interna para otro.
   - El test genera un certificado autofirmado ECDSA para `oca.it3<id>.test`, con `crypto/x509`.
   - Lo siembra como `active` en `origin_ca_certs`, junto con:
     - un dominio `oca` manejado, `proxied`, con `tls_mode: cloudflare`;
     - `origin_ca_hosts.observed = 'proxied'`;
     - un dominio `web` en `http`;
     - los ajustes, con el token y la opción encendida (la URL de un `cftest`).
   - `SyncRoutes` → `synced`: Caddy acepta `load_pem`, `skip_certificates` y `disable_ocsp_stapling`.
   - `tls.Dial` con SNI `oca` → la hoja es **idéntica** a la sembrada.
   - SNI `web` → emisor "Caddy Local Authority" (raíces con `internalRoots`).
   - Los dos hosts responden 200 por HTTPS al backend.
   - Se pasa `observed` a `dns_only` y se hace `SyncRoutes`. Al final, SNI `oca` lo sirve la CA interna: la vuelta a ACME funciona en Caddy real.
2. **`TestIntegrationOriginCAEndToEnd`** (`newITEnv(t, 5)`): el camino completo, con el Cloudflare falso.
   - Un token con `GrantSSL`; `POST …/domains` con `proxied: true` (`cftest` crea el A con proxy).
   - El reconciliador (`m.originCA.Execute`, o el job "proxy") emite.
   - Caddy sirve una hoja firmada por `cftest.OriginRoot()`.
   - `GET …/domains` dice `cert_source: cloudflare_origin_ca` y `origin_ca.status: active`.
   - El job "certificados" (con `pinned`) guarda `cert_issuer`.
   - `PATCH proxied: false` → eventualmente SNI con la CA interna, y `cftest.Revoked(id)` después del margen (`Timing.OriginRevokeGrace` corto en el test).
   - `PATCH /settings` no se usa: los ajustes se escriben con `e.settings`.

### 13.3 El panel

- **Unitarios (node):**
  - mappers: los campos nuevos con cada valor, `origin_ca: null` y `panel_certificate: null`; una respuesta sin `cert_source`, sin `origin_ca`, sin `panel_certificate` o sin `cloudflare_origin_ca` es un `ContractError`; un `error_code` desconocido cae en null;
  - `appDomain`: `originCaNotice` y `hasProvisioning`;
  - los casos de uso, con los fakes en memoria: `setCloudflareOriginCa` y `removeCloudflareToken`;
  - el mapeo de errores del repositorio.
- **Integración contra MSW** (`modules/admin/__tests__`, `modules/ingress/__tests__`):
  - encender sin token da `422` en `cloudflareOriginCa`;
  - `sin-ssl` da `422` con el mensaje;
  - `ssl-lento` da `CloudflareUnavailable`;
  - si sale bien, `GET /settings` la trae y `GET …/domains` trae `origin_ca`.
  - Más los tests de los handlers en `src/mocks/handlers/__tests__`.
- **ViewModels y componentes (Chromium):**
  - `ProxySettings.viewModel`:
    - deshabilitado sin token;
    - guardando;
    - el error 422 en el Banner, con el switch en lo guardado;
    - apagar pide confirmación;
    - éxito, con el toast y la invalidación de `ingressKeys.all`;
    - quitar el token con la opción encendida manda los dos campos.
  - `DomainCard`: el texto de cada `status`, el pie `warn` con Reintentar, y la pista de DNS only en sus dos variantes.
  - Los dos mockean el container, como hoy.
- **E2E (Playwright contra el build con MSW, bajo la CSP):**
  - `e2e/ajustes.e2e.ts`: encender, ver el toast y la línea del panel; apagar con confirmación; un token `sin-ssl` muestra el error.
  - `e2e/deployIngress.e2e.ts`: con la opción encendida, Dominios muestra "Cloudflare Origin CA · vence …" en el que tiene proxy y la pista en el DNS only.
- **`e2e-real`:** no se toca. El recorrido no usa Cloudflare, y no hay un Cloudflare falso del lado del binario. La integración de §13.2 cubre el backend. En la rama de la línea 2, hasta que entra la línea 1, falla (§14, "El punto de encuentro").

---

## 14. Plan de implementación

**Paso 0, antes de las dos líneas:**
- **0a. El texto del contrato** (§10.2) y del diseño (§10.4), en commits solo de docs, sobre `main` @ a1238d9: hecho en la rama `origin-ca`, junto con este documento. Desde ahí, las dos líneas corren en paralelo.
- **0b. La prueba manual contra Cloudflare** (el dueño, con una zona de prueba). No frena a nadie: ajusta fixtures.
  - Con dos tokens (uno con SSL and Certificates Edit y otro sin él), anotar:
    - qué trae `permissions` en `GET /zones` para cada uno;
    - qué responde `POST /certificates` (7 días, apex) al token sin el permiso: el status, el código y el mensaje;
    - si un `POST` con un CSR vacío responde `1007` a los dos.
  - El resultado ajusta `cftest`, las filas de `ClassifyOriginError` y si se usa la pista (§3.3).

### Línea 1: backend (ingress, admin, la migración y la documentación)

Cada paso es rojo → verde → commit, con `go test ./...` en verde.

1. `shared/domain/entity`: `SettingCloudflareOriginCA` en `ProxySettings` (Encode/Decode/ChangedKeys), con sus tests.
2. La migración `0006_origin_ca.sql` (§5.4), con el test del esquema. Sumar la AAD a `crypto.go`.
3. `ingress/domain/entity/origin_ca.go` (`OriginCert`, `OriginHost`, sus enums y su redacción) y `domain/service/origin_ca.go`: candidato, "se sirve", `status`, `NeedsCert`, `ClassifyOriginError`, `OriginBackoff` y los mensajes. Solo tablas de tests.
4. `cftest`: los endpoints de Origin CA, `permissions` de las zonas, `GrantSSL`, `OriginRoot`, `Issued` y `Revoked`, con sus tests.
5. El cliente de Cloudflare: `Zone.Permissions`, `CreateOriginCert` y `RevokeOriginCert` (puerto `service.OriginCA` en `ports.go`), con sus tests.
6. `OriginKeys` (`infra/out/service/origin_keys.go`): `Generate` y `Check`, con sus tests.
7. El repositorio SQLite `OriginCerts`/`OriginHosts` (puerto en `domain/repository/origin_ca.go`), con el cifrado y sus tests.
8. `ProxyConfig`: `OriginPEM` en `Route`/`PanelRoute` y `OriginInputs` en `BuildProxyConfig`, con la redacción y sus tests. Actualizar a los que llaman (`Syncer.build`), por ahora con entradas vacías.
9. `caddy.Render`: `load_pem`, `skip_certificates`, `disable_ocsp_stapling` y las políticas sin esos hosts, con los golden nuevos. Después, `Admin.Load` con la redacción de las claves.
10. `Syncer`: lee las entradas de Origin CA (repositorio y descifrado) y registra `LastGoodBuild`, con sus tests.
11. `CertProbe` con `pinned` en `TLSProbe` (con un `fakeProbe` nuevo) y `CertChecker`, que pasa `pinned`, con sus tests.
12. `CheckOriginCAPermission` y `Module.CheckCloudflareOriginCA`, con sus tests.
13. admin: `ProxyHook.CheckCloudflareOriginCA`, el DTO (GET y PATCH), las reglas de §3.2 en `PatchSettings`, la auditoría, el `fakeHook` y la matriz de tests.
14. `EnsureOriginCA` (el reconciliador, `Kick`/`Run`/`Execute`, `ResetBackoff`), con `Timing.OriginPass` y `OriginRevokeGrace` en `options.go` y los tests de §13.1.
15. El cableado: `module.go` (construcción y `Run`), `ProxyJob` (Kick al final), `ProxySettingsChanged` (Sync y Kick), los casos de uso de los dominios (Kick), `RetryDomain` (reiniciar el backoff), cada uno con su test.
16. La salida: `DomainOutput.cert_source` y `origin_ca`, y `GET /proxy` con `panel_certificate`. Tests de los casos de uso y de HTTP.
17. El test del contenedor (`shared/infra/container`): `PATCH /settings` y `GET /settings` de punta a punta.
18. La integración de §13.2 (las ranuras 4 y 5), corrida en local.
19. La documentación: README (§12), el comentario de `module.go` y lo que falte del diseño.
20. `go vet ./...`, `go test ./...`, `go test -tags integration -count=1 -run Integration ./src/internal/ingress/...`, y con `-tags panel` si el job `panel` lo pide.

### Línea 2: el panel (solo `web/`)

Cada paso es rojo → verde. `pnpm codegen` después de tocar `messages/`.

1. admin `data`: `adminDto` (`cloudflare_origin_ca`), el mapper (`originCa`, el payload) y sus tests.
2. admin `domain`: `CloudflareStatus.originCa`, `SettingsPatch.cloudflareOriginCa`, `OriginCaNeedsToken`, `setCloudflareOriginCa`, el cambio de `removeCloudflareToken` y los fakes, con sus tests.
3. admin `data/repositories`: el `fields` `cloudflare_origin_ca`, la mutation en `adminQueries.ts` y `container.ts`.
4. ingress `data`/`domain`: `domainDto` (`cert_source`, `origin_ca`), `proxyDto` (`panel_certificate`), los value objects, los mappers, `AppDomain`/`ProxyStatus`, `originCaNotice` y `hasProvisioning`, con sus tests. Exportar `PanelCertificate` desde el barrel.
5. MSW: `db/admin.ts`, `handlers/admin.ts`, `db/ingress.ts` y `handlers/ingress.ts` (§11.3), con los tests de los handlers.
6. Las integraciones de módulo contra MSW (admin e ingress).
7. Los textos `messages/admin` e `ingress` (es y en), y `pnpm codegen`.
8. `domainLabels.ts` y `DomainCard.svelte`, con su test de componente.
9. `ProxySettings.viewModel.svelte.ts` y `ProxySettings.svelte` (el switch, la confirmación al apagar, los errores, la línea del panel y la nota), con el test del viewModel.
10. Los E2E (`ajustes.e2e.ts`, `deployIngress.e2e.ts`).
11. `src/modules/ingress/README.md` y `web/CLAUDE.md`. Hasta este paso, los dos dicen algo que ya no vale: la sección "Diferencias con el contrato" del README tiene una nota (del Paso 0a) que dice que §3.7 falta en el panel, y `web/CLAUDE.md` ("Las fases 2, 3 y 5") dice que el README resume el contrato "sin diferencias". Este paso suma los campos y los estados al README, borra esa nota y deja `web/CLAUDE.md` como en §11.2. El Paso 0a no tocó nada más de `web/`.
12. `pnpm verify`, `pnpm build` y `pnpm test:e2e`.

**El punto de encuentro:**
- La línea 2 depende solo del texto del contrato (0a). Con el binario de la línea 1 y `pnpm dev`, se revisa a mano: el toggle, los dominios y la línea del panel.
- Los campos nuevos son obligatorios en los DTOs del panel (§11.1). Hasta que llega la línea 1, el panel se prueba contra MSW, que sigue el contrato letra por letra (§11.3).
- **La línea 2 entra a `main` después de la línea 1, o junto con ella; nunca antes.** Sin la línea 1, `GET /settings` no trae `cloudflare_origin_ca` y Ajustes da un `ContractError`. Al revés no pasa nada: zod descarta las claves que no conoce, así que la línea 1 sola no rompe el panel de hoy.
- Por lo mismo, el job `e2e-real` falla en la rama de la línea 2 hasta que la línea 1 está en su base: el recorrido abre `/ajustes#auditoria`, que lee `GET /settings` del backend de esa rama. No es un error de la línea 2.

---

## 15. Riesgos abiertos

| # | Riesgo | Mitigación |
|---|---|---|
| R1 | No hay una forma documentada de comprobar el permiso sin efectos secundarios. La pista `permissions` está deprecada, y la prueba definitiva emite y revoca un certificado de 7 días | Paso 0b. Si la pista no refleja el token, se usa siempre la prueba. Un falso positivo cae en una falla `permission` con Let's Encrypt (§9) |
| R2 | Los códigos de un token sin el permiso no están documentados (se vieron 400 con `10000` y `1010`) | `ClassifyOriginError` cubre los dos y el 401/403. Paso 0b |
| R3 | La vuelta a Let's Encrypt (opción apagada, DNS only, Origin CA por vencer) tiene una ventana mientras Caddy emite. Con Full (strict), Cloudflare responde `525`. Si la verificación de cada hora cae justo en esa ventana, un dominio `active` pasa a `error` + `certificate` hasta la próxima (o hasta un Reintentar) | DNS-01 suele tardar menos de un minuto. Si el de Let's Encrypt de antes sigue en `/data`, no hay ventana. El panel lo avisa al apagar. Mejora posible: Let's Encrypt "tibio" (§7.4) |
| R4 | Un cambio a DNS only hecho en el dashboard de Cloudflare se ve en a lo más 5 min. Mientras tanto, los navegadores ven el certificado de Origin CA | Es el intervalo del job "proxy". Se puede bajar solo para la observación, con costo en llamadas (§9.2) |
| R5 | Revocar uno que todavía se sirve corta el sitio | `LastGoodBuild` y el margen de 1 min (§6), con un test de la carrera. La integración verifica que la vuelta funciona antes de revocar |
| R6 | Las claves quedan en claro en el `autosave.json` (0600) y en `GET /config/` del socket de Caddy | Se acepta, como el token, que da más poder (editar el DNS). Una clave filtrada solo permite hacerse pasar por el origen ante Cloudflare. Los 90 días acotan la vida de la clave |
| R7 | El 429 bloquea el token 5 min, también el DNS de los dominios | El presupuesto de §9.2 y cortar la pasada con 429 |
| R8 | Caddy no renueva los certificados cargados, y la vuelta a Let's Encrypt a los 14 días también la hace muelle. Si muelle está caído más de 30 días dentro de la ventana de renovación, el certificado vence y Cloudflare corta el sitio | Se acepta: muelle es un servicio de Swarm que se reinicia solo |
| R9 | **NO VERIFICADO:** el texto de `cert_issuer` de un certificado de Origin CA, y si trae URL de OCSP | El panel usa `cert_source`. Se apaga el OCSP stapling de los certificados cargados |
| R10 | Las raíces de Origin CA vencen en 2029, y Cloudflare las va a rotar | 90 días siguen la rotación solos. La sonda fija la hoja, no la raíz |
| R11 | El usuario sin "API Access", o una zona `pending`, devuelve `1010` | Se clasifica como `permission`. El README y el mensaje lo nombran. `CheckCloudflareOriginCA` revisa solo las zonas activas (§3.3), así una `pending` no frena la opción. El reconciliador no mira el estado de la zona: un dominio de una zona `pending` puede quedar en `failed` con `permission` hasta que se active |
| R12 | El modo SSL de la zona no lo toca muelle. Con "Flexible" hay un bucle de redirects (pasa ya hoy) | Solo documentación: Origin CA es para Full o Full (strict) |
| R13 | `domains.proxied` puede no coincidir con Cloudflare (un cambio en el dashboard). muelle lo usa junto con lo observado, pero no lo corrige | La pista `dns_only_in_cloudflare` en Dominios. Mejora aparte: copiar el `proxied` observado al dominio |
| R14 | La integración usa `caddy:2-alpine` (la última 2.x), no el 2.11.7 fijo de muelle-caddy | Correr también §13.2 con la imagen `muelle-caddy` local si está (una variable del test) |
| R15 | La otra sesión cambiaba el contrato y los ajustes (`SettingsOutput`, `PatchSettingsInput`, §4, quizás una migración nueva) | **Resuelto** en `main` @ a1238d9: no sumó claves de ajustes ni migraciones, y las secciones del contrato siguen con los mismos números. Lo que sí cambió es que el panel ya no acepta campos opcionales "por si falta la fase" (§2, §11.1). La 0006 es de esta función y la 0007 del MCP (§5.4) |
| R16 | Con el panel en Origin CA, entrar sin pasar por Cloudflare da un error de certificado. La recuperación por `docker exec` sigue igual | La documentación y el texto del switch. Apagar el proxy del panel en Cloudflare: en a lo más 5 min muelle vuelve a Let's Encrypt |
| R17 | Al encender la opción con muchos dominios, la primera pasada hace unas 2 llamadas por host | Corre en segundo plano, con 5 min por pasada; lo que falte sigue en la próxima |
| R18 | El panel no tiene Reintentar, y no hay un reintento de todos a la vez. Después de arreglar en Cloudflare el permiso del mismo token, el panel espera hasta 6 h y cada dominio necesita su Reintentar (§9.2) | Mientras tanto sirve Let's Encrypt, que con Full (strict) también vale, y el mensaje del panel lo dice. Mejora posible, con un cambio de contrato (§1.3 del contrato): que mandar `cloudflare_origin_ca: true` con la opción encendida compruebe el permiso y reinicie el backoff de todos, con un Reintentar en Ajustes |
