import { UnauthorizedError } from '@/shared/domain'
import { MfaRequired } from '../errors/authErrors'
import type { Me } from './me'

/** Lo que responde el login: si hace falta el segundo paso. */
export interface LoginOutcome {
	readonly mfaRequired: boolean
}

/**
 * En qué punto está la sesión del navegador. La cookie es HttpOnly: el panel no la ve,
 * lo deduce de `GET /auth/me`.
 */
export type SessionState =
	| { readonly status: 'signed_in'; readonly me: Me }
	| { readonly status: 'mfa_pending' }
	| { readonly status: 'signed_out' }

/**
 * Traduce el error de `me()` a un estado de sesión. Devuelve null si el error no tiene
 * que ver con la sesión (backend caído, red): eso no es "sin sesión" y no hay que mandar
 * a nadie al login por eso.
 */
export function sessionStateFromError(error: unknown): SessionState | null {
	if (error instanceof MfaRequired) return { status: 'mfa_pending' }
	if (error instanceof UnauthorizedError) return { status: 'signed_out' }
	return null
}
