import { beforeEach, describe, expect, it } from 'vitest'
import {
	InvalidCode,
	makeBeginTotpSetup,
	makeEnableTotp,
	makeGetMe,
	MalformedCode,
	NoPendingTotp,
} from '../..'
import {
	FAKE_VALID_CODE,
	InMemoryAuthRepository,
} from '../../repositories/__fakes__/inMemoryAuthRepository'

describe('enableTotp', () => {
	let auth: InMemoryAuthRepository

	beforeEach(() => {
		auth = new InMemoryAuthRepository({
			users: [{ username: 'felipe', password: 'una-contraseña-larga', totpEnabled: false }],
			require2fa: true,
		})
		auth.session = { username: 'felipe', mfaPending: false, needsTotpSetup: true }
	})

	it('confirma el secreto pendiente y deja de exigir la configuración', async () => {
		await makeBeginTotpSetup({ auth })()

		await makeEnableTotp({ auth })(FAKE_VALID_CODE)

		await expect(makeGetMe({ auth })()).resolves.toMatchObject({
			totpEnabled: true,
			needsTotpSetup: false,
		})
	})

	it('sin generar el secreto antes propaga NoPendingTotp', async () => {
		await expect(makeEnableTotp({ auth })(FAKE_VALID_CODE)).rejects.toBeInstanceOf(
			NoPendingTotp,
		)
	})

	it('propaga un código incorrecto y rechaza uno mal formado sin llamar', async () => {
		await makeBeginTotpSetup({ auth })()

		await expect(makeEnableTotp({ auth })('000000')).rejects.toBeInstanceOf(InvalidCode)
		await expect(makeEnableTotp({ auth })('abc')).rejects.toBeInstanceOf(MalformedCode)
		expect(auth.calls.enableTotp).toBe(1)
	})
})
