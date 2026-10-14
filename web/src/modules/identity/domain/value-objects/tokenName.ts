import type { Brand } from '@/shared/domain'
import { InvalidTokenName, type TokenNameProblem } from '../errors/identityErrors'

export type TokenName = Brand<string, 'TokenName'>

// `validate.Name` del backend con MaxTokenName: obligatorio, hasta 64 caracteres y sin
// caracteres de control ni invisibles. El backend tiene más tablas de "invisibles"; lo que
// se escape de esta regla vuelve como un 422 en el campo.
export const TOKEN_NAME_MAX_LENGTH = 64

const INVISIBLE = /[\p{Cc}\p{Cf}]/u

export function tokenNameProblem(raw: string): TokenNameProblem | null {
	const name = raw.trim()
	if (name.length === 0) return 'empty'
	if ([...name].length > TOKEN_NAME_MAX_LENGTH) return 'long'
	if (INVISIBLE.test(name)) return 'invisible'
	return null
}

/** Sin los espacios de los bordes: "  ci " y "ci" son el mismo nombre en la lista. */
export function parseTokenName(raw: string): TokenName {
	const problem = tokenNameProblem(raw)
	if (problem) throw new InvalidTokenName(problem)
	return raw.trim() as TokenName
}
