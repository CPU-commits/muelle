// See https://svelte.dev/docs/kit/types#app.d.ts
// SPA sin servidor: no hay `Locals` ni `Platform` que declarar.
declare global {
	namespace App {
		// interface Error {}
		// interface PageData {}
		// interface PageState {}
	}

	interface ImportMetaEnv {
		/** Solo para los tests de node (vite.config.ts): el origen de la API. */
		readonly VITE_API_URL?: string
		/** `1` prende MSW en el navegador (`pnpm dev:mock`, e2e). */
		readonly VITE_USE_MOCKS?: string
	}
}

export {}
