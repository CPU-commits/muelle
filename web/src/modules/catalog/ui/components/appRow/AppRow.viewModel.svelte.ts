import { m } from '@/shared/ui/i18n/paraglide/messages'
import { type App, type AppContents, appContents } from '../../../domain'
import type { AppRowDeploy, AppRowDeployFactory } from './appRowDeploy'

// Una app en Proyectos. Con la Fase 2 (`deploy`, lo que pasa la ruta), el estado, la imagen
// que corre y el último deploy; sin ella, lo que sabe el catálogo: el tipo, la imagen del
// spec (o los servicios del compose) y cuándo se desplegó.
//
// El estado se pide una vez por fila (contrato §2.8) y recién cuando la fila se ve
// (`markSeen`): con muchas apps, las de abajo no piden nada hasta que se llega a ellas.
export class AppRowViewModel {
	/** La fila ya entró en la pantalla: desde ahí `deploy` puede pedir. No vuelve a false. */
	seen = $state(false)

	/** Lo de la Fase 2, o null si la ruta no lo pasa. */
	readonly deploy: AppRowDeploy | null
	readonly #contents: AppContents

	constructor(app: () => App, deploy: AppRowDeployFactory | undefined) {
		this.deploy = deploy ? deploy(app, () => this.seen) : null
		this.#contents = $derived(appContents(app()))
	}

	/** La imagen que corre, si la Fase 2 la sabe; si no, la del spec o los servicios. */
	get image(): string {
		const running = this.deploy?.image
		if (running) return running
		const contents = this.#contents
		if (contents.kind === 'image') return contents.image
		return contents.services.join(' · ') || m.catalog_app_no_services()
	}

	markSeen = (): void => {
		this.seen = true
	}
}
