import { describe, expect, it } from 'vitest'
import { sameApp, stackName, stageNetwork, swarmServiceName } from '../appRef'

const ref = { project: 'auditfy', stage: 'staging', app: 'api' }

describe('nombres de Swarm de una app (spec §5.2)', () => {
	it('el stack es proyecto_stage_app', () => {
		expect(stackName(ref)).toBe('auditfy_staging_api')
	})

	it('la red es la del stage', () => {
		expect(stageNetwork(ref)).toBe('auditfy_staging')
	})

	it('el servicio es stack_servicio', () => {
		expect(swarmServiceName(ref, 'api')).toBe('auditfy_staging_api_api')
	})

	it('sameApp compara los tres slugs', () => {
		expect(sameApp(ref, { ...ref })).toBe(true)
		expect(sameApp(ref, { ...ref, stage: 'production' })).toBe(false)
	})
})
