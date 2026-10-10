import adapter from '@sveltejs/adapter-static'

/** @type {import('@sveltejs/kit').Config} */
const config = {
	kit: {
		// SPA estricta: un solo shell (`200.html`) que nginx sirve para cualquier
		// ruta, y el router resuelve del lado del cliente. No hay proceso Node en
		// produccion, asi que la capa `data` corre en un solo lado: el navegador.
		adapter: adapter({ fallback: '200.html', precompress: false, strict: false }),
		alias: {
			'@': './src',
		},
	},
	vitePlugin: {
		dynamicCompileOptions: ({ filename }) =>
			filename.includes('node_modules') ? undefined : { runes: true },
	},
}

export default config
