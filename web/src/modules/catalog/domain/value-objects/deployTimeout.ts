import { InvalidDeployTimeout } from '../errors/catalogErrors'

// `entity.ValidateDeployTimeout`: cuánto espera muelle a que el servicio converja.
export const DEPLOY_TIMEOUT = { min: 30, max: 3600, default: 300 } as const

export function isValidDeployTimeout(seconds: number): boolean {
	return (
		Number.isInteger(seconds) && seconds >= DEPLOY_TIMEOUT.min && seconds <= DEPLOY_TIMEOUT.max
	)
}

export function parseDeployTimeout(seconds: number): number {
	if (!isValidDeployTimeout(seconds)) throw new InvalidDeployTimeout()
	return seconds
}
