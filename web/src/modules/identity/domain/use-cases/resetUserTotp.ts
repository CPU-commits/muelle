import type { UserDeps } from '../repositories/identityRepositories'

/** Para quien perdió su teléfono: le quita el 2FA y le cierra todas las sesiones. */
export type ResetUserTotp = (username: string) => Promise<void>

export function makeResetUserTotp({ users }: UserDeps): ResetUserTotp {
	return async (username) => users.resetTotp(username)
}
