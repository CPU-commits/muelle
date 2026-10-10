import type { AuthDeps } from '../repositories/authRepository'

export type Logout = () => Promise<void>

// Cerrar sesión nunca falla del lado del usuario: si el backend no responde o la sesión
// ya había vencido, el panel igual vuelve al login.
export function makeLogout({ auth }: AuthDeps): Logout {
	return async () => {
		try {
			await auth.logout()
		} catch {
			return
		}
	}
}
