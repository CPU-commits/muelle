import { globalStyle, style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// El tema oscuro de CodeMirror hecho con los tokens: se le da estilo a sus clases
// (`.cm-*`) desde la raíz del editor, y el resaltado usa las clases `token*` de abajo
// (HighlightStyle con `class`), así nada lleva un color suelto.

export const root = style({
	minWidth: 0,
	backgroundColor: theme.colors.background.page,
	transition: `box-shadow ${theme.transition.fast}`,
	selectors: {
		'&:focus-within': { boxShadow: `inset 0 0 0 1px ${theme.colors.borders.focus}` },
	},
})

globalStyle(`${root} .cm-editor`, {
	backgroundColor: theme.colors.background.page,
	color: theme.colors.text.primary,
	fontSize: theme.typography.fontSize[13],
})

globalStyle(`${root} .cm-editor.cm-focused`, { outline: 'none' })

globalStyle(`${root} .cm-scroller`, {
	fontFamily: theme.typography.fontFamily.mono,
	lineHeight: '1.75',
	minHeight: '320px',
	maxHeight: '70vh',
	overflow: 'auto',
})

globalStyle(`${root} .cm-content`, {
	padding: `${theme.space[10]} 0 ${theme.space[14]}`,
	caretColor: theme.colors.text.primary,
})

globalStyle(`${root} .cm-line`, { paddingInline: `${theme.space[4]} ${theme.space[16]}` })

globalStyle(`${root} .cm-gutters`, {
	backgroundColor: theme.colors.background.page,
	border: 0,
	color: theme.colors.text.disabled,
})

globalStyle(`${root} .cm-lineNumbers .cm-gutterElement`, {
	minWidth: '32px',
	padding: `0 ${theme.space[12]} 0 ${theme.space[8]}`,
	textAlign: 'right',
})

globalStyle(`${root} .cm-activeLine`, { backgroundColor: theme.colors.background.sunken })
globalStyle(`${root} .cm-activeLineGutter`, {
	backgroundColor: 'transparent',
	color: theme.colors.text.secondary,
})

globalStyle(`${root} .cm-cursor, ${root} .cm-dropCursor`, {
	borderLeftColor: theme.colors.text.primary,
})

globalStyle(
	`${root} .cm-selectionBackground, ${root} .cm-editor.cm-focused .cm-selectionBackground, ${root} .cm-content ::selection`,
	{ backgroundColor: `${theme.colors.selection} !important` },
)

globalStyle(`${root} .cm-matchingBracket`, {
	backgroundColor: theme.colors.background.raised,
	outline: `1px solid ${theme.colors.borders.strong}`,
})

// Una línea con un error de sintaxis, como las del diseño.
export const errorLine = style({
	backgroundColor: theme.colors.status.errorSurface,
})

// Resaltado de YAML (`@codemirror/lang-yaml`).
export const tokenKey = style({ color: theme.colors.text.primary })
export const tokenValue = style({ color: theme.colors.text.secondary })
export const tokenString = style({ color: theme.colors.status.ok })
export const tokenComment = style({ color: theme.colors.text.tertiary, fontStyle: 'italic' })
export const tokenPunctuation = style({ color: theme.colors.text.disabled })
export const tokenSpecial = style({ color: theme.colors.status.warnText })
