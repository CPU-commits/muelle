import type { StatusTone } from '@/shared/ui'
import type { App } from '../../../domain'

/**
 * Lo que la Fase 2 sabe de una app en Proyectos (`Main.dc.html`): el punto y el texto del
 * estado ("Corriendo · 2/2"), la imagen que corre y el último deploy ("#143 · hace 12 min",
 * con "· falló" si falló). Lo da `AppDeployState` del módulo `deploy`, que el catálogo no
 * importa: la ruta se lo pasa a `ProjectsScreen` como `appDeploy`.
 */
export interface AppRowDeploy {
	readonly tone: StatusTone
	/** "—" si no se sabe (Docker no responde, muelle falló, todavía no se pidió). */
	readonly statusText: string
	/** Null si no se sabe: la fila muestra la imagen del spec o los servicios del compose. */
	readonly image: string | null
	readonly lastDeployText: string
	readonly lastDeployFailed: boolean
	readonly lastDeployFailedText: string
}

/**
 * Crea el estado de una fila, una vez, al montarla (puede usar `createQuery`). `enabled` es
 * false hasta que la fila entra en la pantalla: las que no se ven no piden nada.
 */
export type AppRowDeployFactory = (app: () => App, enabled: () => boolean) => AppRowDeploy
