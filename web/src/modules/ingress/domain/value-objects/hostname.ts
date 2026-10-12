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

export function hostnameProblem(raw: string): string | null {
	const value = normalizeHostname(raw)
	if (value.length === 0) return 'el hostname está vacío'
	if (value.length > HOSTNAME_MAX_LENGTH) return 'es demasiado largo'
	if (/^[a-z]+:\/\//.test(value) || value.includes('/')) return 'va sin https:// ni ruta'
	const labels = value.split('.')
	if (labels.length < 2) return 'le falta el dominio (ej. api.tudominio.cl)'
	const tld = labels.at(-1) ?? ''
	if (!TLD.test(tld)) return 'el dominio de primer nivel no es válido'
	if (!labels.every((label) => LABEL.test(label))) {
		return 'solo letras, números y guiones, sin empezar ni terminar con guion'
	}
	return null
}

export function parseHostname(raw: string, field = 'hostname'): Hostname {
	const problem = hostnameProblem(raw)
	if (problem) throw new ValidationError(problem, field)
	return normalizeHostname(raw) as Hostname
}

/** Para lo que ya viene validado del backend. */
export function asHostname(raw: string): Hostname {
	return raw as Hostname
}
