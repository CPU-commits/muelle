import { describe, expect, it } from 'vitest'
import { InvalidCidr, makeUpdateSecurity, TotpRequiredFirst, WouldLockOut } from '../..'
import { InMemorySettingsRepository } from '../../repositories/__fakes__/inMemoryAdminRepository'

describe('updateSecurity', () => {
	it('manda solo lo que cambió', async () => {
		const settings = new InMemorySettingsRepository()
		const update = makeUpdateSecurity({ settings })

		await update({ require2fa: true })
		await update({ allowedCidrs: '198.51.100.0/24\n\n203.0.113.0/24' })

		expect(settings.patches).toEqual([
			{ require2fa: true },
			{ panelAllowedCidrs: ['198.51.100.0/24', '203.0.113.0/24'] },
		])
		expect(settings.settings.require2fa).toBe(true)
		expect(settings.settings.panelAccess?.allowedCidrs).toHaveLength(2)
	})

	it('sin el propio 2FA no se puede exigir', async () => {
		const settings = new InMemorySettingsRepository({ sessionHasTotp: false })

		await expect(makeUpdateSecurity({ settings })({ require2fa: true })).rejects.toBeInstanceOf(
			TotpRequiredFirst,
		)
		expect(settings.settings.require2fa).toBe(false)
	})

	it('apagarlo no exige nada', async () => {
		const settings = new InMemorySettingsRepository({ sessionHasTotp: false })

		await expect(
			makeUpdateSecurity({ settings })({ require2fa: false }),
		).resolves.toMatchObject({ require2fa: false })
	})

	it('una lista que deja fuera la IP de la sesión es WouldLockOut, salvo confirmándola', async () => {
		const settings = new InMemorySettingsRepository()
		const update = makeUpdateSecurity({ settings })

		await expect(update({ allowedCidrs: '10.0.0.0/8' })).rejects.toBeInstanceOf(WouldLockOut)
		expect(settings.settings.panelAccess?.allowedCidrs).toEqual([])

		await update({ allowedCidrs: '10.0.0.0/8', confirmLockout: true })
		expect(settings.patches.at(-1)).toEqual({
			panelAllowedCidrs: ['10.0.0.0/8'],
			confirmLockout: true,
		})
		expect(settings.settings.panelAccess?.allowedCidrs).toEqual(['10.0.0.0/8'])
	})

	it('una IP mal escrita no sale del panel', async () => {
		const settings = new InMemorySettingsRepository()

		await expect(
			makeUpdateSecurity({ settings })({ allowedCidrs: '10.0.0/8' }),
		).rejects.toBeInstanceOf(InvalidCidr)
		expect(settings.patches).toEqual([])
	})
})
