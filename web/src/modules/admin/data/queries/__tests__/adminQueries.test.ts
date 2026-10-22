import { QueryClient } from '@tanstack/svelte-query'
import { describe, expect, it, vi } from 'vitest'
import type { Settings } from '../../../domain'
import { fakeSettings } from '../../../domain/repositories/__fakes__/inMemoryAdminRepository'
import {
	addRegistryMutation,
	adminKeys,
	removeRegistryMutation,
	replaceCloudflareTokenMutation,
	setCloudflareOriginCaMutation,
	updateRegistryMutation,
	updateSecurityMutation,
} from '../adminQueries'

const SETTINGS: Settings = fakeSettings({ require2fa: true })

describe('adminQueries', () => {
	it('las mutaciones con un secreto no se pausan sin red y no quedan en el cache', () => {
		const client = new QueryClient()

		for (const options of [
			replaceCloudflareTokenMutation(client),
			addRegistryMutation(client),
			updateRegistryMutation(client),
		]) {
			expect(options).toMatchObject({ gcTime: 0, networkMode: 'always' })
		}
	})

	it('guardar ajustes pone el resultado en el cache y no pide la auditoría por su cuenta', async () => {
		const client = new QueryClient()
		const spy = vi.spyOn(client, 'invalidateQueries')

		await updateSecurityMutation(client).onSuccess(SETTINGS)

		expect(client.getQueryData(adminKeys.settings())).toEqual(SETTINGS)
		// La auditoría la refresca `refreshAuditOnWrites` mientras está montada.
		expect(spy).not.toHaveBeenCalled()
	})

	it('encender los certificados de Cloudflare pone los ajustes que vuelven en el cache', async () => {
		const client = new QueryClient()
		const options = setCloudflareOriginCaMutation(client)

		await options.onSuccess(SETTINGS)

		expect(client.getQueryData(adminKeys.settings())).toEqual(SETTINGS)
	})

	it('un cambio de registries solo recarga la lista', async () => {
		const client = new QueryClient()
		const spy = vi.spyOn(client, 'invalidateQueries')

		await removeRegistryMutation(client).onSettled()

		expect(spy.mock.calls.map(([filters]) => filters?.queryKey)).toEqual([
			adminKeys.registries(),
		])
	})
})
