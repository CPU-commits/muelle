import { AmbiguousSessionError, UnauthorizedError } from '@/shared/domain'
import type { AuthDeps } from '../repositories/authRepository'

export type Logout = () => Promise<void>

// Un 401 (sin sesión, o una que esperaba el código) es "ya estaba cerrada": cuenta como
// éxito. Cualquier otra falla (backend caído, dos cookies de sesión) se propaga: la sesión
// sigue abierta en el servidor y la pantalla tiene que decirlo en vez de fingir que salió.
export function makeLogout({ auth }: AuthDeps): Logout {
	return async () => {
		try {
			await auth.logout()
		} catch (error) {
			if (error instanceof UnauthorizedError && !(error instanceof AmbiguousSessionError)) {
				return
			}
			throw error
		}
	}
}
