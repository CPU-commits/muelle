import type { AuthDeps } from '../repositories/authRepository'
import { type CredentialsInput, parseNewAccount } from '../value-objects/credentials'
import { parseInviteToken } from '../value-objects/inviteToken'
import type { Username } from '../value-objects/username'

export interface AcceptInviteInput extends CredentialsInput {
	/** El `<token>` de `/invite/<token>`. */
	token: string
}

/** Crea la cuenta invitada y deja su sesión abierta. */
export type AcceptInvite = (input: AcceptInviteInput) => Promise<Username>

export function makeAcceptInvite({ auth }: AuthDeps): AcceptInvite {
	return async ({ token, ...account }) =>
		auth.acceptInvite(parseInviteToken(token), parseNewAccount(account))
}
