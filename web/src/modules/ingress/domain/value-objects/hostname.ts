import { type Brand, ValidationError } from '@/shared/domain'

// Un hostname de un dominio (spec §5.1 tabla `domains`: único global). Se guarda en
// minúsculas y sin el punto final. Sin comodines: cada dominio es un registro y una ruta.
export type Hostname = Brand<string, 'Hostname'>

export const HOSTNAME_MAX_LENGTH = 253

const LABEL = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/
const TLD = /^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$/

export function normalizeHostname(raw: string): string {
	return raw.trim().toLowerCase().replace(/\.$/, '')
}

/** Por qué un hostname no sirve; la UI lo traduce. */
export type HostnameProblem =
	| 'empty'
	| 'too_long'
	| 'not_bare'
	| 'no_domain'
	| 'bad_tld'
	| 'bad_label'

// El texto de cada problema para el `ValidationError` (logs, y si llega sin traducir).
const PROBLEM_TEXT: Record<HostnameProblem, string> = {
	empty: 'el hostname está vacío',
	too_long: 'es demasiado largo',
	not_bare: 'va sin https:// ni ruta',
	no_domain: 'le falta el dominio (ej. api.tudominio.cl)',
	bad_tld: 'el dominio de primer nivel no es válido',
	bad_label: 'solo letras, números y guiones, sin empezar ni terminar con guion',
}

export function hostnameProblem(raw: string): HostnameProblem | null {
	const value = normalizeHostname(raw)
	if (value.length === 0) return 'empty'
	if (value.length > HOSTNAME_MAX_LENGTH) return 'too_long'
	if (/^[a-z]+:\/\//.test(value) || value.includes('/')) return 'not_bare'
	const labels = value.split('.')
	if (labels.length < 2) return 'no_domain'
	const tld = labels.at(-1) ?? ''
	if (!TLD.test(tld)) return 'bad_tld'
	if (!labels.every((label) => LABEL.test(label))) return 'bad_label'
	return null
}

export function parseHostname(raw: string, field = 'hostname'): Hostname {
	const problem = hostnameProblem(raw)
	if (problem) throw new ValidationError(PROBLEM_TEXT[problem], field)
	return normalizeHostname(raw) as Hostname
}

/** Para lo que ya viene validado del backend. */
export function asHostname(raw: string): Hostname {
	return raw as Hostname
}
