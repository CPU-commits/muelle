import type { DeployAccepted } from '../entities/deploy'
import { type AppKind, buildDeployRequest } from '../entities/deployRequest'
import type { DeployDeps } from '../repositories/deployRepository'
import type { AppRef } from '../value-objects/appRef'

export interface CreateDeployInput {
	readonly app: AppRef
	/** `service` o `compose` si se sabe (catálogo); si no, se deduce de los servicios. */
	readonly kind?: AppKind | null
	/** Los servicios que tiene hoy la app (del estado); vacío si nunca se desplegó. */
	readonly services: readonly string[]
	/** Imagen por servicio; vacío = redesplegar la actual. */
	readonly overrides: Readonly<Record<string, string>>
}

export type CreateDeploy = (input: CreateDeployInput) => Promise<DeployAccepted>

/** "Desplegar": encola un deploy con la configuración actual (spec §7, paso 1). */
export function makeCreateDeploy({ deploys }: Pick<DeployDeps, 'deploys'>): CreateDeploy {
	return async (input) => {
		const request = buildDeployRequest({
			app: input.app.app,
			kind: input.kind ?? null,
			services: input.services,
			overrides: input.overrides,
		})
		return deploys.create(input.app, request)
	}
}
