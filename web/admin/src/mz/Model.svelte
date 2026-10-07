<script lang="ts">
  import type { MzConsole } from "./console.svelte";
  import Card from "../components/Card.svelte";
  let { mz }: { mz: MzConsole } = $props();

  let list = $state<{ current: string; models: { id: string; name?: string }[] } | null>(null);
  let error = $state("");
  let pick = $state("");
  let busy = $state(false);

  async function load() {
    try {
      list = await mz.api.models();
      pick = list.current;
      error = "";
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }
  $effect(() => { void load(); });

  const label = (m: { id: string; name?: string }) => (m.name ? `${m.name} (${m.id})` : m.id);

  async function change() {
    if (!list || pick === list.current) return;
    if (!confirm(`Switch her to ${pick}?\n\nEach channel conversation is cleared (the recap and memories stay). ` +
      "Her next reply waits while the model loads, about 10 seconds. Reset on model in Settings, or ~reset, goes back to config.yml's.")) return;
    busy = true;
    await mz.act(() => mz.api.switchModel(pick), `switched to ${pick}`);
    busy = false;
    await load();
  }
</script>

<Card title="Model">
  {#if error}
    <p class="bad">{error}</p>
  {:else if !list}
    <p class="empty">Asking the model server…</p>
  {:else}
    <p class="sub">What the model server offers; the one she's using is selected. Same as <code>~models</code>.</p>
    <div class="row">
      <select class="grow" bind:value={pick} aria-label="Model" disabled={busy}>
        {#each list.models as m (m.id)}
          <option value={m.id}>{label(m)}{m.id === list.current ? " · current" : ""}</option>
        {/each}
      </select>
      <button onclick={change} disabled={busy || pick === list.current}>{busy ? "Switching…" : "Switch"}</button>
    </div>
  {/if}
</Card>

<style>
  .row { display: flex; gap: 8px; align-items: center; }
  p.sub { margin: 0 0 8px; }
</style>
