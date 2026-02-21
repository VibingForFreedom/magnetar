<script lang="ts">
	import type { Component } from 'svelte';

	interface Props {
		label: string;
		value: string;
		subtitle?: string;
		icon: Component;
		color?: string;
	}

	let { label, value, subtitle, icon: Icon, color = 'text-accent-blue' }: Props = $props();
</script>

<div class="rounded-lg border border-border bg-bg-secondary p-4">
	<div class="flex items-center justify-between">
		<span class="text-xs font-medium uppercase tracking-wider text-text-secondary">{label}</span>
		<Icon class="h-4 w-4 {color}" />
	</div>
	<div class="mt-2 text-2xl font-semibold tracking-tight text-text-primary">{value}</div>
	{#if subtitle}
		<div class="mt-1 text-xs text-text-secondary">{subtitle}</div>
	{/if}
</div>
