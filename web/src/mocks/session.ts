import { apiError } from './api'
import { authDb, type MockUser } from './db/auth'

// Para los handlers de rutas con sesión: lo mismo que responde el Kit del backend sin
// sesión, con la sesión pendiente del código o sin la 2FA que exige el panel.
//
//   http.get(api('/projects'), () => requireSession() ?? HttpResponse.json(...))
export function requireSession() {
	const session = authDb.state.session
	if (!session) return apiError(401, 'unauthenticated', 'inicia sesión')
	if (session.mfaPending) return apiError(401, 'mfa_required', 'falta el código de 2FA')
	const user = authDb.findUser(session.username)
	if (!user) return apiError(401, 'unauthenticated', 'inicia sesión')
	if (authDb.needsTotpSetup(user)) {
		return apiError(403, 'totp_setup_required', 'configura 2FA para continuar')
	}
	return undefined
}

/** El usuario de la sesión (después de `requireSession`). */
export function sessionUser(): MockUser | undefined {
	return authDb.currentUser()
}
