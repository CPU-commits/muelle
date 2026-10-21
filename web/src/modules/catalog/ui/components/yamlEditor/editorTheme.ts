import { HighlightStyle } from '@codemirror/language'
import { EditorView } from '@codemirror/view'
import { tags } from '@lezer/highlight'
import { theme } from '@/shared/ui/themes/theme.css'

// El tema oscuro de CodeMirror. El editor está en un shadow root (ShadowHost): ahí no
// llegan las clases de vanilla-extract, pero sí las variables del tema (`theme.*` son
// `var(--…)` de :root), así que el tema va en `EditorView.theme` y el resaltado en un
// `HighlightStyle` con colores. CodeMirror los monta como hojas construidas, que la CSP
// (`style-src 'self'`) permite. Es la excepción a "`theme` solo en `styles.css.ts`"
// (web/CLAUDE.md, CSP).

/** La clase de una línea con un error (de sintaxis o un servicio repetido). */
export const ERROR_LINE = 'cm-muelle-error-line'

export const darkTheme = EditorView.theme(
	{
		'&': {
			backgroundColor: theme.colors.background.page,
			color: theme.colors.text.primary,
			fontSize: theme.typography.fontSize[13],
		},
		'&.cm-focused': { outline: 'none' },
		'.cm-scroller': {
			fontFamily: theme.typography.fontFamily.mono,
			lineHeight: '1.75',
			minHeight: '320px',
			maxHeight: '70vh',
			overflow: 'auto',
		},
		'.cm-content': {
			padding: `${theme.space[10]} 0 ${theme.space[14]}`,
			caretColor: theme.colors.text.primary,
		},
		// Solo horizontal: un padding vertical en la línea la despega de su número.
		'.cm-line': { paddingInline: `${theme.space[4]} ${theme.space[16]}` },
		// Números de línea y puntuación con `faint`: lo más tenue que todavía se lee (≥ 4.5:1).
		'.cm-gutters': {
			backgroundColor: theme.colors.background.page,
			border: 'none',
			color: theme.colors.text.faint,
		},
		'.cm-lineNumbers .cm-gutterElement': {
			minWidth: '32px',
			padding: `0 ${theme.space[12]} 0 ${theme.space[8]}`,
			textAlign: 'right',
		},
		'.cm-activeLine': { backgroundColor: theme.colors.background.sunken },
		'.cm-activeLineGutter': {
			backgroundColor: 'transparent',
			color: theme.colors.text.secondary,
		},
		'.cm-cursor, .cm-dropCursor': { borderLeftColor: theme.colors.text.primary },
		'&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection':
			{ backgroundColor: theme.colors.selection },
		'.cm-matchingBracket, &.cm-focused .cm-matchingBracket': {
			backgroundColor: theme.colors.background.raised,
			outline: `1px solid ${theme.colors.borders.strong}`,
		},
		[`.${ERROR_LINE}`]: { backgroundColor: theme.colors.status.errorSurface },
	},
	{ dark: true },
)

export const yamlHighlight = HighlightStyle.define([
	{ tag: tags.propertyName, color: theme.colors.text.primary },
	{ tag: [tags.content, tags.attributeValue], color: theme.colors.text.secondary },
	{ tag: [tags.string, tags.special(tags.string)], color: theme.colors.status.ok },
	{ tag: tags.lineComment, color: theme.colors.text.tertiary, fontStyle: 'italic' },
	{
		tag: [tags.separator, tags.punctuation, tags.squareBracket, tags.brace, tags.meta],
		color: theme.colors.text.faint,
	},
	{ tag: [tags.labelName, tags.typeName, tags.keyword], color: theme.colors.status.warnText },
])
