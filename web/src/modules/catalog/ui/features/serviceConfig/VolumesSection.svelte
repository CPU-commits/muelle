<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Banner, Button, Card, firstError, Icon, Input } from '@/shared/ui'
	import type { ServiceConfigViewModel } from './ServiceConfig.viewModel.svelte.ts'
	import {
		addButton,
		empty,
		fieldError,
		form as formClass,
		narrow,
		volumeActionColumn,
		volumeHeader,
		volumeInputs,
		volumeList,
		volumeMountColumn,
		volumeNameColumn,
		volumeRow,
	} from './styles.css'

	let { vm, id }: { vm: ServiceConfigViewModel; id: string } = $props()

	const baseId = $props.id()
</script>

<form
	class={formClass}
	novalidate
	onsubmit={(event) => {
		event.preventDefault()
		event.stopPropagation()
		void vm.volumes.form.handleSubmit()
	}}
>
	<Card
		{id}
		title={m.catalog_config_volumes()}
		description={m.catalog_config_volumes_description()}
		note={m.catalog_config_volumes_note()}
	>
		{#if vm.volumes.saver.formError}
			<Banner tone="error" title={vm.volumes.saver.formError} />
		{/if}

		<vm.volumes.form.Field name="volumes" mode="array">
			{#snippet children(list)}
				<div class={narrow}>
					{#if list.state.value.length === 0}
						<p class={empty}>{m.catalog_config_volumes_empty()}</p>
					{:else}
						<div class={volumeHeader} aria-hidden="true">
							<span class={volumeNameColumn}>{m.catalog_config_volume_name()}</span>
							<span class={volumeMountColumn}>{m.catalog_config_volume_mount()}</span>
							<span class={volumeActionColumn}></span>
						</div>
						<ul class={volumeList}>
							{#each list.state.value, index (index)}
								{@const position = index + 1}
								<li class={volumeRow}>
									<vm.volumes.form.Field name={`volumes[${index}].name`}>
										{#snippet children(nameField)}
											<vm.volumes.form.Field name={`volumes[${index}].mount`}>
												{#snippet children(mountField)}
													{@const nameError =
														firstError(nameField.state.meta.errors) ??
														vm.volumes.saver.serverError(
															nameField.name,
															nameField.state.value,
														)}
													{@const mountError =
														firstError(mountField.state.meta.errors) ??
														vm.volumes.saver.serverError(
															mountField.name,
															mountField.state.value,
														)}
													<div class={volumeInputs}>
														<Input
															class={volumeNameColumn}
															mono
															name={nameField.name}
															aria-label={m.catalog_config_volume_name_label(
																{
																	index: position,
																},
															)}
															aria-invalid={nameError !== null}
															aria-describedby={nameError
																? `${baseId}-${index}-name`
																: undefined}
															autocomplete="off"
															autocapitalize="none"
															spellcheck={false}
															value={nameField.state.value}
															onblur={nameField.handleBlur}
															oninput={(event) =>
																nameField.handleChange(
																	event.currentTarget.value,
																)}
														/>
														<Input
															class={volumeMountColumn}
															mono
															name={mountField.name}
															placeholder="/data"
															aria-label={m.catalog_config_volume_mount_label(
																{
																	index: position,
																},
															)}
															aria-invalid={mountError !== null}
															aria-describedby={mountError
																? `${baseId}-${index}-mount`
																: undefined}
															autocomplete="off"
															autocapitalize="none"
															spellcheck={false}
															value={mountField.state.value}
															onblur={mountField.handleBlur}
															oninput={(event) =>
																mountField.handleChange(
																	event.currentTarget.value,
																)}
														/>
														<span class={volumeActionColumn}>
															<Button
																variant="ghost"
																iconOnly
																aria-label={m.catalog_config_volume_remove(
																	{
																		index: position,
																	},
																)}
																onclick={() =>
																	vm.removeVolume(index)}
															>
																<Icon name="x" />
															</Button>
														</span>
													</div>
													{#if nameError}
														<p
															id={`${baseId}-${index}-name`}
															class={fieldError}
														>
															{nameError}
														</p>
													{/if}
													{#if mountError}
														<p
															id={`${baseId}-${index}-mount`}
															class={fieldError}
														>
															{mountError}
														</p>
													{/if}
												{/snippet}
											</vm.volumes.form.Field>
										{/snippet}
									</vm.volumes.form.Field>
								</li>
							{/each}
						</ul>
					{/if}
					{#if firstError(list.state.meta.errors)}
						<p class={fieldError}>{firstError(list.state.meta.errors)}</p>
					{/if}
					<span class={addButton}>
						<Button
							variant="secondary"
							size="sm"
							disabled={!vm.canAddVolume}
							onclick={vm.addVolume}
						>
							<Icon name="plus" size={14} />
							{m.catalog_config_volume_add()}
						</Button>
					</span>
				</div>
			{/snippet}
		</vm.volumes.form.Field>

		{#snippet footer()}
			<Button type="submit" size="sm" loading={vm.volumes.saver.isPending}>
				{m.common_save()}
			</Button>
		{/snippet}
	</Card>
</form>
