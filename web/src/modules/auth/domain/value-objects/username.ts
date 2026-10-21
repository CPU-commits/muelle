import type { Brand } from '@/shared/domain'
import { InvalidUsername } from '../errors/authErrors'

export type Username = Brand<string, 'Username'>

// La regla de `validate.Username` del backend: empieza con letra o número y tiene de 3
// a 32 caracteres de a-z, 0-9, '.', '_' o '-'.
export const USERNAME_PATTERN = /^[a-z0-9][a-z0-9._-]{2,31}$/

// El backend solo acepta minúsculas: "Felipe" se escribe "felipe". Se normaliza antes de
// validar para que el usuario no tenga que saberlo.
export function normalizeUsername(raw: string): string {
	return raw.trim().toLowerCase()
}

export function isValidUsername(raw: string): boolean {
	return USERNAME_PATTERN.test(normalizeUsername(raw))
}

/** Para crear una cuenta (setup, invitación): exige la regla completa. */
export function parseUsername(raw: string): Username {
	const normalized = normalizeUsername(raw)
	if (!USERNAME_PATTERN.test(normalized)) throw new InvalidUsername()
	return normalized as Username
}

/**
 * Para entrar: solo exige que no esté vacío. Un nombre fuera de la regla no existe, y el
 * backend lo responde igual que una contraseña mala (sin revelar qué usuarios hay).
 */
export function parseLoginUsername(raw: string): Username {
	const normalized = normalizeUsername(raw)
	if (normalized.length === 0) throw new InvalidUsername()
	return normalized as Username
}

export function usernameInitial(username: string): string {
	return username.trim().charAt(0).toUpperCase() || '?'
}
