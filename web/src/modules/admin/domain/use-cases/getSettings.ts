import type { Settings } from '../entities/settings'
import type { SettingsDeps } from '../repositories/adminRepositories'

export type GetSettings = (signal?: AbortSignal) => Promise<Settings>

export function makeGetSettings({ settings }: SettingsDeps): GetSettings {
	return async (signal) => settings.get(signal)
}
