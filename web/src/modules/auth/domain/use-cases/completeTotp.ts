import { NoMfaPending } from '../errors/authErrors'
import type { AuthDeps } from '../repositories/authRepository'
import { parseTotpCode } from '../value-objects/totpCode'

export type CompleteTotp = (code: string) => Promise<void>

export function makeCompleteTotp({ auth }: AuthDeps): CompleteTotp {
	return async (code) => {
		try {
			await auth.completeTotp(parseTotpCode(code))
		} catch (error) {
			// Un doble envío: el primero ya completó la sesión. Para el usuario es un éxito.
			if (error instanceof NoMfaPending) return
			throw error
		}
	}
}
