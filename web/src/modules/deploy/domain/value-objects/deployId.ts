import type { Brand } from '@/shared/domain'

// El id de un deploy es el autoincremental de la tabla `deploys` (spec §5.1): el panel lo
// muestra como "#144".
export type DeployId = Brand<number, 'DeployId'>

export function asDeployId(raw: number): DeployId {
	return raw as DeployId
}

/** El `?deploy=144` de la URL de Deploys. Cualquier otra cosa es "ninguno". */
export function parseDeployId(raw: string | null | undefined): DeployId | null {
	if (!raw || !/^[1-9]\d{0,15}$/.test(raw)) return null
	const value = Number(raw)
	return Number.isSafeInteger(value) ? asDeployId(value) : null
}
