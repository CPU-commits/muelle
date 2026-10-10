import type { Brand } from '@/shared/domain'
import { type EnvField, type EnvKeyProblem, InvalidEnvKey } from '../errors/secretsErrors'

/** La clave de una variable, ya validada con las reglas del backend. */
export type EnvKey = Brand<string, 'EnvKey'>

// `validate.EnvKey` (src/shared/domain/validate/validate.go): empieza con letra o '_',
// sigue con letras, números o '_', y tiene hasta 128 caracteres. Distingue mayúsculas.
export const ENV_KEY_PATTERN = /^[A-Za-z_][A-Za-z0-9_]{0,127}$/
export const ENV_KEY_MAX_LENGTH = 128
// compose-go lee del entorno las variables COMPOSE_* (§5.2 del spec). Solo en mayúsculas,
// como `strings.HasPrefix` del backend: `compose_x` es una clave válida.
export const RESERVED_ENV_PREFIX = 'COMPOSE_'

const ENV_KEY_CHARS = /^[A-Za-z_][A-Za-z0-9_]*$/

/** Qué tiene de malo una clave, o null si el backend la acepta. No recorta espacios. */
export function envKeyProblem(key: string): EnvKeyProblem | null {
	if (key.length === 0) return 'empty'
	if (!ENV_KEY_CHARS.test(key)) return 'format'
	if (key.length > ENV_KEY_MAX_LENGTH) return 'too_long'
	if (key.startsWith(RESERVED_ENV_PREFIX)) return 'reserved'
	return null
}

/** Para lo que escribe una persona: los espacios de los bordes no son parte de la clave. */
export function normalizeEnvKey(raw: string): string {
	return raw.trim()
}

export function isValidEnvKey(key: string): key is EnvKey {
	return envKeyProblem(key) === null
}

/** Valida una clave tal cual (sin recortar). `field` es el campo del cuerpo donde va. */
export function parseEnvKey(key: string, field: EnvField): EnvKey {
	const problem = envKeyProblem(key)
	if (problem) throw new InvalidEnvKey(problem, key, field)
	return key as EnvKey
}
