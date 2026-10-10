import type { TotpSetup } from '../entities/totpSetup'
import type { AuthDeps } from '../repositories/authRepository'

/** Genera un secreto nuevo (pendiente): si ya había 2FA, la actual sigue hasta confirmarlo. */
export type BeginTotpSetup = () => Promise<TotpSetup>

export function makeBeginTotpSetup({ auth }: AuthDeps): BeginTotpSetup {
	return async () => auth.beginTotpSetup()
}
