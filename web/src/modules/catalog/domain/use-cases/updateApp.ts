import type { App, AppRef } from '../entities/app'
import type { AppChanges, CatalogDeps } from '../repositories/catalogRepository'
import { parseAppSpec } from '../value-objects/appSpec'
import { parseComposeYaml } from '../value-objects/composeYaml'
import { parseDeployTimeout } from '../value-objects/deployTimeout'

export interface UpdateAppInput {
	ref: AppRef
	changes: AppChanges
}

/**
 * Guarda cambios de configuración. No redespliega: el spec o el compose nuevos quedan
 * como "cambios pendientes" hasta el próximo deploy (el timeout no cuenta como cambio).
 */
export type UpdateApp = (input: UpdateAppInput) => Promise<App>

export function makeUpdateApp({ catalog }: CatalogDeps): UpdateApp {
	return async ({ ref, changes }) => {
		const valid: AppChanges = {
			...(changes.spec ? { spec: parseAppSpec(changes.spec) } : {}),
			...(changes.composeYaml === undefined
				? {}
				: { composeYaml: parseComposeYaml(changes.composeYaml) }),
			...(changes.deployTimeoutSeconds === undefined
				? {}
				: { deployTimeoutSeconds: parseDeployTimeout(changes.deployTimeoutSeconds) }),
		}
		return catalog.updateApp(ref, valid)
	}
}
