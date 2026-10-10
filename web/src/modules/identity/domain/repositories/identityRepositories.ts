import type { ApiToken, CreatedApiToken, NewApiToken } from '../entities/apiToken'
import type { Invite } from '../entities/invite'
import type { ProjectRef, StageRef } from '../entities/scope'
import type { User } from '../entities/user'
import type { TokenId } from '../value-objects/tokenId'

// Los puertos de identity. La implementación HTTP traduce los `error.code` del backend a
// los errores de `errors/identityErrors.ts`; ningún método devuelve un status.

export interface UserRepository {
	list(signal?: AbortSignal): Promise<User[]>
	delete(username: string): Promise<void>
	/** Quita el 2FA del usuario y cierra todas sus sesiones. */
	resetTotp(username: string): Promise<void>
	createInvite(): Promise<Invite>
}

export interface TokenRepository {
	list(signal?: AbortSignal): Promise<ApiToken[]>
	create(token: NewApiToken): Promise<CreatedApiToken>
	revoke(id: TokenId): Promise<void>
}

/** Los proyectos y stages que puede tener el alcance de un token (son de `catalog`). */
export interface ScopeRepository {
	projects(signal?: AbortSignal): Promise<ProjectRef[]>
	stages(project: string, signal?: AbortSignal): Promise<StageRef[]>
}

export interface UserDeps {
	readonly users: UserRepository
}

export interface TokenDeps {
	readonly tokens: TokenRepository
}

export interface ScopeDeps {
	readonly scopes: ScopeRepository
}
