<script lang="ts">
	import { Search } from 'lucide-svelte';

	interface Props {
		value: string;
		onInput: (value: string) => void;
		placeholder?: string;
	}

	let { value, onInput, placeholder = 'Search torrents...' }: Props = $props();

	function handleInput(e: Event) {
		const target = e.target as HTMLInputElement;
		onInput(target.value);
	}
</script>

<div class="relative">
	<Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-secondary" />
	<input
		type="text"
		{value}
		oninput={handleInput}
		{placeholder}
		class="w-full rounded-md border border-border bg-bg-tertiary py-2 pl-10 pr-4 text-sm text-text-primary placeholder:text-text-secondary/60 focus:border-accent-blue focus:outline-none"
	/>
</div>
