import type { Brand } from '@/shared/domain'
import { InvalidPassword, type PasswordProblem } from '../errors/authErrors'

export type Password = Brand<string, 'Password'>

// `validate.Password` del backend: al menos 12 caracteres (runas, no bytes) y como
// mucho 256 bytes en UTF-8.
export const PASSWORD_MIN_LENGTH = 12
export const PASSWORD_MAX_BYTES = 256

function byteLength(raw: string): number {
	return new TextEncoder().encode(raw).length
}

export function passwordProblem(raw: string): PasswordProblem | null {
	if (raw.length === 0) return 'empty'
	if ([...raw].length < PASSWORD_MIN_LENGTH) return 'short'
	if (byteLength(raw) > PASSWORD_MAX_BYTES) return 'long'
	return null
}

/** Para una cuenta nueva: la regla completa. La contraseña nunca se recorta. */
export function parseNewPassword(raw: string): Password {
	const problem = passwordProblem(raw)
	if (problem) throw new InvalidPassword(problem)
	return raw as Password
}

/** Para entrar: que no esté vacía ni pase el máximo (el backend la rechazaría sin mirar). */
export function parseLoginPassword(raw: string): Password {
	if (raw.length === 0) throw new InvalidPassword('empty')
	if (byteLength(raw) > PASSWORD_MAX_BYTES) throw new InvalidPassword('long')
	return raw as Password
}
