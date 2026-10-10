import { parseEmail } from '@/shared/domain'
import type { Settings } from '../entities/settings'
import type { SettingsDeps } from '../repositories/adminRepositories'

/** A dónde avisa Let's Encrypt si un certificado tiene problemas. */
export type UpdateAcmeEmail = (email: string) => Promise<Settings>

export function makeUpdateAcmeEmail({ settings }: SettingsDeps): UpdateAcmeEmail {
	return async (email) => settings.patch({ acmeEmail: parseEmail(email, 'acmeEmail') })
}
