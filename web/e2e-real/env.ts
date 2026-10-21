// Lo que comparten la config (playwright.real.config.ts), globalSetup.ts y los tests de
// `pnpm test:e2e:real`. Todo lo de Docker que crea la corrida empieza con PREFIX (it7, la
// línea de este e2e entre los de integración: it2 deploy, it3 ingress, it5 install, it6, it9
// container), y los puertos van del 24000 al 24999.

export const PREFIX = 'it7'

const PORT_MIN = 24000
const PORT_MAX = 24999

function realPort(): number {
	const raw = process.env.E2E_REAL_PORT
	if (raw === undefined || raw === '') return 24080
	const value = Number(raw)
	if (!Number.isInteger(value) || value < PORT_MIN || value > PORT_MAX) {
		throw new Error(
			`E2E_REAL_PORT tiene que ser un puerto entre ${PORT_MIN} y ${PORT_MAX}, no "${raw}"`,
		)
	}
	return value
}

/** Donde escucha `muelle serve` (MUELLE_LISTEN) y su MUELLE_PANEL_ORIGIN. */
export const REAL_PORT = realPort()
export const REAL_URL = `http://127.0.0.1:${REAL_PORT}`

/** Lo que globalSetup.ts deja en el entorno de los tests. */
export interface RealRun {
	/** 4 hex al azar: los nombres de esta corrida. */
	id: string
	/** El slug del proyecto, it7p<id>: el prefijo de todo lo de Docker (proyecto_stage…). */
	project: string
	username: string
	/** El link de setup que muelle imprimió en el log al arrancar. */
	setupUrl: string
}

const ENV = 'MUELLE_E2E_REAL_RUN'

export function publishRun(run: RealRun): void {
	process.env[ENV] = JSON.stringify(run)
}

export function realRun(): RealRun {
	const raw = process.env[ENV]
	if (!raw) throw new Error(`falta ${ENV}: corre los tests con pnpm test:e2e:real`)
	return JSON.parse(raw) as RealRun
}
