import { adminDb } from './admin'
import { loadPersisted, savePersisted } from './persist'

// Dominios (Fase 3) del backend simulado, con el contrato de
// src/modules/ingress/README.md.
//
// Reglas, para probar desde el panel:
// - un hostname bajo `tudominio.cl` cae en una zona de Cloudflare: muelle crea el
//   registro y a los ~3 s el dominio pasa a `active` con su certificado;
//   - si la primera etiqueta contiene `existe` (`existe.tudominio.cl`), el registro ya
//     existía (con Proxied prendido): `error` con `record_exists` → Adoptar, que copia su
//     Proxied al dominio. Con `existe-tmp`, el registro ajeno desaparece a los 5 s y
//     "Volver a mirar" (retry) lo crea;
//   - si contiene `cf-error`, Cloudflare responde con error → Reintentar;
//   - si contiene `cf-caido`, Cloudflare no responde al borrar (`502`) y no se borra;
//   - si contiene `cf-intermitente`, Cloudflare falla al cambiar Proxied o al adoptar: se
//     responde 200 con el dominio en `error` + `cloudflare` (Reintentar lo arregla);
// - un hostname con `caddy-rechaza` en la primera etiqueta hace que Caddy no acepte la
//   configuración: `GET /proxy` dice `out_of_sync` (el aviso "proxy desincronizado");
// - cambiar los puertos en Ajustes recrea muelle-caddy: `GET /proxy` dice `recreating` unos
//   segundos (`recreateMs`) y después `synced`. El puerto 8081 está "ocupado" en el host:
//   el `PATCH` lo prueba antes de guardar y responde `422` en ese campo (contrato §3.5; sin
//   dominio del panel, con el proxy deshabilitado, no prueba nada). El
//   9443 pasa la prueba, pero otro lo toma antes de recrear: muelle-caddy no arranca con él,
//   y muelle vuelve a los puertos anteriores (`ports_rollback` en `GET /proxy`, los ajustes
//   de antes y `settings.ports_rollback` en la auditoría);
// - cualquier otro hostname está fuera de Cloudflare (`tls_mode: http`): queda `pending`
//   ("Esperando DNS") hasta que se toca Verificar (retry), que lo pasa a `active`;
// - lo de Cloudflare y las IP públicas salen de los ajustes del mock (`db/admin.ts`), como
//   en el backend: sin token, todo hostname queda fuera de Cloudflare; `records` se calcula
//   con las IP de ahora (`[]` sin `public_ip`); adoptar sin token o sin IP deja el dominio en
//   `error` + `cloudflare`; y sin dominio del panel, `GET /proxy` dice `disabled`;
// - los certificados de Cloudflare (Origin CA, contrato §3.7), con la opción encendida en
//   Ajustes (y el token, y el dominio del panel: si no, `origin_ca` es null): durante la
//   primera pasada (`provisionMs` desde que se encendió, o desde que se creó el dominio) un
//   dominio que califica dice `issuing`, y después `active`, con 90 días. El manejado sin
//   Proxied es `dns_only`, y uno que muelle no maneja o fuera de Cloudflare, `not_eligible`.
//   Por la primera etiqueta: con `gris-`, en Cloudflare está en DNS only (`dns_only` con
//   Proxied prendido en muelle); con `falla-`, la emisión falla por el permiso del token
//   (`failed`, se reintenta en 6 h o con Reintentar); con `renueva-`, falló la renovación y
//   el actual todavía sirve (`active` con el aviso). El panel sigue las mismas reglas con su
//   dominio (`panel_certificate` de `GET /proxy`), con su mensaje sin Reintentar.
// Como los deploys, un `pending` con `activeAt` se resuelve solo con la hora.

export const MOCK_ZONE = 'tudominio.cl'
const STORAGE_KEY = 'ingress'
const DAY = 24 * 60 * 60 * 1000
/** Lo que tarda en emitirse un certificado en el mock. */
const PROVISION_MS = 3000
/** Lo que tarda en recrearse muelle-caddy después de cambiar los puertos. */
const RECREATE_MS = 4000
/** Un puerto que algo ya usa en el host simulado: el `PATCH` de los puertos lo rechaza. */
export const MOCK_BUSY_PORT = 8081
/**
 * Un puerto que está libre cuando el `PATCH` lo prueba y que otro toma antes de recrear:
 * muelle-caddy no arranca con él y muelle vuelve a los anteriores (contrato §3.5).
 */
export const MOCK_RACED_PORT = 9443
const CADDY_IMAGE = 'ghcr.io/cpu-commits/muelle-caddy:v0.1.0'

export interface MockDomain {
	app: string
	hostname: string
	targetService: string
	targetPort: number
	proxied: boolean
	tlsMode: 'cloudflare' | 'http'
	zoneId: string | null
	recordId: string | null
	status: 'pending' | 'active' | 'error'
	lastError: string | null
	lastErrorCode: 'record_exists' | 'cloudflare' | 'certificate' | null
	certIssuer: string | null
	certExpiresAt: number | null
	/** Cuándo un `pending` pasa solo a `active` (null: hasta que alguien haga retry). */
	activeAt: number | null
	/** El registro que ya existía en Cloudflare (`record_exists`): su Proxied, y si se va. */
	foreignRecord?: { proxied: boolean; goneAt: number | null } | null
	/** El último Reintentar: reinicia la espera del certificado de Cloudflare que falló. */
	originCaRetriedAt?: number | null
	createdAt: number
	updatedAt?: number
}

interface IngressState {
	domains: MockDomain[]
	/** Apps ya sembradas (`proyecto/stage/app`). */
	seeded: string[]
	provisionMs: number
	recreateMs?: number
	/** Hasta cuándo `GET /proxy` dice `recreating` (un cambio de puertos). */
	recreatingUntil?: number | null
	/** Los puertos con que servía muelle-caddy antes de la recreación en curso. */
	recreatingFrom?: MockProxyPorts | null
	/** La última vuelta a los puertos anteriores (`ports_rollback`, contrato §3.3). */
	portsRollback?: MockPortsRollback | null
	/**
	 * Cuándo se encendieron los certificados de Cloudflare (Origin CA): desde ahí, la primera
	 * pasada tarda `provisionMs` en emitirlos. null o sin la clave: hace mucho.
	 */
	originCaSince?: number | null
}

export interface MockProxyPorts {
	http_port: number
	https_port: number
}

export interface MockPortsRollback {
	attempted: MockProxyPorts
	restored: MockProxyPorts
	error: string
	at: string
}

function seed(overrides: Partial<IngressState> = {}): IngressState {
	return {
		domains: [],
		seeded: [],
		provisionMs: PROVISION_MS,
		recreateMs: RECREATE_MS,
		recreatingUntil: null,
		recreatingFrom: null,
		portsRollback: null,
		...overrides,
	}
}

let state: IngressState = loadPersisted<IngressState>(STORAGE_KEY) ?? seed()

function commit(): void {
	savePersisted(STORAGE_KEY, state)
}

export function appKey(project: string, stage: string, app: string): string {
	return `${project}/${stage}/${app}`
}

/** Sin token, muelle no ve ninguna zona (contrato §3.1: `tls_mode: http`). */
function inZone(hostname: string): boolean {
	if (adminDb.state.cloudflareToken === null) return false
	return hostname === MOCK_ZONE || hostname.endsWith(`.${MOCK_ZONE}`)
}

/** El motivo del backend cuando hace falta el token y no está (`msgNoToken`). */
export const NO_TOKEN_REASON = 'agrega el token de Cloudflare en Ajustes'

/** Lo que falta para adoptar (el token y la IP pública), con el mensaje del backend. */
function adoptBlocker(): string | null {
	if (adminDb.state.cloudflareToken === null) {
		return NO_TOKEN_REASON
	}
	if (adminDb.state.publicIp === '') return 'configura la IP pública en Ajustes'
	return null
}

function firstLabel(hostname: string): string {
	return hostname.split('.')[0] ?? ''
}

function recordId(hostname: string): string {
	let hash = 0
	for (const char of hostname) hash = (Math.imul(hash, 31) + char.charCodeAt(0)) >>> 0
	return hash.toString(16).padStart(8, '0').repeat(4)
}

function issued(domain: MockDomain, now: number): MockDomain {
	return {
		...domain,
		status: 'active',
		lastError: null,
		lastErrorCode: null,
		certIssuer: "Let's Encrypt",
		certExpiresAt: now + 90 * DAY,
		activeAt: null,
	}
}

/** El dominio a la hora `now` (un `pending` con `activeAt` vencido ya es `active`). */
export function viewDomain(domain: MockDomain, now: number): MockDomain {
	if (domain.status === 'pending' && domain.activeAt !== null && domain.activeAt <= now) {
		return issued(domain, domain.activeAt)
	}
	return domain
}

/** Lo que pasaría al crear este hostname, según las reglas de arriba. */
function provision(
	base: Omit<
		MockDomain,
		| 'tlsMode'
		| 'zoneId'
		| 'recordId'
		| 'status'
		| 'lastError'
		| 'lastErrorCode'
		| 'certIssuer'
		| 'certExpiresAt'
		| 'activeAt'
	>,
	now: number,
	/** Para la siembra: forzar el caso "el registro ya existe" con cualquier hostname. */
	forceRecordExists = false,
): MockDomain {
	const empty = { certIssuer: null, certExpiresAt: null, lastError: null, lastErrorCode: null }
	if (!inZone(base.hostname)) {
		return {
			...base,
			...empty,
			proxied: false,
			tlsMode: 'http',
			zoneId: null,
			recordId: null,
			status: 'pending',
			activeAt: null,
		}
	}
	const zone = { tlsMode: 'cloudflare' as const, zoneId: `zona-${MOCK_ZONE}` }
	const label = firstLabel(base.hostname)
	if (forceRecordExists || label.includes('existe')) {
		return {
			...base,
			...empty,
			...zone,
			recordId: null,
			status: 'error',
			lastError: 'ya hay un registro con ese nombre que muelle no creó',
			lastErrorCode: 'record_exists',
			activeAt: null,
			foreignRecord: {
				proxied: true,
				goneAt: label.includes('existe-tmp') ? now + 5000 : null,
			},
		}
	}
	if (label.includes('cf-error')) {
		return {
			...base,
			...empty,
			...zone,
			recordId: null,
			status: 'error',
			lastError: 'Cloudflare respondió 403: el token no tiene permiso para editar la zona',
			lastErrorCode: 'cloudflare',
			activeAt: null,
		}
	}
	return {
		...base,
		...empty,
		...zone,
		recordId: recordId(base.hostname),
		status: 'pending',
		activeAt: now + state.provisionMs,
	}
}

function seedApp(project: string, stage: string, app: string, now: number): MockDomain[] {
	const owner = appKey(project, stage, app)
	const base = { app: owner, targetService: app, targetPort: 3000, createdAt: now - 30 * DAY }
	const active = issued(
		provision({ ...base, hostname: `${app}-${stage}.${MOCK_ZONE}`, proxied: true }, now),
		now - 20 * DAY,
	)
	return [
		{ ...active, certExpiresAt: Date.parse('2027-01-08T00:00:00Z') },
		// El del diseño: el registro de "webhooks-staging" ya existía en Cloudflare (con
		// Proxied prendido, que se copia al adoptar).
		provision(
			{ ...base, hostname: `webhooks-${stage}.${MOCK_ZONE}`, proxied: false },
			now,
			true,
		),
		provision({ ...base, hostname: `${app}.${stage}.otro-dominio.com`, proxied: false }, now),
	]
}

/** `origin_ca` de un dominio o del panel (contrato §3.7). */
export interface MockOriginCa {
	status: 'active' | 'issuing' | 'failed' | 'dns_only' | 'not_eligible'
	error_code: 'permission' | 'cloudflare' | 'rejected' | null
	error: string | null
	retry_at: string | null
	expires_at: string | null
}

const HOUR = 60 * 60 * 1000
const SHORT_MONTHS = [
	'ene',
	'feb',
	'mar',
	'abr',
	'may',
	'jun',
	'jul',
	'ago',
	'sep',
	'oct',
	'nov',
	'dic',
]

/** Como `shortDate` del backend, en UTC: "2 ene 2027". */
function shortDate(ms: number): string {
	const date = new Date(ms)
	return `${String(date.getUTCDate())} ${SHORT_MONTHS[date.getUTCMonth()] ?? ''} ${String(date.getUTCFullYear())}`
}

/** Las cinco claves, sin falla. */
function originCaState(
	status: MockOriginCa['status'],
	expiresAt: number | null = null,
): MockOriginCa {
	return {
		status,
		error_code: null,
		error: null,
		retry_at: null,
		expires_at: expiresAt === null ? null : new Date(expiresAt).toISOString(),
	}
}

/** Los certificados de Cloudflare están en uso: encendidos, con token y con el proxy. */
function originCaOn(): boolean {
	const settings = adminDb.state
	return (
		settings.cloudflareOriginCa &&
		settings.cloudflareToken !== null &&
		settings.panelDomain !== ''
	)
}

/**
 * Un hostname que califica (`active`, `issuing`, `failed`, o `dns_only` en Cloudflare), por
 * las reglas de arriba. `start` es desde cuándo corre su primera pasada; `retriedAt`, el
 * último Reintentar (el panel no tiene).
 */
function eligibleOriginCa(
	hostname: string,
	start: number,
	retriedAt: number | null,
	panel: boolean,
	now: number,
): MockOriginCa {
	const label = firstLabel(hostname)
	if (label.startsWith('gris-')) return originCaState('dns_only')
	const passEnd = start + state.provisionMs
	if (label.startsWith('falla-')) {
		const retried = retriedAt !== null && retriedAt >= start ? retriedAt : null
		if (retried === null && now < passEnd) return originCaState('issuing')
		// Justo después de un Reintentar, `retry_at` es la hora del pedido (en el pasado),
		// hasta que termina la pasada; después, la próxima espera de 6 h.
		const attempt = retried ?? start
		const retryAt =
			retried !== null && now <= retried + state.provisionMs
				? retried
				: attempt + state.provisionMs + 6 * HOUR
		const action = panel
			? 'agrégalo al token: muelle lo reintenta solo cada 6 h'
			: 'agrégalo al token y pulsa Reintentar'
		return {
			status: 'failed',
			error_code: 'permission',
			error: `muelle no pudo emitir el certificado de Cloudflare: el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona de ${hostname} (Cloudflare: Authentication error); ${action}; mientras tanto usa Let's Encrypt`,
			retry_at: new Date(retryAt).toISOString(),
			expires_at: null,
		}
	}
	if (now < passEnd) return originCaState('issuing')
	if (label.startsWith('renueva-')) {
		// Le quedan 20 días: se renueva a los 30 y deja de servirse a los 14.
		const expiresAt = now + 20 * DAY
		return {
			status: 'active',
			error_code: 'cloudflare',
			error: `muelle no pudo renovar el certificado de Cloudflare, que vence el ${shortDate(expiresAt)}: Cloudflare no respondió (HTTP 503); se reintenta solo; si no lo renueva antes del ${shortDate(expiresAt - 14 * DAY)}, vuelve a Let's Encrypt`,
			retry_at: new Date(now + 10 * 60_000).toISOString(),
			expires_at: new Date(expiresAt).toISOString(),
		}
	}
	return originCaState('active', passEnd + 90 * DAY)
}

/** `cert_source` y `origin_ca` (contrato §3.1): `cloudflare_origin_ca` solo con `active`. */
export function certFields(originCa: MockOriginCa | null) {
	return {
		cert_source: originCa?.status === 'active' ? 'cloudflare_origin_ca' : 'letsencrypt',
		origin_ca: originCa,
	}
}

export interface NewMockDomain {
	hostname: string
	targetService: string
	targetPort: number
	proxied: boolean
}

/** Lo que dice `GET /proxy` ahora, sin `ports_rollback`. */
function proxyState(now: number) {
	const settings = adminDb.state
	if (settings.panelDomain === '') {
		return {
			status: 'disabled' as const,
			error: null,
			synced_at: null,
			caddy: {
				image: null,
				running: false,
				http_port: settings.proxyHttpPort,
				https_port: settings.proxyHttpsPort,
			},
		}
	}
	// Sin contenedor (se está recreando): nada corre, y los puertos son los de los ajustes
	// (contrato §3.3).
	const noContainer = {
		image: null,
		running: false,
		http_port: settings.proxyHttpPort,
		https_port: settings.proxyHttpsPort,
	}
	if (state.recreatingUntil != null && now < state.recreatingUntil) {
		return {
			status: 'recreating' as const,
			error: null,
			synced_at: new Date(now - 60_000).toISOString(),
			caddy: noContainer,
		}
	}
	const caddy = {
		image: CADDY_IMAGE,
		running: true,
		http_port: settings.proxyHttpPort,
		https_port: settings.proxyHttpsPort,
	}
	const rejected = state.domains.find((domain) =>
		firstLabel(domain.hostname).includes('caddy-rechaza'),
	)
	if (rejected) {
		return {
			status: 'out_of_sync' as const,
			error: `Caddy rechazó la configuración: la ruta de ${rejected.hostname} no es válida`,
			synced_at: new Date(now - 5 * 60_000).toISOString(),
			caddy,
		}
	}
	return {
		status: 'synced' as const,
		error: null,
		synced_at: new Date(now - 60_000).toISOString(),
		caddy,
	}
}

export const ingressDb = {
	get state(): IngressState {
		return state
	},

	/** Vuelve al estado inicial; los tests lo llaman en `beforeEach`. */
	reset(overrides: Partial<IngressState> = {}): void {
		state = seed(overrides)
		commit()
	},

	commit,

	/** Los dominios de la app (sembrados la primera vez), resueltos a la hora `now`. */
	list(project: string, stage: string, app: string, now = Date.now()): MockDomain[] {
		const key = appKey(project, stage, app)
		if (!state.seeded.includes(key)) {
			state.seeded = [...state.seeded, key]
			state.domains = [...state.domains, ...seedApp(project, stage, app, now)]
			commit()
		}
		return state.domains
			.filter((domain) => domain.app === key)
			.map((domain) => viewDomain(domain, now))
			.sort((a, b) => a.hostname.localeCompare(b.hostname))
	},

	/** Una app recién creada no tiene dominios: no le toca la siembra del diseño. */
	startEmpty(project: string, stage: string, app: string): void {
		const key = appKey(project, stage, app)
		if (state.seeded.includes(key)) return
		state.seeded = [...state.seeded, key]
		commit()
	},

	/** Busca en todo muelle (el hostname es único global). */
	findAnywhere(hostname: string): MockDomain | undefined {
		return state.domains.find((domain) => domain.hostname === hostname)
	},

	find(key: string, hostname: string, now = Date.now()): MockDomain | undefined {
		const domain = state.domains.find((item) => item.app === key && item.hostname === hostname)
		return domain ? viewDomain(domain, now) : undefined
	},

	create(key: string, input: NewMockDomain, now = Date.now()): MockDomain {
		const domain = provision({ app: key, createdAt: now, ...input }, now)
		state.domains = [...state.domains, domain]
		commit()
		return domain
	},

	save(domain: MockDomain, now = Date.now()): MockDomain {
		const saved = { ...domain, updatedAt: now }
		state.domains = state.domains.map((item) =>
			item.app === saved.app && item.hostname === saved.hostname ? saved : item,
		)
		commit()
		return saved
	},

	remove(key: string, hostname: string): void {
		state.domains = state.domains.filter(
			(domain) => !(domain.app === key && domain.hostname === hostname),
		)
		commit()
	},

	/**
	 * Adoptar: muelle apunta el registro a su IP y emite el certificado. Conserva el
	 * Proxied del registro y lo copia al dominio (adoptar no cambia hacia dónde va el tráfico).
	 */
	adopt(domain: MockDomain, now = Date.now()): MockDomain {
		const blocker = adoptBlocker()
		if (blocker) {
			return this.save(
				{
					...domain,
					status: 'error',
					lastError: blocker,
					lastErrorCode: 'cloudflare',
					activeAt: null,
				},
				now,
			)
		}
		return this.save(
			{
				...domain,
				proxied: domain.foreignRecord?.proxied ?? domain.proxied,
				foreignRecord: null,
				recordId: recordId(domain.hostname),
				status: 'pending',
				lastError: null,
				lastErrorCode: null,
				activeAt: now + state.provisionMs,
			},
			now,
		)
	},

	/** Reintentar / Verificar. Con `record_exists`, vuelve a mirar: si ya no está, lo crea. */
	retry(found: MockDomain, now = Date.now()): MockDomain {
		// Con los certificados de Cloudflare, también reinicia la espera del suyo (§3.2).
		const domain = { ...found, originCaRetriedAt: now }
		if (domain.tlsMode === 'http') return this.save(issued(domain, now), now)
		if (domain.lastErrorCode === 'record_exists') {
			const goneAt = domain.foreignRecord?.goneAt ?? null
			if (goneAt === null || goneAt > now) return this.save(domain, now)
		}
		return this.save(
			{
				...domain,
				recordId: domain.recordId ?? recordId(domain.hostname),
				status: 'pending',
				lastError: null,
				lastErrorCode: null,
				foreignRecord: null,
				activeAt: now + state.provisionMs,
			},
			now,
		)
	},

	/**
	 * Un `PATCH /settings` cambió los puertos: muelle-caddy se recrea (contrato §3.5).
	 * `previous` son los puertos de antes del `PATCH`; si ya se estaba recreando, los dos
	 * cambios se juntan y vale el de antes del primero (con esos servía). Es un intento
	 * nuevo: limpia `ports_rollback`.
	 */
	recreateProxy(previous: MockProxyPorts, now = Date.now()): void {
		this.settle(now)
		const recreating = state.recreatingUntil != null && now < state.recreatingUntil
		state.recreatingFrom = recreating ? (state.recreatingFrom ?? previous) : previous
		state.recreatingUntil = now + (state.recreateMs ?? RECREATE_MS)
		state.portsRollback = null
		commit()
	},

	/**
	 * Termina la recreación si ya pasó su tiempo. Con `MOCK_RACED_PORT`, muelle-caddy no
	 * arranca: muelle vuelve a los puertos con que servía, si los guardados siguen siendo los
	 * que fallaron, y lo deja en `ports_rollback` y en la auditoría (actor `system`).
	 */
	settle(now = Date.now()): void {
		if (state.recreatingUntil == null || now < state.recreatingUntil) return
		const from = state.recreatingFrom ?? null
		state.recreatingUntil = null
		state.recreatingFrom = null
		const settings = adminDb.state
		const attempted = { http_port: settings.proxyHttpPort, https_port: settings.proxyHttpsPort }
		const raced =
			attempted.http_port === MOCK_RACED_PORT || attempted.https_port === MOCK_RACED_PORT
		if (from && raced && settings.panelDomain !== '') {
			settings.proxyHttpPort = from.http_port
			settings.proxyHttpsPort = from.https_port
			adminDb.commit()
			const error = `los puertos ${String(attempted.http_port)} o ${String(attempted.https_port)} del host ya están en uso, y muelle-caddy no pudo arrancar con ellos`
			adminDb.record(
				'settings.ports_rollback',
				'settings',
				{
					proxy_http_port: from.http_port,
					proxy_https_port: from.https_port,
					attempted: {
						proxy_http_port: attempted.http_port,
						proxy_https_port: attempted.https_port,
					},
					error,
				},
				'system',
			)
			state.portsRollback = {
				attempted,
				restored: from,
				error,
				at: new Date(now).toISOString(),
			}
		}
		commit()
	},

	/**
	 * `GET /proxy`: sincronizado, salvo que algún dominio tenga `caddy-rechaza`. Sin dominio
	 * del panel, `disabled`: no hay contenedor (`image: null`) y no se consulta Docker.
	 * `ports_rollback` viene siempre (contrato §3.3).
	 */
	proxy(now = Date.now()) {
		this.settle(now)
		return {
			...proxyState(now),
			ports_rollback: state.portsRollback ?? null,
			panel_certificate: this.panelCertificate(now),
		}
	},

	/** Un `PATCH /settings` encendió los certificados de Cloudflare: empieza la primera pasada. */
	originCaTurnedOn(now = Date.now()): void {
		state.originCaSince = now
		commit()
	},

	/** El certificado de Cloudflare de un dominio (contrato §3.7), o null si no se usan. */
	originCa(domain: MockDomain, now = Date.now()): MockOriginCa | null {
		if (!originCaOn()) return null
		if (domain.tlsMode !== 'cloudflare' || domain.zoneId === null || domain.recordId === null) {
			return originCaState('not_eligible')
		}
		if (!domain.proxied) return originCaState('dns_only')
		const start = Math.max(state.originCaSince ?? 0, domain.createdAt)
		return eligibleOriginCa(
			domain.hostname,
			start,
			domain.originCaRetriedAt ?? null,
			false,
			now,
		)
	},

	/** El del panel (`panel_certificate`, contrato §3.3); null sin dominio del panel. */
	panelCertificate(now = Date.now()) {
		const hostname = adminDb.state.panelDomain
		if (hostname === '') return null
		const originCa = !originCaOn()
			? null
			: inZone(hostname)
				? eligibleOriginCa(hostname, state.originCaSince ?? 0, null, true, now)
				: originCaState('not_eligible')
		return { hostname, ...certFields(originCa) }
	},

	/** Cloudflare falla al escribir (`cf-intermitente`): el dominio queda en error. */
	cloudflareFlaky(domain: MockDomain): boolean {
		return firstLabel(domain.hostname).includes('cf-intermitente')
	},

	/**
	 * Cloudflare actualizó los registros (un `PATCH` de Proxied que salió bien): un dominio
	 * que estaba en `error` + `cloudflare` vuelve a `pending` (contrato §3.2).
	 */
	recoverCloudflare(domain: MockDomain, now = Date.now()): MockDomain {
		if (domain.status !== 'error' || domain.lastErrorCode !== 'cloudflare') {
			return this.save(domain, now)
		}
		return this.save(
			{
				...domain,
				status: 'pending',
				lastError: null,
				lastErrorCode: null,
				activeAt: now + state.provisionMs,
			},
			now,
		)
	},

	/** Cloudflare falló (o no hay token, con su mensaje): `error` + `cloudflare`. */
	failCloudflare(
		domain: MockDomain,
		reason = 'Cloudflare respondió 500 al actualizar el registro',
		now = Date.now(),
	): MockDomain {
		return this.save(
			{
				...domain,
				status: 'error',
				lastError: reason,
				lastErrorCode: 'cloudflare',
				activeAt: null,
			},
			now,
		)
	},

	/** Cloudflare no responde al borrar el registro (`cf-caido`): `502` y no se borra. */
	cloudflareDown(domain: MockDomain): boolean {
		return domain.recordId !== null && firstLabel(domain.hostname).includes('cf-caido')
	},
}
