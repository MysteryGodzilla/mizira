<script lang="ts">
  import type { Snippet } from "svelte";
  import type { MzConsole } from "./console.svelte";
  let { mz, id, children }: { mz: MzConsole | null; id: string; children: Snippet } = $props();

  // Hidden while the feature is off, unless "show inactive" is on; then shown dimmed with how to turn
  // it on. Nothing is removed: turning the feature on brings the card back.
  let f = $derived(mz?.feature(id) ?? null);
</script>

{#if !mz || mz.shows(id)}
  {#if f && !f.active}
    <div class="off">
      <p class="sub">{f.label} is off here — {f.how}.</p>
      {@render children()}
    </div>
  {:else}
    {@render children()}
  {/if}
{/if}

<style>
  .off { opacity: .55; }
  .off p { margin: 0 0 4px 4px; }
</style>
