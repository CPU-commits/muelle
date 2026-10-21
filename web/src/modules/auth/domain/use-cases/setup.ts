import type { AuthDeps } from '../repositories/authRepository'
import { type CredentialsInput, parseNewAccount } from '../value-objects/credentials'
import { parseInviteToken } from '../value-objects/inviteToken'
import type { Username } from '../value-objects/username'

export interface SetupInput extends CredentialsInput {
	/** El `?token=` del link que imprime `muelle serve`. */
	token: string | null
}

/** Crea el primer usuario y deja su sesión abierta. */
export type Setup = (input: SetupInput) => Promise<Username>

export function makeSetup({ auth }: AuthDeps): Setup {
	return async ({ token, ...account }) =>
		auth.setup(parseInviteToken(token), parseNewAccount(account))
}
