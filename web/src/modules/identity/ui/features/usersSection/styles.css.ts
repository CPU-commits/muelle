import { style } from '@vanilla-extract/css'
import { theme } from '@/shared/ui/themes/theme.css'

// Las secciones de lista de Ajustes (Usuarios, Registries, Auditoría): título y texto a la
// izquierda, el botón a la derecha, y la lista debajo.
export const section = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[10],
	minWidth: 0,
})

export const heading = style({
	display: 'flex',
	flexWrap: 'wrap',
	alignItems: 'center',
	justifyContent: 'space-between',
	gap: `${theme.space[8]} ${theme.space[16]}`,
})

export const headingText = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[2],
})

export const title = style({
	fontSize: theme.typography.fontSize[16],
	fontWeight: theme.typography.fontWeight.semibold,
	letterSpacing: theme.typography.letterSpacing.snug,
})

export const subtitle = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

export const nameCell = style({
	flex: '1 1 200px',
	minWidth: 0,
	display: 'flex',
	alignItems: 'center',
	gap: theme.space[10],
})

export const name = style({
	fontWeight: theme.typography.fontWeight.medium,
	overflowWrap: 'anywhere',
})

export const you = style({
	fontSize: theme.typography.fontSize[12],
	color: theme.colors.text.tertiary,
})

export const totpCell = style({
	flex: '1 1 120px',
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.secondary,
})

export const sinceCell = style({
	flex: '1 1 180px',
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})

// El lugar del menú, también en la fila propia, para que las columnas queden alineadas.
export const actionsCell = style({
	flex: '0 0 32px',
	display: 'flex',
	justifyContent: 'flex-end',
})

export const inviteBody = style({
	display: 'flex',
	flexDirection: 'column',
	gap: theme.space[12],
})

export const copyRow = style({
	display: 'flex',
	gap: theme.space[8],
	minWidth: 0,
})

export const copyInput = style({
	flex: '1 1 auto',
	minWidth: 0,
})

export const note = style({
	fontSize: theme.typography.fontSize[13],
	color: theme.colors.text.tertiary,
})
