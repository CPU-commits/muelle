import type { EnvValueProblem } from '../errors/secretsErrors'

// Los límites de `entity` en secrets (src/internal/secrets/domain/entity/envvar.go).

/** Variables por dueño (stage o app), y también por `set` y por `unset` de un PATCH. */
export const MAX_ENV_VARS = 500
/** Bytes por valor, en UTF-8. */
export const MAX_VALUE_BYTES = 64 * 1024
/** Bytes por dueño: claves + valores, en UTF-8. */
export const MAX_OWNER_BYTES = 1024 * 1024

/**
 * Cuántos bytes ocupa `text` en UTF-8, sin armar el arreglo. Un surrogate suelto cuenta
 * como el carácter de reemplazo (3 bytes), igual que `TextEncoder`.
 */
export function utf8Length(text: string): number {
	let bytes = 0
	for (const char of text) {
		const code = char.codePointAt(0) ?? 0
		if (code < 0x80) bytes += 1
		else if (code < 0x800) bytes += 2
		else if (code < 0x10000) bytes += 3
		else bytes += 4
	}
	return bytes
}

/** Lo que ocupa una variable en el entorno (`EnvVar.Size` del backend). */
export function envVarSize(key: string, value: string): number {
	return utf8Length(key) + utf8Length(value)
}

/**
 * Qué tiene de malo un valor, o null si el backend lo acepta. En el mismo orden que
 * `NewEnvVar`: tamaño, UTF-8 y NUL. Un string de JS con un surrogate suelto no es UTF-8
 * válido: el backend ni siquiera lo decodifica (400).
 */
export function envValueProblem(value: string): EnvValueProblem | null {
	if (utf8Length(value) > MAX_VALUE_BYTES) return 'too_large'
	if (!value.isWellFormed()) return 'invalid_utf8'
	if (value.includes('\0')) return 'nul'
	return null
}
