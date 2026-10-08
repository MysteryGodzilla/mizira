<script lang="ts">
  import type { MzConsole } from "./console.svelte";
  let { mz }: { mz: MzConsole } = $props();

  let consoleOnly = $derived(mz.offline.length === 1 && mz.offline[0] === "started console-only");
</script>

{#if mz.offline.length}
  <div class="offline" class:quiet={consoleOnly} role="alert">
    {#if consoleOnly}
      <p><strong>Console only.</strong> She isn't on IRC: look things over, then restart her normally.</p>
    {:else}
      <p><strong>Mizira didn't join IRC.</strong> Fix this, then restart her:</p>
      <ul>{#each mz.offline as reason (reason)}<li><code>{reason}</code></li>{/each}</ul>
      <p class="sub">A missing setting goes under <code>env:</code> in config.yml. Or switch the tool off on
        <a href="#tools">Tools</a> and restart.</p>
    {/if}
  </div>
{/if}

<style>
  .offline { font-size: 13px; border: 2px solid var(--bad); border-radius: 8px; padding: 6px 10px; margin: -6px 0 14px;
    color: var(--bad); overflow-wrap: anywhere; }
  .offline.quiet { border-width: 1px; }
  .offline p { margin: 2px 0 6px; }
  .offline a { color: var(--bad); }
  ul { margin: 4px 0 6px; padding-left: 18px; }
  code { color: var(--fg); }
</style>
