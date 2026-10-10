import type { AppStatus } from '../entities/appStatus'
import type { DeployDeps } from '../repositories/deployRepository'
import type { AppRef } from '../value-objects/appRef'

export type GetAppStatus = (app: AppRef, signal?: AbortSignal) => Promise<AppStatus>

export function makeGetAppStatus({ deploys }: Pick<DeployDeps, 'deploys'>): GetAppStatus {
	return async (app, signal) => deploys.status(app, signal)
}
