<script lang="ts">
	import type { DeployStatus } from '../../../domain'
	import { statusLabel } from '../labels/deployLabels'
	import DeployStatusDot from './DeployStatusDot.svelte'
	import { pill, root, textTones } from './styles.css'

	// El estado de un deploy con su punto: en una línea (`inline`) o como pastilla en el
	// encabezado del log (`pill`).
	type Props = { status: DeployStatus; variant?: 'inline' | 'pill' }

	let { status, variant = 'inline' }: Props = $props()
</script>

<span class={[root, variant === 'pill' && pill, textTones[status]]}>
	<DeployStatusDot {status} size={variant === 'pill' ? 6 : 8} />
	{statusLabel(status)}
</span>
