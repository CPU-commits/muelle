import type { Deploy } from '../entities/deploy'
import type { DeployDeps } from '../repositories/deployRepository'
import type { AppRef } from '../value-objects/appRef'
import type { DeployId } from '../value-objects/deployId'

export type GetDeploy = (app: AppRef, id: DeployId, signal?: AbortSignal) => Promise<Deploy>

export function makeGetDeploy({ deploys }: Pick<DeployDeps, 'deploys'>): GetDeploy {
	return async (app, id, signal) => deploys.get(app, id, signal)
}
