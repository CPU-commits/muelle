import type { AppStatus, ServiceStatus, ServiceTask } from '../../entities/appStatus'
import type { Deploy } from '../../entities/deploy'
import type { AppRef } from '../../value-objects/appRef'
import { asDeployId } from '../../value-objects/deployId'

// Datos de prueba para los tests de dominio y de viewModels: valores razonables por
// defecto y `overrides` para lo que importa en cada test.

export const APP: AppRef = { project: 'auditfy', stage: 'staging', app: 'api' }

/**
 * Un deploy terminado. Por defecto sigue un tag: la ref pedida de cada servicio es la de
 * `images` sin el digest (pasá `requestedImages` para uno fijado, o null para un backend
 * que no la manda).
 */
export function makeDeploy(id: number, overrides: Partial<Omit<Deploy, 'id'>> = {}): Deploy {
	const images = overrides.images ?? [
		{
			service: 'api',
			ref: 'ghcr.io/auditfy/api:sha-3f9c2e1',
			digest: `sha256:${'9b2de1f4'.repeat(8)}`,
		},
	]
	return {
		id: asDeployId(id),
		status: 'succeeded',
		images,
		requestedImages: images.map((image) => ({ ...image, digest: null })),
		trigger: 'api',
		actor: { type: 'token', name: 'github-actions' },
		error: null,
		createdAt: new Date('2026-10-10T14:00:00Z'),
		startedAt: new Date('2026-10-10T14:00:01Z'),
		finishedAt: new Date('2026-10-10T14:00:53Z'),
		supersededBy: null,
		routesWarning: false,
		...overrides,
	}
}

export function makeTask(overrides: Partial<ServiceTask> = {}): ServiceTask {
	return {
		id: 'k3x9q2m0a1b2c3d4e5f6g7h8i',
		slot: 1,
		state: 'running',
		desiredState: 'running',
		health: 'healthy',
		node: 'manager-1',
		since: new Date('2026-10-10T14:00:40Z'),
		error: null,
		cpuPercent: 1.7,
		memoryBytes: 155 * 1024 * 1024,
		...overrides,
	}
}

export function makeService(overrides: Partial<ServiceStatus> = {}): ServiceStatus {
	return {
		name: 'api',
		image: `ghcr.io/auditfy/api:sha-3f9c2e1@sha256:${'9b2de1f4'.repeat(8)}`,
		updateState: 'completed',
		desiredReplicas: 2,
		runningReplicas: 2,
		memoryLimitBytes: 512 * 1024 * 1024,
		tasks: [makeTask({ slot: 1 }), makeTask({ id: 'w81zt0c', slot: 2 })],
		...overrides,
	}
}

export function makeStatus(services: ServiceStatus[] = [makeService()]): AppStatus {
	return { services }
}
