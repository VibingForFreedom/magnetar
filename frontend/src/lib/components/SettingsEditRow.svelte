<script lang="ts">
	import { putSetting } from '$lib/api';
	import { Loader2 } from 'lucide-svelte';

	interface Props {
		label: string;
		settingKey: string;
		value: string | number | boolean;
		type?: 'text' | 'number' | 'boolean' | 'textarea';
		onUpdate?: (newValue: string) => void;
	}

	let { label, settingKey, value, type = 'text', onUpdate }: Props = $props();

	let editValue = $state(String(value));
	let saving = $state(false);
	let error = $state<string | null>(null);

	let dirty = $derived(editValue !== String(value));

	async function save() {
		if (saving) return;
		saving = true;
		error = null;
		try {
			await putSetting(settingKey, editValue);
			onUpdate?.(editValue);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Save failed';
		} finally {
			saving = false;
		}
	}

	async function toggleBool() {
		const newVal = value === true ? 'false' : 'true';
		saving = true;
		error = null;
		try {
			await putSetting(settingKey, newVal);
			editValue = newVal;
			onUpdate?.(newVal);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Save failed';
		} finally {
			saving = false;
		}
	}
</script>

{#if type === 'boolean'}
	<div class="flex items-center justify-between py-2">
		<span class="text-sm text-text-secondary">{label}</span>
		<div class="flex items-center gap-2">
			{#if error}
				<span class="text-xs text-accent-red">{error}</span>
			{/if}
			<button
				onclick={toggleBool}
				disabled={saving}
				class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors
					{value
						? 'bg-accent-green/15 text-accent-green hover:bg-accent-green/25'
						: 'bg-bg-tertiary text-text-secondary hover:bg-bg-tertiary/80'}
					disabled:opacity-50"
			>
				{#if saving}
					<Loader2 class="h-3 w-3 animate-spin" />
				{:else}
					{value ? 'Enabled' : 'Disabled'}
				{/if}
			</button>
		</div>
	</div>
{:else if type === 'textarea'}
	<div class="space-y-2 py-2">
		<div class="flex items-center justify-between">
			<span class="text-sm text-text-secondary">{label}</span>
			{#if error}
				<span class="text-xs text-accent-red">{error}</span>
			{/if}
		</div>
		<textarea
			bind:value={editValue}
			rows="4"
			class="w-full rounded-md border border-border bg-bg-primary px-3 py-2 text-xs font-mono text-text-primary placeholder:text-text-secondary focus:border-accent-blue focus:outline-none"
		></textarea>
		{#if dirty}
			<div class="flex justify-end">
				<button
					onclick={save}
					disabled={saving}
					class="rounded-md bg-accent-blue/15 px-3 py-1.5 text-xs font-medium text-accent-blue transition-colors hover:bg-accent-blue/25 disabled:opacity-50"
				>
					{#if saving}
						<Loader2 class="h-3 w-3 animate-spin" />
					{:else}
						Save
					{/if}
				</button>
			</div>
		{/if}
	</div>
{:else}
	<div class="flex items-center justify-between py-2">
		<span class="text-sm text-text-secondary">{label}</span>
		<div class="flex items-center gap-2">
			{#if error}
				<span class="text-xs text-accent-red">{error}</span>
			{/if}
			<input
				bind:value={editValue}
				type={type === 'number' ? 'number' : 'text'}
				class="w-32 rounded-md border border-border bg-bg-primary px-2 py-1 text-right text-sm text-text-primary focus:border-accent-blue focus:outline-none"
			/>
			{#if dirty}
				<button
					onclick={save}
					disabled={saving}
					class="rounded-md bg-accent-blue/15 px-3 py-1.5 text-xs font-medium text-accent-blue transition-colors hover:bg-accent-blue/25 disabled:opacity-50"
				>
					{#if saving}
						<Loader2 class="h-3 w-3 animate-spin" />
					{:else}
						Save
					{/if}
				</button>
			{/if}
		</div>
	</div>
{/if}
