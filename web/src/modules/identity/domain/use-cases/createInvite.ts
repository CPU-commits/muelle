import type { Invite } from '../entities/invite'
import type { UserDeps } from '../repositories/identityRepositories'

/** Una invitación de un solo uso que vence en 24 h; su URL se muestra una sola vez. */
export type CreateInvite = () => Promise<Invite>

export function makeCreateInvite({ users }: UserDeps): CreateInvite {
	return async () => users.createInvite()
}
