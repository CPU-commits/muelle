import { authDb } from './auth'
import { identityDb } from './identity'
import { loadPersisted, savePersisted } from './persist'

// Settings, auditoría y registries del backend simulado. `require_2fa` NO vive acá: es
// `authDb.state.require2fa`, el mismo que lee el login para pedir la 2FA. Lo de la Fase 3
// (proxy, DNS, certificados, IPs permitidas) y la Fase 2 (registries) sigue el spec §5.1.

export interface MockRegistry {
	host: string
	username: string
	password: string
}

export interface MockAuditEvent {
	id: number
	actor_type: 'user' | 'token' | 'system'
	actor_id: number | null
	action: string
	target: string
	details: Record<string, unknown>
	at: string
}

export interface AdminState {
	cloudflareToken: string | null
	cloudflareZones: number
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

const STORAGE_KEY = 'admin'
const MINUTE_MS = 60_000

/** Cuántos eventos trae la auditoría al empezar: más de una página, para ver "Cargar más". */
export const MOCK_AUDIT_SEED_COUNT = 40

type SeedAction = Pick<MockAuditEvent, 'actor_type' | 'action' | 'target'>

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

function seedAudit(): MockAuditEvent[] {
	const now = Date.now()
	return Array.from({ length: MOCK_AUDIT_SEED_COUNT }, (_, index) => {
		const base = SEED_ACTIONS[index % SEED_ACTIONS.length] ?? LOGIN
		const minutesAgo = (MOCK_AUDIT_SEED_COUNT - index) * 37
		return {
			id: index + 1,
			...base,
			// El usuario felipe (id 1) o el token github-actions (id 1).
			actor_id: 1,
			details: base === LOGIN ? { ip: '198.51.100.17', mfa: false } : {},
			at: new Date(now - minutesAgo * MINUTE_MS).toISOString(),
		}
	})
}

function seed(): AdminState {
	return {
		cloudflareToken: 'cf-token-demo',
		cloudflareZones: 4,
		publicIp: '203.0.113.24',
		publicIpv6: '2001:db8::24',
		acmeEmail: 'ops@tudominio.cl',
		panelDomain: 'panel.tudominio.cl',
		proxyHttpPort: 80,
		proxyHttpsPort: 443,
		panelAllowedCidrs: [],
		clientIp: '198.51.100.17',
		registries: [{ host: 'ghcr.io', username: 'cpu-commits', password: 'ghp_demo' }],
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

	/** Lo que hace `audit.Recorder.Record`: el actor sale de la sesión. Nunca con secretos. */
	record(action: string, target: string, details: Record<string, unknown> = {}): void {
		const user = authDb.currentUser()
		state.audit = [
			...state.audit,
			{
				id: state.nextAuditId,
				actor_type: user ? 'user' : 'system',
				actor_id: user ? identityDb.userId(user.username) : null,
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
