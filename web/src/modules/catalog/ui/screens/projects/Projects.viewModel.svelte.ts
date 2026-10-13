import { resolve } from '$app/paths'
import { createMutation, createQuery, useQueryClient } from '@tanstack/svelte-query'
import type { MenuEntry } from '@/shared/ui'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { catalogTreeQuery, deleteProjectMutation, deleteStageMutation } from '../../../data'
import {
	type App,
	type CatalogTree,
	countApps,
	filterCatalog,
	type Project,
	ProjectNotEmpty,
	type ProjectNode,
	type ProjectSlug,
	type StageNode,
	StageNotEmpty,
	type StageSlug,
} from '../../../domain'
import { appsCount, catalogErrorMessage, stagesCount } from '../../components/catalogMessages'

export type ProjectsDialog =
	| { readonly kind: 'newProject' }
	| { readonly kind: 'renameProject'; readonly project: Project }
	| { readonly kind: 'newStage'; readonly project: ProjectSlug }
	| { readonly kind: 'newApp'; readonly project: ProjectSlug; readonly stage: StageSlug }
	| { readonly kind: 'deleteProject'; readonly node: ProjectNode }
	| { readonly kind: 'deleteStage'; readonly project: ProjectSlug; readonly node: StageNode }

// Proyectos (`Main.dc.html`): proyectos → stages → apps, con el buscador y los diálogos
// para crear, renombrar y borrar. El estado de cada app y su último deploy son de la
// Fase 2: la pantalla los recibe como snippets (ver ProjectsScreen).
export class ProjectsViewModel {
	search = $state('')
	dialog = $state.raw<ProjectsDialog | null>(null)
	/** Por qué no se pudo borrar, adentro del diálogo (el `error` de ConfirmDialog). */
	deleteError = $state<string | null>(null)

	readonly #client = useQueryClient()
	readonly #tree = createQuery(() => catalogTreeQuery())
	readonly #deleteProject = createMutation(() => deleteProjectMutation(this.#client))
	readonly #deleteStage = createMutation(() => deleteStageMutation(this.#client))

	get isLoading(): boolean {
		return this.#tree.isPending
	}

	get loadError(): string | null {
		const error = this.#tree.error
		if (!error) return null
		return catalogErrorMessage(error)
	}

	get #all(): CatalogTree {
		return this.#tree.data ?? []
	}

	/** Lo que se muestra: el árbol filtrado por el buscador. */
	get projects(): CatalogTree {
		return filterCatalog(this.#all, this.search)
	}

	get hasProjects(): boolean {
		return this.#all.length > 0
	}

	/** Hay búsqueda y no queda nada. */
	get noMatches(): boolean {
		return this.hasProjects && this.projects.length === 0
	}

	get appsMeta(): string {
		return appsCount(countApps(this.#all))
	}

	get isDeleting(): boolean {
		return this.#deleteProject.isPending || this.#deleteStage.isPending
	}

	/**
	 * El diálogo de borrar, si está abierto. Si el árbol ya dice que no está vacío
	 * (`blocked`), no hay botón de borrar: solo la explicación y "Cerrar". Si el backend
	 * dice `not_empty` igual (otra pestaña agregó algo), el motivo queda en `deleteError`.
	 */
	get confirmation(): { title: string; body: string; blocked: boolean } | null {
		const dialog = this.dialog
		if (dialog?.kind === 'deleteProject') {
			const project = dialog.node.project.slug
			return dialog.node.stages.length > 0
				? {
						title: m.catalog_project_not_empty_title({ project }),
						body: m.catalog_project_not_empty_body(),
						blocked: true,
					}
				: {
						title: m.catalog_delete_project_title({ project }),
						body: m.catalog_delete_project_body(),
						blocked: false,
					}
		}
		if (dialog?.kind === 'deleteStage') {
			const stage = dialog.node.stage.slug
			return dialog.node.apps.length > 0
				? {
						title: m.catalog_stage_not_empty_title({ stage }),
						body: m.catalog_stage_not_empty_body(),
						blocked: true,
					}
				: {
						title: m.catalog_delete_stage_title({ project: dialog.project, stage }),
						body: m.catalog_delete_stage_body(),
						blocked: false,
					}
		}
		return null
	}

	projectMeta = (node: ProjectNode): string =>
		m.catalog_meta_join({
			first: stagesCount(node.stages.length),
			second: appsCount(countApps(node)),
		})

	appHref = (app: App): string =>
		resolve('/(app)/projects/[p]/stages/[s]/apps/[a]', {
			p: app.project,
			s: app.stage,
			a: app.slug,
		})

	stageVariablesHref = (node: StageNode): string =>
		resolve('/(app)/projects/[p]/stages/[s]/variables', {
			p: node.stage.project,
			s: node.stage.slug,
		})

	projectMenu = (node: ProjectNode): MenuEntry[] => [
		{
			kind: 'action',
			id: 'new-stage',
			label: m.catalog_project_new_stage(),
			icon: 'plus',
			onSelect: () => this.open({ kind: 'newStage', project: node.project.slug }),
		},
		{
			kind: 'action',
			id: 'rename',
			label: m.catalog_project_rename(),
			onSelect: () => this.open({ kind: 'renameProject', project: node.project }),
		},
		{ kind: 'separator', id: 'sep' },
		{
			kind: 'action',
			id: 'delete',
			label: m.catalog_project_delete(),
			icon: 'trash',
			tone: 'danger',
			onSelect: () => this.open({ kind: 'deleteProject', node }),
		},
	]

	stageMenu = (project: ProjectSlug, node: StageNode): MenuEntry[] => [
		{
			kind: 'action',
			id: 'new-app',
			label: m.catalog_stage_add_app(),
			icon: 'plus',
			onSelect: () => this.open({ kind: 'newApp', project, stage: node.stage.slug }),
		},
		{ kind: 'separator', id: 'sep' },
		{
			kind: 'action',
			id: 'delete',
			label: m.catalog_stage_delete(),
			icon: 'trash',
			tone: 'danger',
			onSelect: () => this.open({ kind: 'deleteStage', project, node }),
		},
	]

	setSearch = (value: string): void => {
		this.search = value
	}

	clearSearch = (): void => {
		this.search = ''
	}

	open = (dialog: ProjectsDialog): void => {
		this.deleteError = null
		this.dialog = dialog
	}

	openNewProject = (): void => {
		this.open({ kind: 'newProject' })
	}

	openNewApp = (project: ProjectSlug, stage: StageSlug): void => {
		this.open({ kind: 'newApp', project, stage })
	}

	openNewStage = (project: ProjectSlug): void => {
		this.open({ kind: 'newStage', project })
	}

	closeDialog = (): void => {
		if (!this.isDeleting) this.dialog = null
	}

	retry = (): void => {
		void this.#tree.refetch()
	}

	confirmDelete = async (): Promise<void> => {
		const dialog = this.dialog
		if (dialog?.kind === 'deleteProject') await this.#removeProject(dialog.node.project.slug)
		if (dialog?.kind === 'deleteStage') await this.#removeStage(dialog.project, dialog.node)
	}

	async #removeProject(project: ProjectSlug): Promise<void> {
		this.deleteError = null
		try {
			await this.#deleteProject.mutateAsync(project)
			this.dialog = null
			toast.success(m.catalog_project_deleted({ project }))
		} catch (error) {
			this.deleteError =
				error instanceof ProjectNotEmpty
					? m.catalog_project_not_empty_body()
					: catalogErrorMessage(error)
		}
	}

	async #removeStage(project: ProjectSlug, node: StageNode): Promise<void> {
		const stage = node.stage.slug
		this.deleteError = null
		try {
			await this.#deleteStage.mutateAsync({ project, stage })
			this.dialog = null
			toast.success(m.catalog_stage_deleted({ stage }))
		} catch (error) {
			this.deleteError =
				error instanceof StageNotEmpty
					? m.catalog_stage_not_empty_body()
					: catalogErrorMessage(error)
		}
	}
}
