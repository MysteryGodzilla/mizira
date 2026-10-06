<script lang="ts">
  import type { MzConsole } from "./console.svelte";
  import type { ExportResult } from "./types";
  let { mz }: { mz: MzConsole } = $props();

  let exported = $state<ExportResult | null>(null);

  function exportFile() {
    let r: ExportResult | null = null;
    void mz.act(async () => { r = await mz.api.exportSettings(); }, "exported").then((ok) => { if (ok) exported = r; });
  }

  function resetAll() {
    if (!confirm("Reset all?\n\nEvery setting changed here or with ~set goes back to config.yml now, and tool switches too. " +
      "Changed lists (admins, screening, bots) go back at the next restart. A pause or stop is kept.\n\n" +
      "If you want to keep them, Export first and swap the file in for config.yml.")) return;
    let later: string[] = [];
    void mz.act(async () => {
      later = (await mz.api.resetAll()).onRestart;
      exported = null;
    }, "everything reset to config.yml").then((ok) => {
      if (ok && later.length) mz.say(`Reset. ${later.join(", ")} go back to config.yml when she restarts.`);
    });
  }
</script>

{#if mz.differences.length || exported}
  <div class="differs" role="note">
    {#if mz.differences.length}
      <p>Differs from config.yml: {mz.differences.join(" · ")} · <a href="#settings">Settings</a> · <a href="#tools">Tools</a></p>
      <div class="row">
        <button onclick={exportFile} title="Writes config.yml with these folded in, next to it (never over it). It stays on the PC.">Export to a file</button>
        <button class="danger" onclick={resetAll}>Reset all</button>
      </div>
    {/if}
    {#if exported}
      <p class="sub">Written to <code>{exported.path}</code> on the PC. Check it, then rename it to config.yml (keep the old one as a
        backup), press Reset all, and restart her.</p>
      <ul>{#each exported.changes as c (c.key)}<li><code>{c.key}</code>: {c.from} → {c.to}</li>{/each}</ul>
      <button onclick={() => (exported = null)}>Dismiss</button>
    {/if}
  </div>
{/if}

<style>
  .differs { font-size: 13px; border: 1px solid var(--accent); border-radius: 8px; padding: 6px 10px; margin: -6px 0 14px;
    overflow-wrap: anywhere; }
  .differs p { margin: 2px 0 6px; }
  .differs a { color: var(--accent); }
  .row { display: flex; gap: 8px; margin-bottom: 4px; }
  ul { margin: 4px 0 8px; padding-left: 18px; }
</style>
