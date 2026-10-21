<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Banner, Button, formatRelative, Modal, Mono } from '@/shared/ui'
	import {
		type RollbackDialogProps,
		RollbackDialogViewModel,
	} from './RollbackDialog.viewModel.svelte.ts'
	import { body, facts, note, term, value } from './styles.css'

	type Props = RollbackDialogProps & { open: boolean }

	let { open, ...props }: Props = $props()

	const vm = new RollbackDialogViewModel(() => props)
</script>

<Modal {open} title={vm.title} description={m.deploy_rollback_body()} onClose={vm.close}>
	<div class={body}>
		{#if vm.nothingToRollBack}
			<Banner
				tone="warn"
				title={m.deploy_rollback_none_title()}
				body={m.deploy_rollback_none_body()}
			/>
		{:else if vm.formError}
			<Banner tone="error" title={vm.formError} />
		{/if}

		{#if vm.target}
			<dl class={facts}>
				<dt class={term}>{m.deploy_rollback_image()}</dt>
				<dd class={value}><Mono>{vm.targetImages}</Mono></dd>
				{#if vm.targetDeployedAt}
					<dt class={term}>{m.deploy_rollback_deployed()}</dt>
					<dd class={value}>{formatRelative(vm.targetDeployedAt)}</dd>
				{/if}
				<dt class={term}>{m.deploy_rollback_config()}</dt>
				<dd class={value}>{m.deploy_rollback_config_value()}</dd>
			</dl>
		{:else if !vm.nothingToRollBack}
			<p class={note}>{m.deploy_rollback_unknown_target()}</p>
		{/if}
	</div>

	{#snippet footer()}
		<Button variant="secondary" size="sm" disabled={vm.isPending} onclick={vm.close}>
			{vm.nothingToRollBack ? m.common_close() : m.common_cancel()}
		</Button>
		{#if !vm.nothingToRollBack}
			<Button size="sm" loading={vm.isPending} onclick={() => void vm.confirm()}>
				{m.deploy_rollback_submit()}
			</Button>
		{/if}
	{/snippet}
</Modal>
