import type { AuthDeps } from '../repositories/authRepository'
import { parseTotpCode } from '../value-objects/totpCode'

export type EnableTotp = (code: string) => Promise<void>

export function makeEnableTotp({ auth }: AuthDeps): EnableTotp {
	return async (code) => auth.enableTotp(parseTotpCode(code))
}
