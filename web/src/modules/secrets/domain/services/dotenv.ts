import { type EnvKey, envKeyProblem } from '../value-objects/envKey'
import { envValueProblem } from '../value-objects/envLimits'

// El parser de "Importar .env". Acepta lo que se suele pegar de un `.env`:
//
//   # comentario            líneas en blanco y comentarios se ignoran
//   export TZ=UTC            el prefijo `export ` se ignora
//   PORT = 3000              espacios alrededor de la clave y del `=`
//   URL=https://x/#ancla     un `#` pegado al valor es parte del valor…
//   LOG=info # comentario    …y con un espacio antes empieza un comentario
//   NAME="Hola \"mundo\"\n"  comillas dobles: \n \r \t \" \\ \$ se traducen
//   RAW='sin $escapes\n'     comillas simples: literal
//   KEY="-----BEGIN…         las comillas pueden abarcar varias líneas
//   …-----END-----"
//
// No expande `${VAR}`: el valor queda tal cual. Solo los espacios ASCII se recortan (un
// espacio de no separación es parte del valor). Las claves y los valores se validan con
// las reglas del backend (`validate.EnvKey`, `entity.NewEnvVar`).

export type DotenvProblem =
	| 'missing_equals'
	| 'empty_key'
	| 'invalid_key'
	| 'key_too_long'
	| 'reserved_key'
	| 'unterminated_quote'
	| 'trailing_text'
	| 'value_too_large'
	| 'value_invalid_utf8'
	| 'value_nul'

export interface DotenvEntry {
	readonly key: EnvKey
	readonly value: string
	/** La línea (desde 1) donde empieza la variable. */
	readonly line: number
}

export interface DotenvIssue {
	readonly line: number
	readonly problem: DotenvProblem
	/** La clave, si se llegó a leer. */
	readonly key?: string
}

/** Una clave que aparece más de una vez: gana la última, como en `dotenv`. */
export interface DotenvDuplicate {
	readonly key: EnvKey
	readonly line: number
	readonly firstLine: number
}

/**
 * Una línea dentro de un valor de varias líneas que parece otra variable (`B=2`): suele
 * ser una comilla que no se cerró donde se quería. Es un aviso, no un error.
 */
export interface DotenvSwallowed {
	readonly line: number
	/** La variable cuyo valor se tragó la línea. */
	readonly key: string
	/** La clave que parece tener la línea. */
	readonly looksLike: string
}

export interface ParsedDotenv {
	/** Una por clave (la última que aparece), en el orden del archivo. */
	readonly entries: readonly DotenvEntry[]
	readonly issues: readonly DotenvIssue[]
	readonly duplicates: readonly DotenvDuplicate[]
	readonly swallowed: readonly DotenvSwallowed[]
}

const EXPORT_PREFIX = /^export[ \t]+/
const DOUBLE_QUOTE_ESCAPES: Readonly<Record<string, string>> = {
	n: '\n',
	r: '\r',
	t: '\t',
	'"': '"',
	'\\': '\\',
	$: '$',
}

// Lo que recorta el parser: solo los espacios ASCII, como `strings.TrimSpace` sobre bytes
// de compose-go con valores ASCII. Un U+00A0 al borde es parte del valor.
const ASCII_SPACES = ' \t\n\r\f\v'

function trimAsciiStart(text: string): string {
	let start = 0
	while (start < text.length && ASCII_SPACES.includes(text[start] ?? '')) start += 1
	return text.slice(start)
}

function trimAscii(text: string): string {
	let end = text.length
	while (end > 0 && ASCII_SPACES.includes(text[end - 1] ?? '')) end -= 1
	return trimAsciiStart(text.slice(0, end))
}

// Una línea con forma de `KEY=` o `export KEY=` (en mayúsculas, como se escriben). El
// `(?!=)` deja afuera el relleno de base64 (`QUJD==`) de una clave PEM.
const LOOKS_LIKE_ASSIGNMENT = /^[ \t]*(?:export[ \t]+)?([A-Z_][A-Z0-9_]*)[ \t]*=(?!=)/

function keyIssue(key: string): DotenvProblem | null {
	switch (envKeyProblem(key)) {
		case null:
			return null
		case 'empty':
			return 'empty_key'
		case 'format':
			return 'invalid_key'
		case 'too_long':
			return 'key_too_long'
		case 'reserved':
			return 'reserved_key'
	}
}

function valueIssue(value: string): DotenvProblem | null {
	switch (envValueProblem(value)) {
		case null:
			return null
		case 'too_large':
			return 'value_too_large'
		case 'invalid_utf8':
			return 'value_invalid_utf8'
		case 'nul':
			return 'value_nul'
	}
}

/** El índice de la comilla que cierra en `text`, o -1. En las dobles, `\"` no cierra. */
function closingQuote(text: string, quote: string): number {
	for (let index = 0; index < text.length; index += 1) {
		const char = text[index]
		if (quote === '"' && char === '\\') {
			index += 1
			continue
		}
		if (char === quote) return index
	}
	return -1
}

function unescapeDouble(text: string): string {
	return text.replace(/\\(.)/gs, (match, char: string) => DOUBLE_QUOTE_ESCAPES[char] ?? match)
}

/** Un valor sin comillas: hasta un ` #` (comentario), sin los espacios de los bordes. */
function unquotedValue(raw: string): string {
	const comment = raw.search(/[ \t]#/)
	return trimAscii(comment >= 0 ? raw.slice(0, comment) : raw)
}

/** Las líneas (desde 1) que se tragó un valor de varias líneas y su posible clave. */
type Swallowed = readonly { readonly line: number; readonly looksLike: string }[]

type ValueResult =
	| {
			readonly ok: true
			readonly value: string
			readonly nextLine: number
			readonly swallowed: Swallowed
	  }
	| {
			readonly ok: false
			readonly problem: DotenvProblem
			readonly nextLine: number
			readonly swallowed: Swallowed
	  }

/**
 * Lee el valor que empieza en `raw` (lo que sigue al `=` de la línea `lineIndex`). Si
 * tiene comillas que no cierran en la línea, sigue con las siguientes.
 */
function readValue(raw: string, lines: readonly string[], lineIndex: number): ValueResult {
	const text = trimAsciiStart(raw)
	const quote = text[0]
	if (quote !== '"' && quote !== "'") {
		// `KEY= # nada` es un valor vacío: el `#` tiene un espacio antes.
		const value = text.length < raw.length && text.startsWith('#') ? '' : unquotedValue(text)
		return { ok: true, value, nextLine: lineIndex + 1, swallowed: [] }
	}

	let rest = text.slice(1)
	let body = ''
	let next = lineIndex + 1
	const swallowed: { line: number; looksLike: string }[] = []
	for (;;) {
		const end = closingQuote(rest, quote)
		if (end >= 0) {
			body += rest.slice(0, end)
			rest = trimAscii(rest.slice(end + 1))
			break
		}
		if (next >= lines.length) {
			return { ok: false, problem: 'unterminated_quote', nextLine: next, swallowed }
		}
		body += `${rest}\n`
		rest = lines[next] ?? ''
		const looksLike = LOOKS_LIKE_ASSIGNMENT.exec(rest)?.[1]
		if (looksLike) swallowed.push({ line: next + 1, looksLike })
		next += 1
	}
	if (rest.length > 0 && !rest.startsWith('#')) {
		return { ok: false, problem: 'trailing_text', nextLine: next, swallowed }
	}
	return {
		ok: true,
		value: quote === '"' ? unescapeDouble(body) : body,
		nextLine: next,
		swallowed,
	}
}

export function parseDotenv(text: string): ParsedDotenv {
	const lines = text.replace(/^\uFEFF/, '').split(/\r\n|\r|\n/)
	const entries = new Map<EnvKey, DotenvEntry>()
	const issues: DotenvIssue[] = []
	const duplicates: DotenvDuplicate[] = []
	const swallowed: DotenvSwallowed[] = []

	let index = 0
	while (index < lines.length) {
		const line = index + 1
		const content = trimAsciiStart(lines[index] ?? '')
		if (content.length === 0 || content.startsWith('#')) {
			index += 1
			continue
		}

		const statement = content.replace(EXPORT_PREFIX, '')
		const equals = statement.indexOf('=')
		if (equals < 0) {
			issues.push({ line, problem: 'missing_equals' })
			index += 1
			continue
		}

		const key = trimAscii(statement.slice(0, equals))
		const result = readValue(statement.slice(equals + 1), lines, index)
		index = result.nextLine
		for (const suspect of result.swallowed) swallowed.push({ ...suspect, key })

		// Con una clave mala, ese es el problema que se muestra aunque el valor también lo tenga.
		const problem = keyIssue(key) ?? (result.ok ? valueIssue(result.value) : result.problem)
		if (problem !== null) {
			issues.push({ line, problem, ...(key ? { key } : {}) })
			continue
		}
		if (!result.ok) continue

		const envKey = key as EnvKey
		const previous = entries.get(envKey)
		if (previous) {
			duplicates.push({ key: envKey, line, firstLine: previous.line })
			// Gana la última: se saca y se vuelve a poner para que quede en su lugar.
			entries.delete(envKey)
		}
		entries.set(envKey, { key: envKey, value: result.value, line })
	}

	return { entries: [...entries.values()], issues, duplicates, swallowed }
}
