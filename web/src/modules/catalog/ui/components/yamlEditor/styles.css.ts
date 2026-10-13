import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// El editor vive en un shadow root (ver YamlEditor.svelte): la CSP del panel
// (`style-src 'self'`) bloquea el `<style>` que CodeMirror mete en el documento, y adentro
// de un shadow root usa hojas construidas (adoptedStyleSheets), que la CSP permite. Las
// reglas de la página no cruzan esa frontera, así que el tema se arma con
// `EditorView.theme` (editorExtensions.ts) a partir de estos tokens: son `var(--…)` del
// tema, y las variables CSS sí cruzan.

/** El host del shadow root: el borde de foco y el fondo, desde la página. */
export const root = style({
	minWidth: 0,
	backgroundColor: theme.colors.background.page,
	transition: `box-shadow ${theme.transition.fast}`,
	selectors: {
		'&:focus-within': { boxShadow: `inset 0 0 0 1px ${theme.colors.borders.focus}` },
	},
})

export const editorTokens = {
	background: theme.colors.background.page,
	activeLine: theme.colors.background.sunken,
	matchingBracket: theme.colors.background.raised,
	matchingBracketOutline: theme.colors.borders.strong,
	selection: theme.colors.selection,
	text: theme.colors.text.primary,
	// Números de línea y puntuación con `faint`: lo más tenue que todavía se lee (≥ 4.5:1).
	gutter: theme.colors.text.faint,
	activeGutter: theme.colors.text.secondary,
	key: theme.colors.text.primary,
	value: theme.colors.text.secondary,
	string: theme.colors.status.ok,
	comment: theme.colors.text.tertiary,
	punctuation: theme.colors.text.faint,
	special: theme.colors.status.warnText,
	errorLine: theme.colors.status.errorSurface,
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: theme.typography.fontSize[13],
	contentPadding: `${theme.space[10]} 0 ${theme.space[14]}`,
	linePadding: `${theme.space[4]} ${theme.space[16]}`,
	gutterPadding: `0 ${theme.space[12]} 0 ${theme.space[8]}`,
}
