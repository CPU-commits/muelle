import { authDb } from './auth'
import { loadPersisted, savePersisted } from './persist'

// Tokens de API del backend simulado, y los ids de los usuarios para la auditoría. Los
// usuarios y las invitaciones NO viven acá: son de `authDb` (db/auth.ts), que también usa
// el login.

export interface MockToken {
	id: number
	name: string
	project: string
	/** `''`: todo el proyecto. */
	stage: string
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
				createdBy: 'felipe',
				createdAt: ago(120 * DAY_MS),
				lastUsedAt: ago(95 * DAY_MS),
				expiresAt: ago(30 * DAY_MS),
			},
		],
		nextTokenId: 5,
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
