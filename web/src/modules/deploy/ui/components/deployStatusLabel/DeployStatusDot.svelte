<script lang="ts">
	import { StatusDot } from '@/shared/ui'
	import type { DeployStatus } from '../../../domain'
	import { statusTone } from '../labels/deployLabels'
	import { running, runningSmall } from './styles.css'

	// Solo el indicador de un estado: un punto, o un anillo que gira si está en curso.
	// Es decorativo: el estado va siempre también en texto.
	type Props = { status: DeployStatus; size?: 6 | 8 }

	let { status, size = 8 }: Props = $props()
</script>

{#if status === 'running'}
	<span class={[running, size === 6 && runningSmall]} aria-hidden="true"></span>
{:else}
	<StatusDot status={statusTone(status)} {size} />
{/if}
