import { globalStyle } from '@vanilla-extract/css'
import { theme } from './theme.css'

globalStyle(':root', {
	colorScheme: 'dark',
})

globalStyle('*, *::before, *::after', {
	boxSizing: 'border-box',
	WebkitTapHighlightColor: 'transparent',
})

globalStyle('html, body', {
	margin: 0,
	padding: 0,
	minHeight: '100vh',
})

// El contenedor de %sveltekit.body% en app.html: no arma caja propia. Va acá y no como
// `style="display: contents"` en el HTML porque la CSP del panel no deja estilos en línea.
globalStyle('[data-app-root]', { display: 'contents' })

globalStyle('body', {
	fontFamily: theme.typography.fontFamily.sans,
	fontSize: theme.typography.fontSize[14],
	lineHeight: theme.typography.lineHeight.normal,
	color: theme.colors.text.primary,
	backgroundColor: theme.colors.background.page,
	WebkitFontSmoothing: 'antialiased',
	MozOsxFontSmoothing: 'grayscale',
	scrollbarColor: `${theme.colors.borders.control} transparent`,
})

globalStyle('h1, h2, h3, h4, p, dl, dd, figure, fieldset, legend', { margin: 0, padding: 0 })
globalStyle('fieldset', { border: 0, minWidth: 0 })

globalStyle('a', { color: 'inherit', textDecoration: 'none' })

globalStyle('button, input, select, textarea', {
	font: 'inherit',
	color: 'inherit',
})

globalStyle('button', {
	cursor: 'pointer',
	border: 'none',
	background: 'none',
	padding: 0,
})

// El anillo de foco del diseño: 2px claros con 2px de separación, solo con teclado.
globalStyle(':focus-visible', {
	outline: `2px solid ${theme.colors.borders.outline}`,
	outlineOffset: '2px',
})

globalStyle('::selection', {
	backgroundColor: theme.colors.selection,
})

globalStyle('code, kbd, samp, pre', {
	fontFamily: theme.typography.fontFamily.mono,
})
