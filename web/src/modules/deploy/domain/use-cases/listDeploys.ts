import type { Deploy } from '../entities/deploy'
import type { DeployDeps } from '../repositories/deployRepository'
import type { AppRef } from '../value-objects/appRef'

export type ListDeploys = (app: AppRef, signal?: AbortSignal) => Promise<readonly Deploy[]>

export function makeListDeploys({ deploys }: Pick<DeployDeps, 'deploys'>): ListDeploys {
	return async (app, signal) => {
		const list = await deploys.list(app, signal)
		// El panel muestra el historial del más nuevo al más viejo, venga como venga.
		return [...list].sort((a, b) => b.id - a.id)
	}
}
