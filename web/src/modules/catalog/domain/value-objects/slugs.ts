import type { Brand } from '@/shared/domain'
import { InvalidSlug, type SlugProblem } from '../errors/catalogErrors'

export type ProjectSlug = Brand<string, 'ProjectSlug'>
export type StageSlug = Brand<string, 'StageSlug'>
export type AppSlug = Brand<string, 'AppSlug'>

// Las reglas de `validate.ProjectSlug`, `StageSlug` y `AppSlug` del backend: empiezan con
// letra, terminan con letra o número, solo a-z, 0-9 y '-'. El '_' queda libre como
// separador de los nombres de Swarm (`<proyecto>_<stage>_<app>`).
export const PROJECT_SLUG_MAX = 16
export const STAGE_SLUG_MAX = 12
export const APP_SLUG_MAX = 16

const SLUG_16 = /^[a-z]([a-z0-9-]{0,14}[a-z0-9])?$/
const SLUG_12 = /^[a-z]([a-z0-9-]{0,10}[a-z0-9])?$/

/** La red de un stage es `<proyecto>_<stage>`: con estos chocaría con redes de Docker o del panel. */
export const RESERVED_PROJECT_SLUGS: readonly string[] = ['muelle', 'docker']

// El backend solo acepta minúsculas: "Auditfy" se escribe "auditfy".
export function normalizeSlug(raw: string): string {
	return raw.trim().toLowerCase()
}

export function projectSlugProblem(raw: string): SlugProblem | null {
	const slug = normalizeSlug(raw)
	if (!SLUG_16.test(slug)) return 'format'
	return RESERVED_PROJECT_SLUGS.includes(slug) ? 'reserved' : null
}

export function stageSlugProblem(raw: string): SlugProblem | null {
	return SLUG_12.test(normalizeSlug(raw)) ? null : 'format'
}

export function appSlugProblem(raw: string): SlugProblem | null {
	return SLUG_16.test(normalizeSlug(raw)) ? null : 'format'
}

/** Un volumen o un servicio de compose siguen la misma regla que una app. */
export function isValidVolumeName(raw: string): boolean {
	return SLUG_16.test(raw)
}

export function parseProjectSlug(raw: string): ProjectSlug {
	const problem = projectSlugProblem(raw)
	if (problem) throw new InvalidSlug(problem, PROJECT_SLUG_MAX)
	return normalizeSlug(raw) as ProjectSlug
}

export function parseStageSlug(raw: string): StageSlug {
	const problem = stageSlugProblem(raw)
	if (problem) throw new InvalidSlug(problem, STAGE_SLUG_MAX)
	return normalizeSlug(raw) as StageSlug
}

export function parseAppSlug(raw: string): AppSlug {
	const problem = appSlugProblem(raw)
	if (problem) throw new InvalidSlug(problem, APP_SLUG_MAX)
	return normalizeSlug(raw) as AppSlug
}

/**
 * Un slug sugerido a partir de un nombre ("Aumi Agents" → "aumi-agents"): sin acentos,
 * con guiones en lugar de lo demás y cortado al máximo. Puede no ser válido (un nombre
 * que empieza con un número): el formulario lo valida igual.
 */
export function slugFromName(name: string, max: number): string {
	return name
		.normalize('NFD')
		.replace(/\p{M}/gu, '')
		.toLowerCase()
		.replace(/[^a-z0-9]+/g, '-')
		.replace(/^-+/, '')
		.slice(0, max)
		.replace(/-+$/, '')
}
