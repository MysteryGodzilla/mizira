<script lang="ts">
  import type { MzConsole } from "./console.svelte";
  import type { Safety } from "./types";
  import { when } from "../lib/format";
  import Badge from "../components/Badge.svelte";
  import Card from "../components/Card.svelte";
  let { mz }: { mz: MzConsole } = $props();

  const KINDS: Record<string, string> = {
    gatekeeper: "Gatekeeper", reply: "Reply screen", quarantine: "Quarantine", memory: "Memory refused",
    quoted: "Quoted chat dropped", tool: "Tool refused", injection: "Injection stripped", ignore: "Ignored",
    bots: "Bot loop limit", console: "Console changes",
  };
  const tone: Record<string, "bad" | "busy" | "plain"> = { quarantine: "bad", gatekeeper: "busy", reply: "busy", memory: "busy", injection: "bad" };

  let days = $state(7);
  let kind = $state("");
  let who = $state("");
  let data = $state<Safety | null>(null);
  let error = $state("");
  let loading = $state(false);

  // Loads on open and on each filter change; the log isn't re-read every few seconds.
  async function load() {
    loading = true;
    try {
      data = await mz.api.safety(days, kind, who);
      error = "";
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
    loading = false;
  }
  $effect(() => { void days; void kind; void who; void load(); });

  const pick = (k: string) => (kind = kind === k ? "" : k);
  const person = (w: string) => (who = who.toLowerCase() === w.toLowerCase() ? "" : w);
</script>

<Card title="Safety">
  {#snippet actions()}
    <select bind:value={days} aria-label="Period">
      <option value={1}>last day</option><option value={7}>last 7 days</option><option value={30}>last 30 days</option>
    </select>
    <button onclick={load} disabled={loading}>{loading ? "Reading…" : "Refresh"}</button>
  {/snippet}
  <p class="sub">What she refused, screened out or quarantined, read from her log file. Click a kind or a person to
    filter; changes made in this console are listed under "Console changes".{#if data?.truncated} Only the newest events in this period were read.{/if}</p>
  {#if error}<p class="bad">{error}</p>{/if}
  {#if data}
    <div class="chips">
      {#each Object.entries(KINDS) as [k, label] (k)}
        {#if data.kinds[k]}
          <button class="chip" class:on={kind === k} onclick={() => pick(k)}>{label} <span>{data.kinds[k]}</span></button>
        {/if}
      {/each}
      {#if !Object.keys(data.kinds).length}<p class="empty">Nothing in this period.</p>{/if}
    </div>
  {/if}
</Card>

{#if data}
  <div class="layout">
    <Card title="By person">
      {#if data.people.length}
        <ul class="rows">
          {#each data.people as p (p.who)}
            <li>
              <button class="person" class:on={who.toLowerCase() === p.who.toLowerCase()} onclick={() => person(p.who)}>
                <strong>{p.who}</strong> <span class="sub">{p.total}</span>
              </button>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="empty">No one.</p>
      {/if}
    </Card>

    <Card title={`Events${kind ? ` · ${KINDS[kind] ?? kind}` : ""}${who ? ` · ${who}` : ""}`}>
      {#if data.events.length}
        <ul class="rows">
          {#each data.events as e, i (i)}
            <li>
              <div class="grow">
                <div><Badge text={KINDS[e.kind] ?? e.kind} tone={tone[e.kind] ?? "plain"} /><strong>{e.who || "?"}</strong>
                  {#if e.suspicion}<span class="sub">suspicion {Number(e.suspicion).toFixed(1)}</span>{/if}</div>
                {#if e.detail}<div class="detail">{e.detail}</div>{/if}
                <div class="sub">{when(e.time)} · {e.event}{#if e.channel} · {e.channel}{/if}</div>
              </div>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="empty">Nothing matches.</p>
      {/if}
    </Card>
  </div>
{/if}

<style>
  p.sub { margin: 0 0 10px; }
  .bad { color: var(--bad); }
  .chips { display: flex; gap: 6px; flex-wrap: wrap; }
  .chip span { color: var(--muted); margin-left: 4px; }
  .chip.on, .person.on { border-color: var(--accent); color: var(--accent); }
  .layout { display: grid; grid-template-columns: 220px 1fr; gap: 0 18px; align-items: start; }
  @media (max-width: 760px) { .layout { grid-template-columns: 1fr; } }
  .person { display: flex; justify-content: space-between; width: 100%; border: 0; padding: 2px 4px; }
  .detail { font-size: 14px; overflow-wrap: anywhere; margin: 2px 0; }
  select { font: inherit; font-size: 13px; color: var(--fg); background: var(--bg); border: 1px solid var(--line); border-radius: 7px; padding: 3px 6px; }
</style>
