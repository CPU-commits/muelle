<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Button, Card, ConfirmDialog } from '@/shared/ui'
	import type { AppRef } from '../../../domain'
	import { DeleteAppViewModel } from './DeleteApp.viewModel.svelte.ts'

	let { appRef, id }: { appRef: AppRef; id: string } = $props()

	const vm = new DeleteAppViewModel(() => appRef)
</script>

<Card
	{id}
	tone="danger"
	title={m.catalog_delete_app_title()}
	description={m.catalog_delete_app_description({ app: vm.app, stack: vm.stackName })}
>
	{#snippet footer()}
		<Button variant="danger" size="sm" onclick={vm.ask}>{m.common_delete()}</Button>
	{/snippet}
</Card>

<ConfirmDialog
	open={vm.confirming}
	title={m.catalog_delete_app_confirm_title({ app: vm.app })}
	body={m.catalog_delete_app_confirm_body()}
	confirmLabel={m.common_delete()}
	cancelLabel={m.common_cancel()}
	loading={vm.isPending}
	onConfirm={vm.confirm}
	onCancel={vm.cancel}
/>
