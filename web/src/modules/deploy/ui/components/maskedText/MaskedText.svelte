<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { srOnly } from '@/shared/ui'
	import { maskedSegments } from '../../../domain'
	import { masked } from './styles.css'

	// Una línea de log con los `***` (secretos enmascarados por el backend) marcados.
	let { text }: { text: string } = $props()

	const segments = $derived(maskedSegments(text))
</script>

{#each segments as segment, index (index)}
	{#if segment.masked}
		<span class={masked} title={m.deploy_masked()}
			><span aria-hidden="true">{segment.text}</span><span class={srOnly}
				>{m.deploy_masked()}</span
			></span
		>
	{:else}
		{segment.text}
	{/if}
{/each}
