import { style, styleVariants } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const page = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[20],
	minWidth: 0,
})

// Filtro, servicio, tarea, líneas iniciales, En vivo y Limpiar: todo envuelve.
export const toolbar = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	gap: theme.space[8],
})

export const search = style({ flex: '1 1 260px', minWidth: 0 })

export const select = style({ flex: '0 1 160px', minWidth: 0 })

export const status = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	gap: `${theme.space[4]} ${theme.space[12]}`,
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const box = style({
	border: `1px solid ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.lg,
	overflow: 'hidden',
})

// Las líneas se apilan desde abajo, como en una terminal.
export const lines = style({
	listStyle: 'none',
	margin: 0,
	marginTop: 'auto',
	padding: `${theme.space[8]} 0`,
})

// En un teléfono, el mensaje baja a su propia línea cuando no le quedan ~28 caracteres
// al lado de la hora, la tarea y el nivel.
export const line = style({
	display: 'flex',
	flexWrap: 'wrap',
	columnGap: theme.space[16],
	padding: `0 ${theme.space[16]}`,
	// Hasta 5000 líneas: las que están fuera de la vista no se pintan (y `auto` recuerda
	// el alto de cada una para que la vista no salte).
	contentVisibility: 'auto',
	containIntrinsicSize: 'auto 20px',
	selectors: {
		'&:hover': { backgroundColor: theme.colors.background.hover },
	},
})

export const lineError = style({
	backgroundColor: theme.colors.status.errorSunken,
})

export const time = style({
	flex: 'none',
	color: theme.colors.text.tertiary,
	fontVariantNumeric: 'tabular-nums',
})

export const task = style({
	flex: 'none',
	width: '6ch',
	color: theme.colors.text.tertiary,
	overflow: 'hidden',
	textOverflow: 'ellipsis',
})

// El nivel no va en una columna: pinta la línea (el texto, que nunca se recorta, ya lo dice).
export const levels = styleVariants({
	info: { color: theme.colors.text.primary },
	warn: { color: theme.colors.status.warnText },
	error: { color: theme.colors.status.errorText },
})

export const message = style({
	flex: '1 1 28ch',
	minWidth: 0,
	whiteSpace: 'pre-wrap',
	wordBreak: 'break-word',
})

export const placeholder = style({
	margin: 'auto',
	padding: theme.space[16],
	color: theme.colors.text.tertiary,
	fontFamily: theme.typography.fontFamily.sans,
	fontSize: theme.typography.fontSize[13],
	textAlign: 'center',
})

export const note = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})
