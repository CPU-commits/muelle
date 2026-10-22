import { resolve } from '$app/paths'
import type { AppRef } from '../../../domain'

// Los destinos de los selectores del breadcrumb. Cambiar de proyecto, stage o app deja al
// usuario en el mismo tab (Configuración de una app → Configuración de la otra).

export const APP_TABS = ['', 'configuration', 'variables', 'domains', 'deploys', 'logs'] as const
export type AppTab = (typeof APP_TABS)[number]

/** El tab de una app que muestra la URL (`…/apps/api/deploys` → `deploys`). */
export function tabOf(pathname: string): AppTab {
	const last = /\/apps\/[^/]+\/([^/]+)\/?$/.exec(pathname)?.[1] ?? ''
	return (APP_TABS as readonly string[]).includes(last) ? (last as AppTab) : ''
}

export function appTabHref(ref: AppRef, tab: AppTab): string {
	const params = { p: ref.project, s: ref.stage, a: ref.app }
	switch (tab) {
		case '':
			return resolve('/(app)/projects/[p]/stages/[s]/apps/[a]', params)
		case 'configuration':
			return resolve('/(app)/projects/[p]/stages/[s]/apps/[a]/configuration', params)
		case 'variables':
			return resolve('/(app)/projects/[p]/stages/[s]/apps/[a]/variables', params)
		case 'domains':
			return resolve('/(app)/projects/[p]/stages/[s]/apps/[a]/domains', params)
		case 'deploys':
			return resolve('/(app)/projects/[p]/stages/[s]/apps/[a]/deploys', params)
		case 'logs':
			return resolve('/(app)/projects/[p]/stages/[s]/apps/[a]/logs', params)
	}
}

export function stageVariablesHref(project: string, stage: string): string {
	return resolve('/(app)/projects/[p]/stages/[s]/variables', { p: project, s: stage })
}

/** El proyecto en Proyectos (su sección tiene `id="p-<slug>"`). */
export function projectSectionHref(project: string): string {
	return `${resolve('/')}#p-${project}`
}
