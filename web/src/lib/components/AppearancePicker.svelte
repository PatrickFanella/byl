<script lang="ts">
	import { appearance } from '#lib/theme.ts';
</script>

<label class="flex flex-wrap items-center gap-3 text-sm font-bold">
	Appearance
	<select
		aria-label="Appearance"
		bind:value={$appearance}
		class="rounded border border-byl-border-control bg-byl-background px-3 py-2 text-byl-text"
	>
		<option value="system">System</option>
		<option value="light">Light</option>
		<option value="dark">Dark</option>
	</select>
</label>
