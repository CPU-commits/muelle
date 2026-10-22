import { authDb } from './auth'
import { loadPersisted, savePersisted } from './persist'

// Tokens de API del backend simulado, y los ids de los usuarios para la auditoría. Los
// usuarios y las invitaciones NO viven acá: son de `authDb` (db/auth.ts), que también usa
// el login.

export interface MockToken {
	id: number
	name: string
	/** `''` solo con `allProjects`. */
	project: string
	/** `''`: todo el proyecto (o todos los proyectos). */
	stage: string
	readOnly: boolean
	/** Todos los proyectos: sin proyecto ni stage, y siempre `readOnly` (contrato §8.1). */
	allProjects: boolean
	/** El username de quien lo creó, o null si ese usuario se borró. */
	createdBy: string | null
	createdAt: string
	lastUsedAt: string | null
	expiresAt: string | null
}

export interface IdentityState {
	tokens: MockToken[]
	nextTokenId: number
	/** El id de cada usuario: el `actor_id` de la auditoría (el nombre va copiado aparte). */
	userIds: Record<string, number>
	nextUserId: number
}

const STORAGE_KEY = 'identity'
const MINUTE_MS = 60_000
const DAY_MS = 24 * 60 * MINUTE_MS

function ago(ms: number): string {
	return new Date(Date.now() - ms).toISOString()
}

function seed(): IdentityState {
	return {
		tokens: [
			{
				id: 1,
				name: 'github-actions',
				project: 'auditfy',
				stage: '',
				readOnly: false,
				allProjects: false,
				createdBy: 'felipe',
				createdAt: ago(20 * DAY_MS),
				lastUsedAt: ago(12 * MINUTE_MS),
				expiresAt: null,
			},
			{
				id: 2,
				name: 'extractor-ci',
				project: 'extractor',
				stage: 'staging',
				readOnly: false,
				allProjects: false,
				createdBy: 'felipe',
				createdAt: ago(12 * DAY_MS),
				lastUsedAt: ago(40 * MINUTE_MS),
				expiresAt: '2026-12-31T23:59:59.000Z',
			},
			{
				id: 3,
				name: 'extractor-prod-ci',
				project: 'extractor',
				stage: 'production',
				readOnly: false,
				allProjects: false,
				createdBy: null,
				createdAt: ago(30 * DAY_MS),
				lastUsedAt: ago(6 * DAY_MS),
				expiresAt: '2026-12-31T23:59:59.000Z',
			},
			{
				id: 4,
				name: 'aumi-viejo',
				project: 'aumi-agents',
				stage: 'staging',
				readOnly: false,
				allProjects: false,
				createdBy: 'felipe',
				createdAt: ago(120 * DAY_MS),
				lastUsedAt: ago(95 * DAY_MS),
				expiresAt: ago(30 * DAY_MS),
			},
			// Para Claude Code (MCP): uno de solo lectura de un stage y uno de todos los
			// proyectos, que siempre es de solo lectura y el panel propone a 90 días.
			{
				id: 5,
				name: 'claude-auditfy',
				project: 'auditfy',
				stage: 'staging',
				readOnly: true,
				allProjects: false,
				createdBy: 'felipe',
				createdAt: ago(3 * DAY_MS),
				lastUsedAt: ago(2 * 60 * MINUTE_MS),
				expiresAt: null,
			},
			{
				id: 6,
				name: 'claude-debug',
				project: '',
				stage: '',
				readOnly: true,
				allProjects: true,
				createdBy: 'felipe',
				createdAt: ago(DAY_MS),
				lastUsedAt: null,
				// En 89 días: se creó ayer con el vencimiento que propone el panel.
				expiresAt: ago(-89 * DAY_MS),
			},
		],
		nextTokenId: 7,
		userIds: { felipe: 1 },
		nextUserId: 2,
	}
}

let state: IdentityState = loadPersisted<IdentityState>(STORAGE_KEY) ?? seed()

export const identityDb = {
	get state(): IdentityState {
		return state
	},

	/** Vuelve al estado inicial; los tests lo llaman en `beforeEach` (con `authDb.reset()`). */
	reset(overrides: Partial<IdentityState> = {}): void {
		state = { ...seed(), ...overrides }
		savePersisted(STORAGE_KEY, state)
	},

	commit(): void {
		savePersisted(STORAGE_KEY, state)
	},

	/** El id de un usuario de `authDb`; se asigna la primera vez que hace falta. */
	userId(username: string): number {
		const known = state.userIds[username]
		if (known !== undefined) return known
		const id = state.nextUserId
		state.userIds = { ...state.userIds, [username]: id }
		state.nextUserId += 1
		this.commit()
		return id
	},

	/** Borrar un usuario no revoca sus tokens: quedan con `created_by` null. */
	forgetCreator(username: string): void {
		state.tokens = state.tokens.map((token) =>
			token.createdBy === username ? { ...token, createdBy: null } : token,
		)
		this.commit()
	},

	sessionUsername(): string | null {
		return authDb.currentUser()?.username ?? null
	},
}
