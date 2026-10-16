<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Banner, Button, Field, Input, Modal } from '@/shared/ui'
	import {
		type DeployDialogProps,
		DeployDialogViewModel,
	} from './DeployDialog.viewModel.svelte.ts'
	import { form } from './styles.css'

	type Props = DeployDialogProps & { open: boolean }

	let { open, ...props }: Props = $props()

	const vm = new DeployDialogViewModel(() => props)
	const formId = $props.id()
</script>

<Modal
	{open}
	title={m.deploy_dialog_title({ app: props.app.app })}
	description={m.deploy_dialog_body()}
	onClose={vm.close}
>
	<form
		id={formId}
		class={form}
		novalidate
		onsubmit={(event) => {
			event.preventDefault()
			void vm.submit()
		}}
	>
		{#if vm.formError}
			<Banner tone="error" title={vm.formError} />
		{/if}
		{#if vm.afterRollback}
			<Banner
				tone="info"
				title={m.deploy_dialog_after_rollback_title()}
				body={m.deploy_dialog_after_rollback_body()}
			/>
		{/if}
		{#each vm.fields as service (service)}
			<Field
				label={vm.fields.length > 1
					? m.deploy_dialog_image_service({ service })
					: m.deploy_dialog_image()}
				optional
				hint={m.deploy_dialog_image_hint()}
				error={vm.errors[service] ?? null}
			>
				{#snippet children(a11y)}
					<Input
						{...a11y}
						mono
						name={`image-${service}`}
						autocomplete="off"
						autocapitalize="none"
						spellcheck={false}
						placeholder={vm.placeholder(service)}
						value={vm.overrides[service] ?? ''}
						disabled={vm.isPending}
						oninput={(event) => vm.setOverride(service, event.currentTarget.value)}
					/>
				{/snippet}
			</Field>
		{/each}
	</form>

	{#snippet footer()}
		<Button variant="secondary" size="sm" disabled={vm.isPending} onclick={vm.close}>
			{m.common_cancel()}
		</Button>
		<Button type="submit" form={formId} size="sm" loading={vm.isPending}>
			{m.deploy_dialog_submit()}
		</Button>
	{/snippet}
</Modal>
