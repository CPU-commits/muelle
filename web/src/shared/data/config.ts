import './zodSetup'
import { z } from 'zod'
import localConfig from '../../../config/local.json'
import productionConfig from '../../../config/production.json'

// `apiUrl` vacío es lo normal en los dos entornos: el panel y la API comparten origen
// (en producción el binario de Go sirve los dos; en desarrollo Vite proxea `/api` al
// backend), así que la app llama rutas relativas y la cookie de sesión viaja sola.
// `devProxy` solo lo lee vite.config.ts: a dónde apunta ese proxy.
export const AppConfigSchema = z.object({
	env: z.enum(['local', 'production']),
	apiUrl: z.union([z.literal(''), z.url()]),
	devProxy: z.url().optional(),
})

export type AppConfig = z.infer<typeof AppConfigSchema>
export type Environment = AppConfig['env']

const rawConfigs: Record<Environment, unknown> = {
	local: localConfig,
	production: productionConfig,
}

export function resolveEnvironment(mode: string | undefined): Environment {
	return mode === 'production' ? 'production' : 'local'
}

function loadConfig(env: Environment): AppConfig {
	const parsed = AppConfigSchema.safeParse(rawConfigs[env])
	if (!parsed.success) {
		const detail = parsed.error.issues
			.map((issue) => `${issue.path.join('.')}: ${issue.message}`)
			.join('; ')
		throw new Error(`Config inválido para el entorno "${env}": ${detail}`)
	}
	return parsed.data
}

const base = loadConfig(resolveEnvironment(import.meta.env.MODE))

// VITE_API_URL solo existe para los tests de node, donde no hay un origen contra el
// que resolver una ruta relativa (ver vite.config.ts).
export const config: AppConfig = {
	...base,
	apiUrl: import.meta.env.VITE_API_URL || base.apiUrl,
}
