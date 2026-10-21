import { describe, expect, it } from 'vitest'
import { makeDeploy } from '../../repositories/__fakes__/deployFixtures'
import {
	currentDeploy,
	deployDurationMs,
	hasActiveDeploy,
	imageOf,
	latestDeploy,
	pinnedImages,
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

describe('pinnedImages: lo que el deploy actual deja fijo en un digest (contrato §2.1)', () => {
	const digest = (n: string) => `sha256:${n.repeat(64)}`
	const image = (service: string, ref: string, n: string) => ({
		service,
		ref,
		digest: digest(n),
	})
	const tag = (service: string, ref: string) => ({ service, ref, digest: null })
	// Un compose: el rollback #145 volvió a las imágenes de #143 (api:sha-1, web:sha-1) y las
	// pidió con digest.
	const pinned = [image('api', 'x/api:sha-1', 'a'), image('web', 'x/web:sha-1', 'b')]
	const rollback = makeDeploy(145, {
		trigger: 'rollback',
		images: pinned,
		requestedImages: pinned,
	})
	const before = [
		rollback,
		makeDeploy(144, {
			images: [image('api', 'x/api:sha-2', 'c'), image('web', 'x/web:sha-2', 'd')],
		}),
		makeDeploy(143, { images: pinned }),
	]

	it('el deploy actual es el rollback: todos sus servicios, fijados por él', () => {
		expect(pinnedImages(before)).toEqual({
			rollback: { id: 145, services: ['api', 'web'] },
			others: [],
		})
	})

	it('un deploy sin cuerpo después del rollback repite sus refs con digest: siguen fijos', () => {
		const history = [makeDeploy(146, { images: pinned, requestedImages: pinned }), ...before]
		expect(pinnedImages(history)).toEqual({
			rollback: { id: 145, services: ['api', 'web'] },
			others: [],
		})
	})

	it('un override con un tag suelta ese servicio; el otro sigue fijo', () => {
		const history = [
			makeDeploy(146, {
				images: [image('api', 'x/api:sha-3', 'e'), pinned[1]!],
				requestedImages: [tag('api', 'x/api:sha-3'), pinned[1]!],
			}),
			...before,
		]
		expect(pinnedImages(history)).toEqual({
			rollback: { id: 145, services: ['web'] },
			others: [],
		})
	})

	it('un override con el mismo tag que resuelve al mismo digest ya no está fijo', () => {
		// Comparar las imágenes resueltas lo daba por fijo (el falso positivo de antes).
		const history = [
			makeDeploy(146, {
				images: pinned,
				requestedImages: [tag('api', 'x/api:sha-1'), tag('web', 'x/web:sha-1')],
			}),
			...before,
		]
		expect(pinnedImages(history)).toBeNull()
	})

	it('si el rollback que los fijó ya salió del historial, siguen fijos, sin saber por cuál', () => {
		const history = [makeDeploy(170, { images: pinned, requestedImages: pinned })]
		expect(pinnedImages(history)).toEqual({ rollback: null, others: ['api', 'web'] })
	})

	it('fijado por otro digest que el del último rollback (un override, la configuración): sin rollback', () => {
		const other = [image('api', 'x/api:sha-9', 'z'), image('web', 'x/web:sha-9', 'y')]
		const history = [makeDeploy(146, { images: other, requestedImages: other }), ...before]
		expect(pinnedImages(history)).toEqual({ rollback: null, others: ['api', 'web'] })
	})

	it('unos fijados por el rollback y otros por otra cosa: separa los dos, sin perder el rollback', () => {
		// Después del rollback, un override fijó api en otro digest; web sigue el del rollback.
		const mixed = [image('api', 'x/api:sha-9', 'z'), pinned[1]!]
		const history = [makeDeploy(146, { images: mixed, requestedImages: mixed }), ...before]
		expect(pinnedImages(history)).toEqual({
			rollback: { id: 145, services: ['web'] },
			others: ['api'],
		})
	})

	it('con todo siguiendo un tag, o sin un deploy actual, no hay nada fijo', () => {
		expect(pinnedImages([makeDeploy(146, { images: pinned }), ...before])).toBeNull()
		expect(
			pinnedImages([makeDeploy(1, { status: 'failed', requestedImages: pinned })]),
		).toBeNull()
		expect(pinnedImages([])).toBeNull()
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
