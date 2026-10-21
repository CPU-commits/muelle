import type { Username } from '../value-objects/username'

/** Quién tiene la sesión (`GET /auth/me`). */
export interface Me {
	readonly username: Username
	readonly totpEnabled: boolean
	/** El panel exige 2FA y este usuario no la tiene: solo puede ir a configurarla. */
	readonly needsTotpSetup: boolean
}

export type Landing = 'home' | 'totp_setup'

/** A dónde va alguien con sesión completa. */
export function landingFor(me: Me): Landing {
	return me.needsTotpSetup ? 'totp_setup' : 'home'
}
