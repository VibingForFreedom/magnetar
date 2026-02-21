<script lang="ts">
	interface Props {
		data: number[];
		color?: string;
		height?: number;
	}

	let { data, color = '#6b8aaf', height = 32 }: Props = $props();

	let canvas: HTMLCanvasElement;

	$effect(() => {
		if (!canvas) return;
		const ctx = canvas.getContext('2d');
		if (!ctx) return;

		const w = canvas.width;
		const h = canvas.height;
		const dpr = window.devicePixelRatio || 1;

		canvas.width = w * dpr;
		canvas.height = h * dpr;
		ctx.scale(dpr, dpr);

		ctx.clearRect(0, 0, w, h);

		if (data.length < 2) return;

		const max = Math.max(...data, 0.01);
		const step = w / (data.length - 1);

		// Fill
		ctx.beginPath();
		ctx.moveTo(0, h);
		for (let i = 0; i < data.length; i++) {
			const x = i * step;
			const y = h - (data[i] / max) * h * 0.9;
			if (i === 0) ctx.lineTo(x, y);
			else ctx.lineTo(x, y);
		}
		ctx.lineTo(w, h);
		ctx.closePath();
		ctx.fillStyle = color + '15';
		ctx.fill();

		// Line
		ctx.beginPath();
		for (let i = 0; i < data.length; i++) {
			const x = i * step;
			const y = h - (data[i] / max) * h * 0.9;
			if (i === 0) ctx.moveTo(x, y);
			else ctx.lineTo(x, y);
		}
		ctx.strokeStyle = color;
		ctx.lineWidth = 1.5;
		ctx.stroke();

		// Reset scale for next render
		canvas.width = w;
		canvas.height = h;
	});
</script>

<canvas
	bind:this={canvas}
	width={200}
	{height}
	class="w-full"
	style="height: {height}px"
></canvas>
