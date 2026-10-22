import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

export const screen = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[20],
	minWidth: 0,
})

export const header = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'flex-end',
	justifyContent: 'space-between',
	gap: `${theme.space[12]} ${theme.space[24]}`,
})

export const headerText = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[4],
})

export const title = style({
	fontSize: theme.typography.fontSize[18],
	fontWeight: theme.typography.fontWeight.semibold,
	letterSpacing: theme.typography.letterSpacing.tight,
})

export const subtitle = style({
	maxWidth: '68ch',
	color: theme.colors.text.secondary,
})

export const createdTitle = style({
	fontSize: theme.typography.fontSize[16],
	fontWeight: theme.typography.fontWeight.semibold,
	letterSpacing: theme.typography.letterSpacing.snug,
	overflowWrap: 'anywhere',
})

export const createdText = style({
	color: theme.colors.text.secondary,
})

export const copyRow = style({
	display: 'flex',
	gap: theme.space[8],
	maxWidth: '640px',
	minWidth: 0,
})

export const copyInput = style({
	flex: '1 1 auto',
	minWidth: 0,
})

export const snippet = style({
	margin: 0,
	maxWidth: '640px',
	boxSizing: 'border-box',
	padding: `${theme.space[12]} ${theme.space[14]}`,
	border: `1px solid ${theme.colors.borders.subtle}`,
	borderRadius: theme.radius.md,
	backgroundColor: theme.colors.background.page,
	color: theme.colors.text.secondary,
	fontFamily: theme.typography.fontFamily.mono,
	fontSize: theme.typography.fontSize[12],
	lineHeight: 1.7,
	overflowX: 'auto',
	whiteSpace: 'pre',
})

// "Conectar con Claude Code": el comando y el .mcp.json, cada uno con su botón de copiar,
// separados del token y del ejemplo de Actions por una línea.
export const claude = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[10],
	minWidth: 0,
	paddingTop: theme.space[16],
	borderTop: `1px solid ${theme.colors.borders.subtle}`,
})

export const claudeTitle = style({
	fontSize: theme.typography.fontSize[14],
	fontWeight: theme.typography.fontWeight.semibold,
	letterSpacing: theme.typography.letterSpacing.snug,
})

export const claudeSubtitle = style({
	marginTop: theme.space[6],
	fontSize: theme.typography.fontSize[13],
	fontWeight: theme.typography.fontWeight.medium,
	color: theme.colors.text.primary,
})

// El bloque de código y su botón, del ancho del token de arriba: el bloque scrollea de
// costado y, en un teléfono, el botón baja para que el bloque use todo el ancho.
export const codeRow = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'flex-start',
	gap: theme.space[8],
	maxWidth: '640px',
	minWidth: 0,
})

export const codeBlock = style([
	snippet,
	{
		flex: '1 1 280px',
		minWidth: 0,
		maxWidth: 'none',
	},
])

export const group = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[10],
})

export const groupHeading = style({
	display: 'flex',
	alignItems: 'baseline',
	gap: theme.space[10],
})

export const groupTitle = style({
	fontSize: theme.typography.fontSize[16],
	fontWeight: theme.typography.fontWeight.semibold,
	letterSpacing: theme.typography.letterSpacing.snug,
})

export const groupCount = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

// El nombre (con "nuevo") y, debajo, quién lo creó.
export const nameCell = style({
	flex: '1 1 200px',
	minWidth: 0,
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[2],
})

// "nuevo" queda al lado del nombre aunque sea largo: el nombre corta, el badge no salta.
export const nameLine = style({
	display: 'flex',
	alignItems: 'flex-start',
	gap: theme.space[8],
	minWidth: 0,
	fontWeight: theme.typography.fontWeight.medium,
})

export const tokenName = style({
	minWidth: 0,
	overflowWrap: 'anywhere',
})

export const newBadge = style({
	flexShrink: 0,
})

export const creator = style({
	fontSize: theme.typography.fontSize[12],
	color: theme.colors.text.tertiary,
})

// El alcance y, si es de solo lectura, su badge al lado (o abajo, si no entra).
export const scopeCell = style({
	flex: '1 1 200px',
	minWidth: 0,
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	gap: `${theme.space[4]} ${theme.space[8]}`,
	overflowWrap: 'anywhere',
})

export const metaCell = style({
	flex: '1 1 140px',
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const expiredText = style({
	color: theme.colors.status.warnText,
})
