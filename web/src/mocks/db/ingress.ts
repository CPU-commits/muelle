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
// - cualquier otro hostname está fuera de Cloudflare (`tls_mode: http`): queda `pending`
//   ("Esperando DNS") hasta que se toca Verificar (retry), que lo pasa a `active`.
// Como los deploys, un `pending` con `activeAt` se resuelve solo con la hora.

export const MOCK_ZONE = 'tudominio.cl'
export const MOCK_PUBLIC_IP = '203.0.113.24'
/** La IPv6 pública del nodo: con ella, el dominio lleva también un AAAA. */
export const MOCK_PUBLIC_IPV6 = '2001:db8::24'

const STORAGE_KEY = 'ingress'
const DAY = 24 * 60 * 60 * 1000
/** Lo que tarda en emitirse un certificado en el mock. */
const PROVISION_MS = 3000

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
	createdAt: number
	updatedAt?: number
}

interface IngressState {
	domains: MockDomain[]
	/** Apps ya sembradas (`proyecto/stage/app`). */
	seeded: string[]
	provisionMs: number
}

function seed(overrides: Partial<IngressState> = {}): IngressState {
	return { domains: [], seeded: [], provisionMs: PROVISION_MS, ...overrides }
}

let state: IngressState = loadPersisted<IngressState>(STORAGE_KEY) ?? seed()

function commit(): void {
	savePersisted(STORAGE_KEY, state)
}

export function appKey(project: string, stage: string, app: string): string {
	return `${project}/${stage}/${app}`
}

function inZone(hostname: string): boolean {
	return hostname === MOCK_ZONE || hostname.endsWith(`.${MOCK_ZONE}`)
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

export interface NewMockDomain {
	hostname: string
	targetService: string
	targetPort: number
	proxied: boolean
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

	publicIp: MOCK_PUBLIC_IP,

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
	retry(domain: MockDomain, now = Date.now()): MockDomain {
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

	/** `GET /proxy`: sincronizado, salvo que algún dominio tenga `caddy-rechaza`. */
	proxy(now = Date.now()) {
		const caddy = {
			image: 'ghcr.io/cpu-commits/muelle-caddy:v0.1.0',
			running: true,
			http_port: 80,
			https_port: 443,
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
	},

	/** Cloudflare falla al escribir (`cf-intermitente`): el dominio queda en error. */
	cloudflareFlaky(domain: MockDomain): boolean {
		return firstLabel(domain.hostname).includes('cf-intermitente')
	},

	failCloudflare(domain: MockDomain, now = Date.now()): MockDomain {
		return this.save(
			{
				...domain,
				status: 'error',
				lastError: 'Cloudflare respondió 500 al actualizar el registro',
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
