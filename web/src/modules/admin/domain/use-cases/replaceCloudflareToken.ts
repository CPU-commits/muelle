import { RequiredField } from '../errors/adminErrors'
import type { Settings } from '../entities/settings'
import type { SettingsDeps } from '../repositories/adminRepositories'

/** El token se guarda cifrado y no vuelve: después solo se ve si está configurado. */
export type ReplaceCloudflareToken = (token: string) => Promise<Settings>

export function makeReplaceCloudflareToken({ settings }: SettingsDeps): ReplaceCloudflareToken {
	return async (token) => {
		const value = token.trim()
		if (value === '') throw new RequiredField('cloudflareToken')
		return settings.patch({ cloudflareToken: value })
	}
}
