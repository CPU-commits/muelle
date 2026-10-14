import type { User } from '../entities/user'
import type { UserDeps } from '../repositories/identityRepositories'

export type ListUsers = (signal?: AbortSignal) => Promise<User[]>

export function makeListUsers({ users }: UserDeps): ListUsers {
	return async (signal) => users.list(signal)
}
