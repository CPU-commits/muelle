import type {
	ApiToken,
	CreatedApiToken,
	Invite,
	NewApiToken,
	ProjectRef,
	ScopeRepository,
	StageRef,
	TokenId,
	TokenRepository,
	User,
	UserRepository,
} from '../..'
import { CannotDeleteSelf, TokenNotFound, TokenScopeNotFound, UserNotFound } from '../..'

const HOUR_MS = 60 * 60 * 1000

export interface FakeIdentitySeed {
	users?: User[]
	/** El usuario de la sesión: el backend no deja que se borre a sí mismo. */
	sessionUser?: string
	tokens?: ApiToken[]
	/** Proyecto → sus stages. */
	projects?: Record<string, string[]>
	now?: () => Date
}

export function fakeUser(overrides: Partial<User> = {}): User {
	return {
		username: 'felipe',
		totpEnabled: true,
		createdAt: new Date('2026-10-09T12:00:00.000Z'),
		...overrides,
	}
}

export function fakeToken(overrides: Partial<ApiToken> = {}): ApiToken {
	return {
		id: 1 as TokenId,
		name: 'github-actions',
		scope: { project: 'auditfy', stage: null },
		createdBy: { kind: 'user', username: 'felipe' },
		createdAt: new Date('2026-10-01T12:00:00.000Z'),
		lastUsedAt: null,
		expiresAt: null,
		...overrides,
	}
}

// Un backend de identity en memoria con las reglas que le importan al panel: nadie se
// borra a sí mismo, quitar 2FA cierra sesiones, los tokens se crean con alcance que existe
// y su valor en claro solo sale al crearlos. Los tres puertos comparten el estado: se
// piden con `asUsers()`, `asTokens()` y `asScopes()`.
export class InMemoryIdentityRepository {
	users: User[]
	tokens: ApiToken[]
	readonly projectsBySlug: Record<string, string[]>
	readonly sessionUser: string
	readonly now: () => Date
	/** Usuarios a los que se les cerraron las sesiones (al quitarles el 2FA). */
	readonly closedSessions: string[] = []
	invitesCreated = 0
	#nextTokenId: number
	readonly calls = { create: 0, revoke: 0, delete: 0, resetTotp: 0 }

	constructor(seed: FakeIdentitySeed = {}) {
		this.users = [...(seed.users ?? [fakeUser()])]
		this.tokens = [...(seed.tokens ?? [])]
		this.projectsBySlug = seed.projects ?? { auditfy: ['staging', 'production'] }
		this.sessionUser = seed.sessionUser ?? 'felipe'
		this.now = seed.now ?? (() => new Date())
		this.#nextTokenId = Math.max(0, ...this.tokens.map((token) => token.id)) + 1
	}

	// --- UserRepository ---

	asUsers(): UserRepository {
		return {
			list: () => Promise.resolve([...this.users]),
			delete: (username) => this.#deleteUser(username),
			resetTotp: (username) => this.#resetTotp(username),
			createInvite: () => this.#createInvite(),
		}
	}

	#deleteUser(username: string): Promise<void> {
		this.calls.delete += 1
		if (username === this.sessionUser) return Promise.reject(new CannotDeleteSelf())
		if (!this.users.some((user) => user.username === username)) {
			return Promise.reject(new UserNotFound())
		}
		this.users = this.users.filter((user) => user.username !== username)
		this.tokens = this.tokens.map((token) =>
			token.createdBy.kind === 'user' && token.createdBy.username === username
				? { ...token, createdBy: { kind: 'deleted' } }
				: token,
		)
		return Promise.resolve()
	}

	#resetTotp(username: string): Promise<void> {
		this.calls.resetTotp += 1
		const user = this.users.find((candidate) => candidate.username === username)
		if (!user) return Promise.reject(new UserNotFound())
		this.users = this.users.map((candidate) =>
			candidate === user ? { ...candidate, totpEnabled: false } : candidate,
		)
		this.closedSessions.push(username)
		return Promise.resolve()
	}

	#createInvite(): Promise<Invite> {
		this.invitesCreated += 1
		return Promise.resolve({
			url: `http://panel.test/invite/inv-${this.invitesCreated}`,
			expiresAt: new Date(this.now().getTime() + 24 * HOUR_MS),
		})
	}

	// --- TokenRepository ---

	asTokens(): TokenRepository {
		return {
			list: () => Promise.resolve([...this.tokens]),
			create: (token) => this.#createToken(token),
			revoke: (id) => this.#revoke(id),
		}
	}

	#createToken(input: NewApiToken): Promise<CreatedApiToken> {
		this.calls.create += 1
		const stages = this.projectsBySlug[input.scope.project]
		if (!stages || (input.scope.stage !== null && !stages.includes(input.scope.stage))) {
			return Promise.reject(new TokenScopeNotFound())
		}
		const now = this.now()
		const token: ApiToken = {
			id: this.#nextTokenId as TokenId,
			name: input.name,
			scope: input.scope,
			createdBy: { kind: 'user', username: this.sessionUser },
			createdAt: now,
			lastUsedAt: null,
			// Como `AddDate(0, 0, n)` del backend: días de calendario, no 24 h × n.
			expiresAt:
				input.expiresInDays > 0
					? new Date(
							now.getFullYear(),
							now.getMonth(),
							now.getDate() + input.expiresInDays,
							now.getHours(),
							now.getMinutes(),
						)
					: null,
		}
		this.#nextTokenId += 1
		this.tokens = [...this.tokens, token]
		return Promise.resolve({ token, secret: `mk_fake${token.id}` })
	}

	#revoke(id: TokenId): Promise<void> {
		this.calls.revoke += 1
		if (!this.tokens.some((token) => token.id === id))
			return Promise.reject(new TokenNotFound())
		this.tokens = this.tokens.filter((token) => token.id !== id)
		return Promise.resolve()
	}

	// --- ScopeRepository ---

	asScopes(): ScopeRepository {
		return {
			projects: () => this.#projects(),
			stages: (project) => this.#stages(project),
		}
	}

	#projects(): Promise<ProjectRef[]> {
		return Promise.resolve(
			Object.keys(this.projectsBySlug).map((slug) => ({ slug, name: slug })),
		)
	}

	#stages(project: string): Promise<StageRef[]> {
		const stages = this.projectsBySlug[project]
		if (!stages) return Promise.reject(new TokenScopeNotFound())
		return Promise.resolve(stages.map((slug) => ({ slug })))
	}
}
