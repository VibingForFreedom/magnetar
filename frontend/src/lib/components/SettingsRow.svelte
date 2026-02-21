<script lang="ts">
	interface Props {
		label: string;
		value: string | number | boolean;
	}

	let { label, value }: Props = $props();

	function display(v: string | number | boolean): string {
		if (typeof v === 'boolean') return v ? 'Enabled' : 'Disabled';
		return String(v);
	}

	function statusColor(v: string | number | boolean): string {
		if (typeof v === 'boolean') return v ? 'text-accent-green' : 'text-text-secondary';
		return 'text-text-primary';
	}
</script>

<div class="flex items-center justify-between py-2">
	<span class="text-sm text-text-secondary">{label}</span>
	<span class="text-sm font-medium {statusColor(value)}">{display(value)}</span>
</div>
