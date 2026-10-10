import { describe, expect, it } from 'vitest'
import { makeService, makeStatus, makeTask } from '../../repositories/__fakes__/deployFixtures'
import {
	allTasks,
	appHealth,
	replicaCount,
	serviceHealth,
	serviceNodes,
	shortTaskId,
	taskLabel,
	weakestService,
} from '../appStatus'

describe('appHealth', () => {
	it('sin servicios en Swarm, nunca se desplegó', () => {
		expect(appHealth(makeStatus([]))).toBe('not_deployed')
	})

	it('todas las réplicas corriendo es running', () => {
		expect(appHealth(makeStatus())).toBe('running')
	})

	it('falta alguna réplica: degraded', () => {
		expect(appHealth(makeStatus([makeService({ runningReplicas: 1 })]))).toBe('degraded')
	})

	it('un servicio caído y otro bien también es degraded', () => {
		const status = makeStatus([
			makeService({ name: 'gateway', desiredReplicas: 1, runningReplicas: 0 }),
			makeService({ name: 'runner', desiredReplicas: 1, runningReplicas: 1 }),
		])
		expect(appHealth(status)).toBe('degraded')
	})

	it('nada corriendo: down', () => {
		const status = makeStatus([
			makeService({ name: 'gateway', desiredReplicas: 1, runningReplicas: 0 }),
			makeService({ name: 'runner', desiredReplicas: 2, runningReplicas: 0 }),
		])
		expect(appHealth(status)).toBe('down')
	})

	it('todo en 0 réplicas a propósito: stopped; uno en 0 no cuenta', () => {
		expect(
			appHealth(makeStatus([makeService({ desiredReplicas: 0, runningReplicas: 0 })])),
		).toBe('stopped')
		const mixed = makeStatus([
			makeService({ name: 'cron', desiredReplicas: 0, runningReplicas: 0 }),
			makeService({ name: 'web' }),
		])
		expect(appHealth(mixed)).toBe('running')
	})

	it('serviceHealth evalúa un servicio solo', () => {
		expect(serviceHealth(makeService({ runningReplicas: 0 }))).toBe('down')
	})
})

describe('réplicas y servicios', () => {
	const status = makeStatus([
		makeService({ name: 'gateway', desiredReplicas: 1, runningReplicas: 0 }),
		makeService({ name: 'runner', desiredReplicas: 2, runningReplicas: 1 }),
		makeService({ name: 'scheduler', desiredReplicas: 1, runningReplicas: 1 }),
	])

	it('replicaCount suma toda la app', () => {
		expect(replicaCount(status)).toEqual({ running: 2, desired: 4 })
	})

	it('weakestService es el que más le falta', () => {
		expect(weakestService(status)?.name).toBe('gateway')
		expect(weakestService(makeStatus())).toBeNull()
	})

	it('serviceNodes lista los nodos de las tareas vigentes', () => {
		const service = makeService({
			tasks: [
				makeTask({ node: 'manager-1' }),
				makeTask({ node: 'manager-1', slot: 2 }),
				makeTask({ node: 'worker-2', desiredState: 'shutdown', state: 'shutdown' }),
			],
		})
		expect(serviceNodes(service)).toEqual(['manager-1'])
	})
})

describe('tareas', () => {
	it('taskLabel es servicio.slot, o el servicio si es global', () => {
		expect(taskLabel('api', makeTask({ slot: 2 }))).toBe('api.2')
		expect(taskLabel('api', makeTask({ slot: null }))).toBe('api')
	})

	it('shortTaskId deja 7 caracteres', () => {
		expect(shortTaskId('k3x9q2m0a1b2c3d4')).toBe('k3x9q2m')
	})

	it('allTasks pone las vigentes primero y después las más recientes', () => {
		const old = makeTask({
			id: 'old',
			state: 'failed',
			desiredState: 'shutdown',
			since: new Date('2026-10-10T13:00:00Z'),
		})
		const shutdown = makeTask({
			id: 'shutdown',
			state: 'shutdown',
			desiredState: 'shutdown',
			since: new Date('2026-10-10T14:00:00Z'),
		})
		const running = makeTask({ id: 'running', since: new Date('2026-10-10T12:00:00Z') })
		const rows = allTasks(makeStatus([makeService({ tasks: [old, shutdown, running] })]))
		expect(rows.map((row) => row.task.id)).toEqual(['running', 'shutdown', 'old'])
		expect(rows[0]?.service).toBe('api')
	})
})
