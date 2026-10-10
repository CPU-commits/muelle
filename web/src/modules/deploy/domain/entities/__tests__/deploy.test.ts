import { describe, expect, it } from 'vitest'
import { makeDeploy } from '../../repositories/__fakes__/deployFixtures'
import {
	currentDeploy,
	deployDurationMs,
	hasActiveDeploy,
	imageOf,
	latestDeploy,
	rollbackTarget,
} from '../deploy'

// El historial del diseño (App-Deploys.dc.html), desordenado a propósito.
const history = [
	makeDeploy(141, { status: 'superseded', startedAt: null, finishedAt: null }),
	makeDeploy(144, { status: 'running', finishedAt: null }),
	makeDeploy(139),
	makeDeploy(143),
	makeDeploy(142, { status: 'failed', error: 'la versión nueva no pasó el healthcheck' }),
	makeDeploy(140),
]

describe('deploy actual y rollback (spec §5.2 regla 6)', () => {
	it('el último es el más reciente, sea cual sea su estado', () => {
		expect(latestDeploy(history)?.id).toBe(144)
		expect(latestDeploy([])).toBeNull()
	})

	it('el actual es el succeeded más reciente', () => {
		expect(currentDeploy(history)?.id).toBe(143)
	})

	it('el rollback vuelve al succeeded inmediatamente anterior al actual', () => {
		expect(rollbackTarget(history)?.id).toBe(140)
	})

	it('sin un succeeded anterior no hay a dónde volver', () => {
		expect(rollbackTarget([makeDeploy(1), makeDeploy(2, { status: 'failed' })])).toBeNull()
		expect(rollbackTarget([])).toBeNull()
	})

	it('hasActiveDeploy mira si hay uno en cola o corriendo', () => {
		expect(hasActiveDeploy(history)).toBe(true)
		expect(hasActiveDeploy([makeDeploy(1)])).toBe(false)
	})
})

describe('deployDurationMs', () => {
	it('de un terminado: desde que empezó hasta que terminó', () => {
		expect(deployDurationMs(makeDeploy(1))).toBe(52_000)
	})

	it('de uno corriendo: hasta ahora', () => {
		const deploy = makeDeploy(1, { status: 'running', finishedAt: null })
		expect(deployDurationMs(deploy, new Date('2026-10-10T14:00:25Z'))).toBe(24_000)
	})

	it('uno que nunca empezó no tiene duración', () => {
		expect(deployDurationMs(makeDeploy(1, { startedAt: null }))).toBeNull()
	})
})

describe('imageOf', () => {
	it('busca la imagen del servicio en el deploy', () => {
		expect(imageOf(makeDeploy(1), 'api')?.ref).toBe('ghcr.io/auditfy/api:sha-3f9c2e1')
		expect(imageOf(makeDeploy(1), 'worker')).toBeNull()
	})
})
