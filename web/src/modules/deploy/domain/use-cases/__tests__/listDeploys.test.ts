import { describe, expect, it } from 'vitest'
import { APP, makeDeploy } from '../../repositories/__fakes__/deployFixtures'
import { InMemoryDeployRepository } from '../../repositories/__fakes__/inMemoryDeployRepository'
import { makeListDeploys } from '../listDeploys'

describe('listDeploys', () => {
	it('devuelve el historial del más nuevo al más viejo', async () => {
		const repo = new InMemoryDeployRepository()
		repo.deploys = [makeDeploy(139), makeDeploy(144), makeDeploy(141)]

		const list = await makeListDeploys({ deploys: repo })(APP)

		expect(list.map((deploy) => deploy.id)).toEqual([144, 141, 139])
	})
})
