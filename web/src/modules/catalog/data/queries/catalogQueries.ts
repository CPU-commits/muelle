import { type QueryClient, type QueryKey, queryOptions } from '@tanstack/svelte-query'
import { catalogUseCases } from '../../container'
import {
	type App,
	type AppRef,
	appRefOf,
	appSummaryOf,
	type CreateAppInput,
	type CreateProjectInput,
	type CreateStageInput,
	type DeleteStageInput,
	type ProjectSlug,
	type RenameProjectInput,
	serviceTargets,
	type UpdateAppInput,
} from '../../domain'

export const catalogKeys = {
	all: ['catalog'] as QueryKey,
	tree: (): QueryKey => ['catalog', 'tree'],
	apps: (): QueryKey => ['catalog', 'app'],
	app: (ref: AppRef): QueryKey => ['catalog', 'app', ref.project, ref.stage, ref.app],
}

/** Proyectos → stages → apps. Lo usan Proyectos y los selectores del breadcrumb. */
export function catalogTreeQuery() {
	return queryOptions({
		queryKey: catalogKeys.tree(),
		queryFn: ({ signal }) => catalogUseCases.listCatalog(signal),
	})
}

/** Una app: el header, Configuración y (en F2) Resumen la leen del mismo cache. */
export function appQuery(ref: AppRef) {
	return queryOptions({
		queryKey: catalogKeys.app(ref),
		queryFn: ({ signal }) => catalogUseCases.getApp(ref, signal),
	})
}

/**
 * Lo que va en el encabezado de una pantalla de la app (`AppIdentity`). Comparte el cache
 * de `appQuery`: no hace otro pedido.
 */
export function appSummaryQuery(ref: AppRef) {
	return queryOptions({ ...appQuery(ref), select: appSummaryOf })
}

/**
 * Los destinos que el formulario de un dominio ofrece (`servicio:puerto`, contrato §2.12).
 * Comparte el cache de `appQuery`: guardar el compose o el spec los actualiza.
 */
export function appTargetsQuery(ref: AppRef) {
	return queryOptions({ ...appQuery(ref), select: serviceTargets })
}

// Crear o borrar cambia el árbol entero; cambiar una app, además, su entrada del cache.
function refreshTree(client: QueryClient): Promise<void> {
	return client.invalidateQueries({ queryKey: catalogKeys.tree() })
}

function rememberApp(client: QueryClient, app: App): void {
	client.setQueryData(catalogKeys.app(appRefOf(app)), app)
}

export function createProjectMutation(client: QueryClient) {
	return {
		mutationFn: (input: CreateProjectInput) => catalogUseCases.createProject(input),
		onSuccess: () => refreshTree(client),
	}
}

export function renameProjectMutation(client: QueryClient) {
	return {
		mutationFn: (input: RenameProjectInput) => catalogUseCases.renameProject(input),
		onSuccess: () => refreshTree(client),
	}
}

export function deleteProjectMutation(client: QueryClient) {
	return {
		mutationFn: (project: ProjectSlug) => catalogUseCases.deleteProject(project),
		onSuccess: () => refreshTree(client),
	}
}

export function createStageMutation(client: QueryClient) {
	return {
		mutationFn: (input: CreateStageInput) => catalogUseCases.createStage(input),
		onSuccess: () => refreshTree(client),
	}
}

export function deleteStageMutation(client: QueryClient) {
	return {
		mutationFn: (input: DeleteStageInput) => catalogUseCases.deleteStage(input),
		onSuccess: () => refreshTree(client),
	}
}

export function createAppMutation(client: QueryClient) {
	return {
		mutationFn: (input: CreateAppInput) => catalogUseCases.createApp(input),
		onSuccess: async (app: App) => {
			rememberApp(client, app)
			await refreshTree(client)
		},
	}
}

export function updateAppMutation(client: QueryClient) {
	return {
		mutationFn: (input: UpdateAppInput) => catalogUseCases.updateApp(input),
		onSuccess: async (app: App) => {
			rememberApp(client, app)
			await refreshTree(client)
		},
	}
}

export function deleteAppMutation(client: QueryClient) {
	return {
		mutationFn: (ref: AppRef) => catalogUseCases.deleteApp(ref),
		onSuccess: async (_: void, ref: AppRef) => {
			client.removeQueries({ queryKey: catalogKeys.app(ref) })
			await refreshTree(client)
		},
	}
}
