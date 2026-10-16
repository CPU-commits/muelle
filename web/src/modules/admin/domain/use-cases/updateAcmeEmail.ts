import { isEmail } from '@/shared/domain'
import type { Settings } from '../entities/settings'
import { InvalidAcmeEmail } from '../errors/adminErrors'
import type { SettingsDeps } from '../repositories/adminRepositories'

/** A dónde avisa Let's Encrypt si un certificado tiene problemas; `''` lo borra. */
export type UpdateAcmeEmail = (email: string) => Promise<Settings>

export function makeUpdateAcmeEmail({ settings }: SettingsDeps): UpdateAcmeEmail {
	return async (email) => {
		const value = email.trim().toLowerCase()
		if (value !== '' && !isEmail(value)) throw new InvalidAcmeEmail()
		if (/[{}]/.test(value)) throw new InvalidAcmeEmail('placeholder')
		return settings.patch({ acmeEmail: value })
	}
}
