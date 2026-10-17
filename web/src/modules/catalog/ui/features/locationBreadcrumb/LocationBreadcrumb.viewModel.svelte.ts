import { resolve } from '$app/paths'
import { page } from '$app/state'
import { createQuery } from '@tanstack/svelte-query'
import type { BreadcrumbItem, MenuEntry } from '@/shared/ui'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { catalogTreeQuery } from '../../../data'
import {
	appRefFrom,
	type CatalogTree,
	findProject,
	type ProjectNode,
	type ProjectSlug,
	type StageNode,
	type StageSlug,
	switchProjectTarget,
	switchStageTarget,
} from '../../../domain'
import { appTabHref, projectSectionHref, stageVariablesHref, tabOf } from './locationLinks'

/** Dónde se está: una app (los tabs) o las variables de un stage (sin app). */
export interface Location {
	readonly project: string
	readonly stage: string
	readonly app: string | null
}

function link(id: string, label: string, href: string, current: boolean): MenuEntry {
	return { kind: 'link', id, label, href, ...(current ? { current: true } : {}) }
}

// "logo / proyecto ⇅ / stage ⇅ / app ⇅" (`App-*.dc.html`). Cada selector abre un menú con
// los hermanos; elegir uno lleva al mismo lugar en el otro (el mismo tab, la misma app si
// existe en el otro stage). Mientras el árbol no llega, los segmentos son links.
export class LocationBreadcrumbViewModel {
	readonly #where: () => Location
	readonly #tree

	constructor(where: () => Location) {
		this.#where = where
		// El snippet de <ShellHeader> se evalúa una vez con los params de la ruta nueva al
		// salir (CLAUDE.md): sin proyecto o stage, no se pide nada.
		this.#tree = createQuery(() => ({ ...catalogTreeQuery(), enabled: this.ready }))
	}

	get ready(): boolean {
		const { project, stage, app } = this.#where()
		return Boolean(project && stage && (app === null || app))
	}

	get items(): BreadcrumbItem[] {
		const where = this.#where()
		const tree = this.#tree.data
		const node = tree ? findProject(tree, where.project) : undefined
		const stageNode = node?.stages.find((item) => item.stage.slug === where.stage)
		if (!tree || !node || !stageNode) return this.#plainItems(where)

		const items: BreadcrumbItem[] = [
			{
				kind: 'menu',
				label: where.project,
				initial: where.project.charAt(0),
				ariaLabel: m.catalog_crumb_project({ project: where.project }),
				items: this.#projectEntries(tree, where),
			},
			{
				kind: 'menu',
				label: where.stage,
				ariaLabel: m.catalog_crumb_stage({ stage: where.stage }),
				items: this.#stageEntries(node, where),
			},
		]
		if (where.app !== null) {
			items.push({
				kind: 'menu',
				label: where.app,
				ariaLabel: m.catalog_crumb_app({ app: where.app }),
				items: this.#appEntries(stageNode, where, where.app),
			})
		}
		return items
	}

	#plainItems(where: Location): BreadcrumbItem[] {
		const items: BreadcrumbItem[] = [
			{
				label: where.project,
				initial: where.project.charAt(0),
				href: projectSectionHref(where.project),
			},
			where.app === null
				? { label: where.stage }
				: { label: where.stage, href: stageVariablesHref(where.project, where.stage) },
		]
		if (where.app !== null) {
			items.push({
				label: where.app,
				href: appTabHref(appRefFrom(where.project, where.stage, where.app), ''),
			})
		}
		return items
	}

	#projectEntries(tree: CatalogTree, where: Location): MenuEntry[] {
		const tab = tabOf(page.url.pathname)
		const entries = tree.map((node) => {
			const slug = node.project.slug
			return link(
				`p-${slug}`,
				slug,
				where.app === null
					? this.#stagePageOf(node, where.stage)
					: this.#appPageOf(tree, where, where.app, slug, tab),
				slug === where.project,
			)
		})
		return [
			{ kind: 'link', id: 'projects', label: m.catalog_crumb_projects(), href: resolve('/') },
			{ kind: 'separator', id: 'sep' },
			...entries,
		]
	}

	#stageEntries(node: ProjectNode, where: Location): MenuEntry[] {
		const tab = tabOf(page.url.pathname)
		const stages = node.stages.map((stageNode) => {
			const slug = stageNode.stage.slug
			let href = stageVariablesHref(where.project, slug)
			if (where.app !== null) {
				const current = appRefFrom(where.project, where.stage, where.app)
				const target = switchStageTarget([node], current, slug as StageSlug)
				if (target) href = appTabHref(target, tab)
			}
			return link(`s-${slug}`, slug, href, slug === where.stage)
		})
		if (where.app === null) return stages
		// Desde una app, el stage también lleva a sus variables (como el link de antes).
		return [
			{
				kind: 'link',
				id: 'stage-variables',
				label: m.catalog_crumb_stage_variables({ stage: where.stage }),
				href: stageVariablesHref(where.project, where.stage),
			},
			{ kind: 'separator', id: 'sep' },
			...stages,
		]
	}

	#appEntries(stageNode: StageNode, where: Location, app: string): MenuEntry[] {
		const tab = tabOf(page.url.pathname)
		return stageNode.apps.map((item) =>
			link(
				`a-${item.slug}`,
				item.slug,
				appTabHref(appRefFrom(where.project, where.stage, item.slug), tab),
				item.slug === app,
			),
		)
	}

	// El mismo tab en la app que corresponde del otro proyecto; si no tiene apps, su
	// sección en Proyectos.
	#appPageOf(
		tree: CatalogTree,
		where: Location,
		app: string,
		project: string,
		tab: ReturnType<typeof tabOf>,
	): string {
		const current = appRefFrom(where.project, where.stage, app)
		const target = switchProjectTarget(tree, current, project as ProjectSlug)
		return target ? appTabHref(target, tab) : projectSectionHref(project)
	}

	// Las variables del stage del mismo nombre en el otro proyecto (o del primero que tenga).
	#stagePageOf(node: ProjectNode, stage: string): string {
		const target = node.stages.find((item) => item.stage.slug === stage) ?? node.stages[0]
		return target
			? stageVariablesHref(node.project.slug, target.stage.slug)
			: projectSectionHref(node.project.slug)
	}
}
