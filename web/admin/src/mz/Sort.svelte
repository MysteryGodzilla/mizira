<script lang="ts">
  import type { MzConsole } from "./console.svelte";
  import { kinds, type SortItem } from "./types";
  import Card from "../components/Card.svelte";
  let { mz, network, onclose }: { mz: MzConsole; network: string; onclose: (applied: boolean) => void } = $props();

  type Row = SortItem & { take: boolean; othersText: string };
  let asking = $state(true);
  let error = $state("");
  let rows = $state<Row[]>([]);
  let unsorted = $state(0);
  let problems = $state<string[]>([]);

  async function preview() {
    asking = true;
    error = "";
    problems = [];
    try {
      const r = await mz.api.sortPreview(network);
      unsorted = r.unsorted;
      rows = r.suggestions.map((s) => ({ ...s, take: true, othersText: (s.others ?? []).join(", ") }));
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
    asking = false;
  }
  $effect(() => { void network; void preview(); });

  let taken = $derived(rows.filter((r) => r.take));

  async function apply() {
    const items = taken.map((r) => ({ id: r.id, kind: r.kind, about: r.about.trim(),
      others: r.othersText.split(",").map((o) => o.trim()).filter(Boolean) }));
    let res: { applied: number; problems: string[] } | null = null;
    const ok = await mz.act(async () => { res = await mz.api.sortApply(network, items); }, `sorting applied`);
    if (!ok || !res) return;
    const r: { applied: number; problems: string[] } = res;
    if (r.problems.length) { problems = r.problems; rows = rows.filter((x) => !x.take); return; }
    onclose(true);
  }
</script>

<Card title="Sort memories">
  {#snippet actions()}
    <button onclick={() => onclose(problems.length > 0)}>Close</button>
  {/snippet}
  {#if asking}
    <p class="empty">Asking the model to sort up to 25 memories… (waits its turn behind replies)</p>
  {:else if error}
    <p class="bad">{error}</p>
    <button onclick={preview}>Try again</button>
  {:else}
    <p class="sub">Nothing has changed yet. {rows.length} of {unsorted} unsorted memories. Check each one: what kind it is,
      who it's about (moving it changes whose memories it's sent with) and who else it involves. Untick any to leave as
      they are, then apply.</p>
    {#if problems.length}<p class="bad">Not applied: {problems.join("; ")}</p>{/if}
    <ul class="rows">
      {#each rows as r (r.id)}
        <li>
          <input type="checkbox" bind:checked={r.take} aria-label={`Apply #${r.id}`} />
          <div class="grow">
            <div>{r.fact}</div>
            <div class="row">
              <select bind:value={r.kind} aria-label="Kind"><option value="">no kind</option>{#each kinds as k (k)}<option value={k}>{k}</option>{/each}</select>
              <label>about <input bind:value={r.about} class:moved={r.about !== r.subject} maxlength="64" /></label>
              <label class="grow">also about <input bind:value={r.othersText} placeholder="nobody else" /></label>
            </div>
            <div class="sub">#{r.id} · now about {r.subject}{#if r.about !== r.subject} · moves to {r.about}{/if}</div>
          </div>
        </li>
      {/each}
    </ul>
    <div class="row">
      <button onclick={apply} disabled={!taken.length}>Apply {taken.length}</button>
      {#if unsorted > rows.length}<button onclick={preview}>Suggest the next batch</button>{/if}
    </div>
  {/if}
</Card>

<style>
  .row { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; margin-top: 6px; }
  .row .grow { flex: 1; min-width: 140px; }
  label { display: flex; gap: 4px; align-items: center; font-size: 12px; color: var(--muted); }
  label input { min-width: 0; }
  .moved { border-color: var(--accent); }
  li { align-items: flex-start; }
  .bad { color: var(--bad); }
</style>
