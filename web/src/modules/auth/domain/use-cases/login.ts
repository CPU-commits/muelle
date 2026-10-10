import type { LoginOutcome } from '../entities/session'
import type { AuthDeps } from '../repositories/authRepository'
import { type CredentialsInput, parseLoginCredentials } from '../value-objects/credentials'

export type LoginInput = CredentialsInput

/** Devuelve `{ mfaRequired: true }` si la cuenta tiene 2FA: falta `completeTotp`. */
export type Login = (input: LoginInput) => Promise<LoginOutcome>

export function makeLogin({ auth }: AuthDeps): Login {
	return async (input) => auth.login(parseLoginCredentials(input))
}
