import { describe, expect, it } from 'vitest'
import { APP, makeStatus } from '../../repositories/__fakes__/deployFixtures'
import { InMemoryDeployRepository } from '../../repositories/__fakes__/inMemoryDeployRepository'
import { makeGetAppStatus } from '../getAppStatus'

describe('getAppStatus', () => {
	it('devuelve el estado del repositorio', async () => {
		const repo = new InMemoryDeployRepository()
		repo.appStatus = makeStatus()

		await expect(makeGetAppStatus({ deploys: repo })(APP)).resolves.toBe(repo.appStatus)
	})
})
