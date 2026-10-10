import type { DeployAccepted } from '../entities/deploy'
import type { DeployDeps } from '../repositories/deployRepository'
import type { AppRef } from '../value-objects/appRef'

export type RollbackDeploy = (app: AppRef) => Promise<DeployAccepted>

/**
 * Vuelve a las imágenes del `succeeded` anterior al actual, con la configuración actual
 * (spec §5.2 regla 6). Sin anterior, rechaza con `NoPreviousDeploy`.
 */
export function makeRollbackDeploy({ deploys }: Pick<DeployDeps, 'deploys'>): RollbackDeploy {
	return async (app) => deploys.rollback(app)
}
