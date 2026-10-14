import { describe, expect, it } from 'vitest'
import { DEPLOY_STATUSES, isActiveStatus, isDeployStatus, isTerminalStatus } from '../deployStatus'

describe('estados de un deploy', () => {
	it('en cola y corriendo todavía cambian; el resto es final', () => {
		expect(DEPLOY_STATUSES.filter(isActiveStatus)).toEqual(['queued', 'running'])
		expect(DEPLOY_STATUSES.filter(isTerminalStatus)).toEqual([
			'succeeded',
			'failed',
			'superseded',
		])
	})

	it('isDeployStatus reconoce solo los cinco del spec', () => {
		expect(isDeployStatus('running')).toBe(true)
		expect(isDeployStatus('done')).toBe(false)
	})
})
