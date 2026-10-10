import { http } from '@/shared/data/http/client'
import { parseOrThrow } from '@/shared/data/http/parse'
import type { EnvOwner } from '../../domain'
import {
	type EnvDto,
	envDtoSchema,
	type PatchEnvPayload,
	type ReplaceEnvPayload,
} from '../dtos/envDto'

const segment = encodeURIComponent

/** `/projects/{p}/stages/{s}/env` o `/projects/{p}/stages/{s}/apps/{a}/env`. */
export function envPath(owner: EnvOwner): string {
	const stage = `/projects/${segment(owner.project)}/stages/${segment(owner.stage)}`
	return owner.kind === 'stage' ? `${stage}/env` : `${stage}/apps/${segment(owner.app)}/env`
}

export const envHttpDataSource = {
	async list(owner: EnvOwner, signal?: AbortSignal): Promise<EnvDto> {
		const payload = await http.get<unknown>(envPath(owner), signal ? { signal } : {})
		return parseOrThrow(envDtoSchema, payload, 'secrets.env')
	},

	async replace(owner: EnvOwner, body: ReplaceEnvPayload): Promise<void> {
		await http.put<unknown>(envPath(owner), { body })
	},

	async patch(owner: EnvOwner, body: PatchEnvPayload): Promise<void> {
		await http.patch<unknown>(envPath(owner), { body })
	},
}
