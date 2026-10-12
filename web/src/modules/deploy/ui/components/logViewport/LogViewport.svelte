<script lang="ts">
	import type { Snippet } from 'svelte'
	import type { Attachment } from 'svelte/attachments'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Button, Icon } from '@/shared/ui'
	import { jumpSlot, root, scroller, sizes } from './styles.css'

	// La caja con scroll de un log. Mientras `follow` es true, cada línea nueva (`count`)
	// baja hasta el final. Si quien lee sube, avisa `onFollowChange(false)` y deja de
	// bajar; volver al final (o "Ir al final") lo reactiva. El estado de `follow` es del
	// viewModel: esto solo mira el scroll.
	type Props = {
		label: string
		/** Cuántas líneas hay: cambia cuando llegan nuevas. */
		count: number
		follow: boolean
		onFollowChange: (follow: boolean) => void
		size?: 'md' | 'lg'
		children: Snippet
	}

	let { label, count, follow, onFollowChange, size = 'md', children }: Props = $props()

	/** A menos de esto del final, se considera "en el final". */
	const NEAR_BOTTOM_PX = 24

	// Corre de nuevo cada vez que cambian `count` o `follow` (un attachment es un effect).
	// Baja en el próximo frame, cuando las líneas nuevas ya están pintadas.
	const stickToBottom: Attachment<HTMLDivElement> = (element) => {
		void count
		if (!follow) return
		const frame = requestAnimationFrame(() => {
			element.scrollTop = element.scrollHeight
		})
		return () => cancelAnimationFrame(frame)
	}

	function onScroll(event: Event & { currentTarget: HTMLDivElement }): void {
		const element = event.currentTarget
		const distance = element.scrollHeight - element.scrollTop - element.clientHeight
		const atBottom = distance <= NEAR_BOTTOM_PX
		if (atBottom !== follow) onFollowChange(atBottom)
	}
</script>

<div class={root}>
	<!-- Una región con scroll tiene que poder recibir el foco para leerse con teclado. -->
	<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
	<div
		class={[scroller, sizes[size]]}
		role="region"
		aria-label={label}
		tabindex="0"
		onscroll={onScroll}
		{@attach stickToBottom}
	>
		{@render children()}
	</div>
	{#if !follow}
		<div class={jumpSlot}>
			<Button variant="secondary" size="sm" onclick={() => onFollowChange(true)}>
				<Icon name="chevronDown" size={14} />
				{m.deploy_log_jump()}
			</Button>
		</div>
	{/if}
</div>
