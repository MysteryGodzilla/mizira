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
          {#if s.meter}
            {@const m = s.meter}
            <div class="meter" class:over={m.over} class:warn={!m.over && !!m.warning}>
              <div class="meter-label"><span>{m.label}</span><span>${m.used.toFixed(2)} / ${m.limit.toFixed(2)}</span></div>
              <div class="track" role="meter" aria-label={m.label} aria-valuemin={0} aria-valuemax={m.limit}
                aria-valuenow={Math.min(m.used, m.limit)}>
                <div class="fill" style:width="{Math.min(100, (m.used / m.limit) * 100)}%"></div>
              </div>
              {#if m.warning}<p class="meter-warning">{m.warning}</p>{/if}
              {#if m.note}<p class="meter-note">{m.note}</p>{/if}
            </div>
          {/if}
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
  .meter { margin-top: 10px; font-size: 13px; }
  .meter-label { display: flex; justify-content: space-between; margin-bottom: 4px; }
  .meter-label span:first-child { color: var(--muted); }
  .track { height: 8px; border-radius: 999px; background: var(--line); overflow: hidden; }
  .fill { height: 100%; background: var(--ok); border-radius: 999px; }
  .warn .fill { background: var(--accent); }
  .over .fill { background: var(--bad); }
  .meter-warning { margin: 6px 0 0; color: var(--accent); }
  .over .meter-warning { color: var(--bad); }
  .meter-note { margin: 4px 0 0; color: var(--muted); font-size: 12px; }
</style>
