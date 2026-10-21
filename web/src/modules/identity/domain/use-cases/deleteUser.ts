import { CannotDeleteSelf } from '../errors/identityErrors'
import type { UserDeps } from '../repositories/identityRepositories'

export interface DeleteUserInput {
	username: string
	/** El usuario de la sesión: a sí mismo no se borra (el backend también lo rechaza). */
	currentUsername: string | null
}

/** Borra la cuenta con sus sesiones e invitaciones. Sus tokens siguen: son del proyecto. */
export type DeleteUser = (input: DeleteUserInput) => Promise<void>

export function makeDeleteUser({ users }: UserDeps): DeleteUser {
	return async ({ username, currentUsername }) => {
		if (currentUsername !== null && username === currentUsername) throw new CannotDeleteSelf()
		await users.delete(username)
	}
}
