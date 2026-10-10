import { describe, expect, it } from 'vitest'
import { appOwner, ownerLabel, ownerStage, sameOwner, stageOwner } from '../envOwner'

describe('envOwner', () => {
	it('el stage de una app y de un stage', () => {
		expect(ownerStage(appOwner('auditfy', 'staging', 'api'))).toEqual(
			stageOwner('auditfy', 'staging'),
		)
		expect(ownerStage(stageOwner('auditfy', 'staging'))).toEqual(
			stageOwner('auditfy', 'staging'),
		)
	})

	it('compara dueños por tipo y slugs', () => {
		const api = appOwner('auditfy', 'staging', 'api')
		expect(sameOwner(api, appOwner('auditfy', 'staging', 'api'))).toBe(true)
		expect(sameOwner(api, appOwner('auditfy', 'staging', 'web'))).toBe(false)
		expect(sameOwner(api, stageOwner('auditfy', 'staging'))).toBe(false)
		expect(sameOwner(stageOwner('a', 's'), stageOwner('a', 's'))).toBe(true)
		expect(sameOwner(stageOwner('a', 's'), stageOwner('a', 'p'))).toBe(false)
	})

	it('la etiqueta es la del target de auditoría del backend', () => {
		expect(ownerLabel(stageOwner('auditfy', 'staging'))).toBe('stage:auditfy/staging')
		expect(ownerLabel(appOwner('auditfy', 'staging', 'api'))).toBe('app:auditfy/staging/api')
	})
})
