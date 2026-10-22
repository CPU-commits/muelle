import { authDb } from './auth'
import { identityDb } from './identity'
import { loadPersisted, savePersisted } from './persist'

// Settings, auditoría y registries del backend simulado. `require_2fa` NO vive acá: es
// `authDb.state.require2fa`, el mismo que lee el login para pedir la 2FA. Lo de las Fases 2
// y 3 sigue el contrato (docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md,
// §2.10 y §4).

export interface MockRegistry {
	host: string
	username: string
	password: string
	createdAt: string
	updatedAt: string
}

export interface MockAuditEvent {
	id: number
	actor_type: 'user' | 'token' | 'system'
	actor_id: number | null
	/**
	 * El username, el nombre del token o "system", copiado al escribir el evento (contrato
	 * §4.2). `''` solo en un evento de antes de la migración 0004 cuyo autor ya no estaba.
	 */
	actor_name: string
	action: string
	target: string
	details: Record<string, unknown>
	at: string
}

export interface AdminState {
	cloudflareToken: string | null
	/** Las zonas que veía el token al guardarlo (un valor guardado, no una consulta). */
	cloudflareZones: number | null
	/** Los certificados de Cloudflare (Origin CA, contrato §3.7): apagados por defecto. */
	cloudflareOriginCa: boolean
	publicIp: string
	publicIpv6: string
	acmeEmail: string
	panelDomain: string
	proxyHttpPort: number
	proxyHttpsPort: number
	panelAllowedCidrs: string[]
	/** La IP con la que el "backend" ve al navegador. */
	clientIp: string
	registries: MockRegistry[]
	/** En orden de id (el más viejo primero). */
	audit: MockAuditEvent[]
	nextAuditId: number
}

// Con versión: una pestaña con el estado de antes (registries sin fechas, la auditoría sin
// el primer arranque o sin `actor_name`, los ajustes sin `cloudflareOriginCa`) empieza de
// cero.
const STORAGE_KEY = 'admin-v5'
const MINUTE_MS = 60_000

/** Cuántos eventos trae la auditoría al empezar: más de una página, para ver "Cargar más". */
export const MOCK_AUDIT_SEED_COUNT = 40

type SeedAction = Pick<MockAuditEvent, 'actor_type' | 'action' | 'target'>

/** Los nombres de los actores del seed: el usuario felipe (id 1) y el token github-actions (id 1). */
const SEED_NAMES: Record<SeedAction['actor_type'], string> = {
	user: 'felipe',
	token: 'github-actions',
	system: 'system',
}

/**
 * Un evento de antes de la migración 0004 (contrato §6) cuyo autor, un token, ya se había
 * borrado: queda sin nombre, y el panel muestra el tipo y el id.
 */
const ORPHAN: Omit<MockAuditEvent, 'id' | 'at'> = {
	actor_type: 'token',
	actor_id: 9,
	actor_name: '',
	action: 'deploy.create',
	target: 'app:auditfy/staging/api',
	details: {},
}

/**
 * Un deploy que hizo Claude Code con la herramienta `deploy` del servidor MCP, con el token
 * de escritura de extractor/staging: `details` suma `"via": "mcp"` (contrato §8.6). Va entre
 * los más nuevos, en la primera página.
 */
const MCP_DEPLOY: Omit<MockAuditEvent, 'id' | 'at'> = {
	actor_type: 'token',
	actor_id: 2,
	actor_name: 'extractor-ci',
	action: 'deploy.create',
	target: 'app:extractor/staging/api',
	details: { deploy: 12, images: {}, trigger: 'api', via: 'mcp' },
}

const MCP_DEPLOY_INDEX = MOCK_AUDIT_SEED_COUNT - 2

const LOGIN: SeedAction = { actor_type: 'user', action: 'auth.login', target: 'user:felipe' }

const SEED_ACTIONS: readonly SeedAction[] = [
	LOGIN,
	{ actor_type: 'user', action: 'project.create', target: 'project:auditfy' },
	{ actor_type: 'user', action: 'stage.create', target: 'stage:auditfy/staging' },
	{ actor_type: 'user', action: 'app.create', target: 'app:auditfy/staging/api' },
	{ actor_type: 'user', action: 'env.patch', target: 'app:auditfy/staging/api' },
	{ actor_type: 'token', action: 'deploy.create', target: 'app:auditfy/staging/api' },
	{ actor_type: 'user', action: 'token.create', target: 'token:2' },
	{ actor_type: 'user', action: 'domain.create', target: 'webhooks-staging.tudominio.cl' },
]

/**
 * Lo primero que queda en la auditoría es el primer arranque de la base, sin usuario
 * (actor `system`): los ajustes de install (`settings.init`, solo las claves) y el registry
 * con que install bajó las imágenes de muelle (`registry.create`, `source: install`;
 * contrato §2.10 y §5.2).
 */
const INSTALL_ACTIONS: readonly (SeedAction & { details: Record<string, unknown> })[] = [
	{
		actor_type: 'system',
		action: 'settings.init',
		target: 'settings',
		details: { keys: ['acme_email', 'public_ip', 'proxy_http_port', 'proxy_https_port'] },
	},
	{
		actor_type: 'system',
		action: 'registry.create',
		target: 'registry:ghcr.io',
		details: { username: 'cpu-commits', source: 'install' },
	},
]

function seedAudit(): MockAuditEvent[] {
	const now = Date.now()
	return Array.from({ length: MOCK_AUDIT_SEED_COUNT }, (_, index) => {
		const install = INSTALL_ACTIONS[index]
		const minutesAgo = (MOCK_AUDIT_SEED_COUNT - index) * 37
		const at = new Date(now - minutesAgo * MINUTE_MS).toISOString()
		if (install) {
			return { id: index + 1, ...install, actor_id: null, actor_name: 'system', at }
		}
		if (index === INSTALL_ACTIONS.length) return { id: index + 1, ...ORPHAN, at }
		if (index === MCP_DEPLOY_INDEX) return { id: index + 1, ...MCP_DEPLOY, at }
		const base = SEED_ACTIONS[index % SEED_ACTIONS.length] ?? LOGIN
		return {
			id: index + 1,
			...base,
			// El usuario felipe (id 1) o el token github-actions (id 1).
			actor_id: 1,
			actor_name: SEED_NAMES[base.actor_type],
			details: base === LOGIN ? { ip: '198.51.100.17', mfa: false } : {},
			at,
		}
	})
}

function seed(): AdminState {
	return {
		cloudflareToken: 'cf_token_demo_de_cuarenta_caracteres_xx',
		cloudflareZones: 4,
		cloudflareOriginCa: false,
		publicIp: '203.0.113.24',
		publicIpv6: '2001:db8::24',
		acmeEmail: 'ops@tudominio.cl',
		panelDomain: 'panel.tudominio.cl',
		proxyHttpPort: 80,
		proxyHttpsPort: 443,
		panelAllowedCidrs: [],
		clientIp: '198.51.100.17',
		registries: [
			{
				host: 'ghcr.io',
				username: 'cpu-commits',
				password: 'ghp_demo',
				createdAt: '2026-10-01T12:00:00.000Z',
				updatedAt: '2026-10-09T15:30:00.000Z',
			},
		],
		audit: seedAudit(),
		nextAuditId: MOCK_AUDIT_SEED_COUNT + 1,
	}
}

let state: AdminState = loadPersisted<AdminState>(STORAGE_KEY) ?? seed()

export const adminDb = {
	get state(): AdminState {
		return state
	},

	/** Vuelve al estado inicial; los tests lo llaman en `beforeEach` (con `authDb.reset()`). */
	reset(overrides: Partial<AdminState> = {}): void {
		state = { ...seed(), ...overrides }
		savePersisted(STORAGE_KEY, state)
	},

	commit(): void {
		savePersisted(STORAGE_KEY, state)
	},

	/**
	 * Lo que hace `audit.Recorder.Record`: el actor sale de la sesión, o es `system` (lo que
	 * hace muelle solo, como volver a los puertos anteriores). Nunca con secretos.
	 */
	record(
		action: string,
		target: string,
		details: Record<string, unknown> = {},
		actor: 'session' | 'system' = 'session',
	): void {
		const user = actor === 'session' ? authDb.currentUser() : null
		state.audit = [
			...state.audit,
			{
				id: state.nextAuditId,
				actor_type: user ? 'user' : 'system',
				actor_id: user ? identityDb.userId(user.username) : null,
				actor_name: user ? user.username : 'system',
				action,
				target,
				details,
				at: new Date().toISOString(),
			},
		]
		state.nextAuditId += 1
		this.commit()
	},
}
