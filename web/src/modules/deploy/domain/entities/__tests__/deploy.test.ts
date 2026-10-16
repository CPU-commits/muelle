import { describe, expect, it } from 'vitest'
import { makeDeploy } from '../../repositories/__fakes__/deployFixtures'
import {
	currentDeploy,
	deployDurationMs,
	hasActiveDeploy,
	imageOf,
	latestDeploy,
	rollbackPin,
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

describe('rollbackPin: lo que sigue fijado por un rollback (contrato §2.2)', () => {
	const digest = (n: string) => `sha256:${n.repeat(64)}`
	const image = (service: string, ref: string, n: string) => ({
		service,
		ref,
		digest: digest(n),
	})
	// Un compose: el rollback #145 volvió a las imágenes de #143 (api:sha-1, web:sha-1).
	const pinned = [image('api', 'x/api:sha-1', 'a'), image('web', 'x/web:sha-1', 'b')]
	const before = [
		makeDeploy(145, { trigger: 'rollback', images: pinned }),
		makeDeploy(144, {
			images: [image('api', 'x/api:sha-2', 'c'), image('web', 'x/web:sha-2', 'd')],
		}),
		makeDeploy(143, { images: pinned }),
	]

	it('el deploy actual es el rollback: todos sus servicios', () => {
		expect(rollbackPin(before)).toEqual({ rollback: 145, services: ['api', 'web'] })
	})

	it('un deploy sin cuerpo después del rollback repite sus refs: sigue fijado', () => {
		const history = [
			makeDeploy(147, { images: pinned }),
			makeDeploy(146, { images: pinned }),
			...before,
		]
		expect(rollbackPin(history)).toEqual({ rollback: 145, services: ['api', 'web'] })
	})

	it('un override de un servicio lo suelta; el otro sigue fijado', () => {
		const history = [
			makeDeploy(146, { images: [image('api', 'x/api:sha-3', 'e'), pinned[1]!] }),
			...before,
		]
		expect(rollbackPin(history)).toEqual({ rollback: 145, services: ['web'] })
	})

	it('con imágenes nuevas en todos, ya no hay nada fijado', () => {
		const history = [
			makeDeploy(146, {
				images: [image('api', 'x/api:sha-3', 'e'), image('web', 'x/web:sha-3', 'f')],
			}),
			...before,
		]
		expect(rollbackPin(history)).toBeNull()
	})

	it('la misma ref con otro digest (el tag se movió) no es la del rollback', () => {
		const history = [
			makeDeploy(146, {
				images: [image('api', 'x/api:sha-1', 'z'), image('web', 'x/web:sha-1', 'y')],
			}),
			...before,
		]
		expect(rollbackPin(history)).toBeNull()
	})

	it('un rollback que falló, o en curso, no fija nada', () => {
		expect(
			rollbackPin([
				makeDeploy(146, { trigger: 'rollback', status: 'failed', images: pinned }),
				makeDeploy(145, { images: pinned }),
			]),
		).toBeNull()
		expect(
			rollbackPin([
				makeDeploy(146, { trigger: 'rollback', status: 'running', images: pinned }),
				makeDeploy(145, { images: pinned }),
			]),
		).toBeNull()
	})

	it('sin rollback en el historial (o sin historial) no hay nada fijado', () => {
		expect(rollbackPin(history)).toBeNull()
		expect(rollbackPin([])).toBeNull()
	})
})

describe('deployDurationMs', () => {
	it('de un terminado: desde que empezó hasta que terminó', () => {
		expect(deployDurationMs(makeDeploy(1))).toBe(52_000)
	})

	it('de uno corriendo: hasta ahora', () => {
		const deploy = makeDeploy(1, { status: 'running', finishedAt: null })
		expect(deployDurationMs(deploy, Date.parse('2026-10-10T14:00:25Z'))).toBe(24_000)
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
