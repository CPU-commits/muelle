import { type QueryClient, type QueryKey, queryOptions } from '@tanstack/svelte-query'
import { secretsUseCases } from '../../container'
import type { EnvOwner, PatchEnvInput, ReplaceEnvInput } from '../../domain'

export const secretsKeys = {
	all: ['secrets'] as QueryKey,
	stage: (project: string, stage: string): QueryKey => ['secrets', 'env', project, stage],
	owner: (owner: EnvOwner): QueryKey =>
		owner.kind === 'stage'
			? secretsKeys.stage(owner.project, owner.stage)
			: [...secretsKeys.stage(owner.project, owner.stage), 'apps', owner.app],
}

/** Las variables de un stage o una app (las secretas sin valor). */
export function envQuery(owner: EnvOwner) {
	return queryOptions({
		queryKey: secretsKeys.owner(owner),
		queryFn: ({ signal }) => secretsUseCases.getEnv(owner, signal),
	})
}

// `exact`: la key de un stage es prefijo de las de sus apps, y las variables propias de
// cada app no cambian cuando cambian las del stage. Se espera el refetch: cuando la
// mutación termina, la lista ya muestra lo guardado.
function refreshOwner(client: QueryClient, owner: EnvOwner): Promise<void> {
	return client.invalidateQueries({ queryKey: secretsKeys.owner(owner), exact: true })
}

// Las variables de una mutación llevan valores en claro (también secretos): `gcTime: 0`
// las saca del MutationCache apenas nadie la observa, en vez de 5 minutos después.
const FORGET_VALUES = { gcTime: 0 } as const

/**
 * Agregar, editar, borrar o importar. El backend marca con cambios pendientes a la app (o
 * a todas las apps del stage): las queries de catalog que lo muestran las marca viejas el
 * `EnvEditorViewModel` (data no importa otro módulo; la UI sí, por el barrel).
 */
export function patchEnvMutation(client: QueryClient) {
	return {
		...FORGET_VALUES,
		mutationFn: (input: PatchEnvInput) => secretsUseCases.patchEnv(input),
		onSuccess: (_: void, input: PatchEnvInput) => refreshOwner(client, input.owner),
	}
}

export function replaceEnvMutation(client: QueryClient) {
	return {
		...FORGET_VALUES,
		mutationFn: (input: ReplaceEnvInput) => secretsUseCases.replaceEnv(input),
		onSuccess: (_: void, input: ReplaceEnvInput) => refreshOwner(client, input.owner),
	}
}
