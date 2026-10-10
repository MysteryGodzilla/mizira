<script lang="ts">
  import { onDestroy } from "svelte";
  import type { MzConsole } from "./console.svelte";
  import type { Service } from "./types";
  import Card from "../components/Card.svelte";
  let { mz }: { mz: MzConsole } = $props();

  // Slower than the rest of the page: each refresh asks the model server for its figures.
  const EVERY_MS = 15000;
  let services = $state<Service[] | null>(null);
  let failed = $state(false);

  async function load() {
    try {
      services = await mz.api.services();
      failed = false;
    } catch {
      failed = true;
    }
  }
  void load();
  const timer = setInterval(load, EVERY_MS);
  onDestroy(() => clearInterval(timer));
</script>

<Card title="Services">
  {#if services}
    {#each services as s (s.id)}
      <div class="service">
        <p class="name">
          {s.name}
          {#if s.link}<a href={s.link} target="_blank" rel="noopener noreferrer">open ↗</a>{/if}
        </p>
        {#if s.error}
          <p class="bad">{s.error}</p>
        {:else}
          <dl>
            {#each s.stats as st (st.label)}<div><dt>{st.label}</dt><dd>{st.value}</dd></div>{/each}
          </dl>
        {/if}
      </div>
    {/each}
  {:else if failed}
    <p class="empty">Couldn't load the services.</p>
  {:else}
    <p class="empty">Loading…</p>
  {/if}
</Card>

<style>
  .service + .service { border-top: 1px solid var(--line); margin-top: 10px; padding-top: 10px; }
  .name { margin: 0 0 6px; font-weight: 600; font-size: 14px; display: flex; gap: 10px; align-items: baseline; }
  .name a { font-weight: 400; font-size: 13px; color: var(--accent); }
  .bad { margin: 0; font-size: 13px; color: var(--bad); }
  dl { display: flex; flex-wrap: wrap; gap: 6px 18px; margin: 0; font-size: 13px; }
  dl div { display: flex; gap: 6px; }
  dt { color: var(--muted); }
  dd { margin: 0; }
</style>
