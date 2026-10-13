import { ValidationError } from '@/shared/domain'

// A dónde apunta un dominio: un servicio de la app y su puerto (spec §5.1
// `target_service`, `target_port`). En la UI se escribe "api:3000".
export interface DomainTarget {
	readonly service: string
	readonly port: number
}

// El nombre de un servicio sigue la regla de los slugs (spec §5.2): hasta 16, empieza con
// letra y no termina en guion.
const SERVICE = /^[a-z]([a-z0-9-]{0,14}[a-z0-9])?$/
const TARGET = /^\s*([^:\s]+)\s*:\s*(\d{1,5})\s*$/

export function formatTarget(target: DomainTarget): string {
	return `${target.service}:${target.port}`
}

/** Por qué un destino no sirve; la UI lo traduce. */
export type TargetProblem = 'format' | 'service' | 'port'

const PROBLEM_TEXT: Record<TargetProblem, string> = {
	format: 'escribe servicio:puerto (ej. api:3000)',
	service: 'no es un nombre de servicio válido',
	port: 'el puerto va de 1 a 65535',
}

export function targetProblem(raw: string): TargetProblem | null {
	const match = TARGET.exec(raw)
	if (!match) return 'format'
	const [, service = '', port = ''] = match
	if (!SERVICE.test(service)) return 'service'
	const value = Number(port)
	if (!Number.isInteger(value) || value < 1 || value > 65535) return 'port'
	return null
}

export function parseTarget(raw: string, field = 'target'): DomainTarget {
	const problem = targetProblem(raw)
	if (problem) throw new ValidationError(PROBLEM_TEXT[problem], field)
	const [, service = '', port = ''] = TARGET.exec(raw) ?? []
	return { service, port: Number(port) }
}

export function sameTarget(a: DomainTarget, b: DomainTarget): boolean {
	return a.service === b.service && a.port === b.port
}
