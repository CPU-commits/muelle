type ScreenModule = typeof import('./AppConfigScreen.svelte')

/** La pantalla de verdad (formularios y editor), en su propio chunk. */
export const loadAppConfigScreen = (): Promise<ScreenModule> => import('./AppConfigScreen.svelte')

const reloadPage = (): void => window.location.reload()

/**
 * Baja el chunk de Configuración. Si no baja (sin red, un deploy nuevo que borró el
 * anterior), "Reintentar" recarga la página: el navegador guarda el `import()` fallido de
 * esa URL mientras dure la página (Chromium no lo vuelve a pedir), así que repetirlo no
 * alcanza. Lo comprobé contra el backend real, con el chunk cortado una vez.
 */
export class AppConfigLoaderViewModel {
	readonly screen: Promise<ScreenModule>
	readonly #reload: () => void

	constructor(load: () => Promise<ScreenModule> = loadAppConfigScreen, reload = reloadPage) {
		this.screen = load()
		this.#reload = reload
	}

	retry = (): void => {
		this.#reload()
	}
}
