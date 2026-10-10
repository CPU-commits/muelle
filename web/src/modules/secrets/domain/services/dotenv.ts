import { type EnvKey, envKeyProblem } from '../value-objects/envKey'
import { envValueProblem } from '../value-objects/envLimits'

// El parser de "Importar .env". Acepta lo que se suele pegar de un `.env`:
//
//   # comentario            líneas en blanco y comentarios se ignoran
//   export TZ=UTC            el prefijo `export ` se ignora
//   PORT = 3000              espacios alrededor de la clave y del `=`
//   URL=https://x/#ancla     un `#` pegado al valor es parte del valor…
//   LOG=info # comentario    …y con un espacio antes empieza un comentario
//   NAME="Hola \"mundo\"\n"  comillas dobles: \n \r \t \" \\ se traducen
//   RAW='sin $escapes\n'     comillas simples: literal
//   KEY="-----BEGIN…         las comillas pueden abarcar varias líneas
//   …-----END-----"
//
// No expande `${VAR}`: el valor queda tal cual. Las claves y los valores se validan con
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

export interface ParsedDotenv {
	/** Una por clave (la última que aparece), en el orden del archivo. */
	readonly entries: readonly DotenvEntry[]
	readonly issues: readonly DotenvIssue[]
	readonly duplicates: readonly DotenvDuplicate[]
}

const EXPORT_PREFIX = /^export[ \t]+/
const DOUBLE_QUOTE_ESCAPES: Readonly<Record<string, string>> = {
	n: '\n',
	r: '\r',
	t: '\t',
	'"': '"',
	'\\': '\\',
}

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
	const comment = raw.search(/\s#/)
	return (comment >= 0 ? raw.slice(0, comment) : raw).trim()
}

type ValueResult =
	| { readonly ok: true; readonly value: string; readonly nextLine: number }
	| { readonly ok: false; readonly problem: DotenvProblem; readonly nextLine: number }

/**
 * Lee el valor que empieza en `raw` (lo que sigue al `=` de la línea `lineIndex`). Si
 * tiene comillas que no cierran en la línea, sigue con las siguientes.
 */
function readValue(raw: string, lines: readonly string[], lineIndex: number): ValueResult {
	const text = raw.trimStart()
	const quote = text[0]
	if (quote !== '"' && quote !== "'") {
		// `KEY= # nada` es un valor vacío: el `#` tiene un espacio antes.
		const value = text.length < raw.length && text.startsWith('#') ? '' : unquotedValue(text)
		return { ok: true, value, nextLine: lineIndex + 1 }
	}

	let rest = text.slice(1)
	let body = ''
	let next = lineIndex + 1
	for (;;) {
		const end = closingQuote(rest, quote)
		if (end >= 0) {
			body += rest.slice(0, end)
			rest = rest.slice(end + 1).trim()
			break
		}
		if (next >= lines.length)
			return { ok: false, problem: 'unterminated_quote', nextLine: next }
		body += `${rest}\n`
		rest = lines[next] ?? ''
		next += 1
	}
	if (rest.length > 0 && !rest.startsWith('#')) {
		return { ok: false, problem: 'trailing_text', nextLine: next }
	}
	return { ok: true, value: quote === '"' ? unescapeDouble(body) : body, nextLine: next }
}

export function parseDotenv(text: string): ParsedDotenv {
	const lines = text.replace(/^\uFEFF/, '').split(/\r\n|\r|\n/)
	const entries = new Map<EnvKey, DotenvEntry>()
	const issues: DotenvIssue[] = []
	const duplicates: DotenvDuplicate[] = []

	let index = 0
	while (index < lines.length) {
		const line = index + 1
		const content = (lines[index] ?? '').trimStart()
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

		const key = statement.slice(0, equals).trim()
		const result = readValue(statement.slice(equals + 1), lines, index)
		index = result.nextLine

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

	return { entries: [...entries.values()], issues, duplicates }
}
