import { InvalidProjectName, type ProjectNameProblem } from '../errors/catalogErrors'

/** `validate.Name("name", …, 64)` del backend: cuenta caracteres (runas), no bytes. */
export const PROJECT_NAME_MAX = 64

// Caracteres de control, de formato, separadores de línea y de párrafo, los ignorables por
// defecto (que incluyen los selectores de variación y el ZWJ de los emoji) y los que se
// ven en blanco (braille vacío, rellenos Hangul). El backend rechaza lo mismo.
const INVISIBLE = /[\p{Cc}\p{Cf}\p{Zl}\p{Zp}\p{Default_Ignorable_Code_Point}\u2800]/u

export function projectNameProblem(raw: string): ProjectNameProblem | null {
	const name = raw.trim()
	if (name.length === 0) return 'empty'
	if ([...name].length > PROJECT_NAME_MAX) return 'long'
	if (INVISIBLE.test(name)) return 'invisible'
	return null
}

/** El nombre visible de un proyecto, sin los espacios de los bordes. */
export function parseProjectName(raw: string): string {
	const problem = projectNameProblem(raw)
	if (problem) throw new InvalidProjectName(problem)
	return raw.trim()
}
