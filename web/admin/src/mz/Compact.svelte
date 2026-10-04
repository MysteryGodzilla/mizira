<script lang="ts">
  import type { MzConsole } from "./console.svelte";
  import type { Memory } from "./types";
  import Card from "../components/Card.svelte";
  let { mz, network, subject, current, perSubject, onclose }: {
    mz: MzConsole; network: string; subject: string; current: Memory[]; perSubject: number; onclose: (applied: boolean) => void;
  } = $props();

  let asking = $state(true);
  let error = $state("");
  let basedOn = $state<number[]>([]);
  let text = $state("");

  async function preview() {
    asking = true;
    error = "";
    try {
      const r = await mz.api.compactPreview(network, subject);
      basedOn = r.basedOn;
      text = r.facts.join("\n");
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
    asking = false;
  }
  $effect(() => { void subject; void preview(); });

  let facts = $derived(text.split("\n").map((l) => l.trim()).filter(Boolean));
  let tooMany = $derived(perSubject > 0 && facts.length > perSubject);

  async function apply() {
    if (!confirm(`Replace the ${basedOn.length} memories about ${subject} with these ${facts.length}?\n\nThe old ones are deleted.`)) return;
    const ok = await mz.act(() => mz.api.compactApply(network, subject, basedOn, facts),
      `${subject}: ${basedOn.length} memories compacted to ${facts.length}`);
    if (ok) onclose(true);
  }
</script>

<Card title={`Compact ${subject}`}>
  {#snippet actions()}
    <button onclick={() => onclose(false)}>Cancel</button>
  {/snippet}
  {#if asking}
    <p class="empty">Asking the model to merge {current.length} memories… (waits its turn behind replies)</p>
  {:else if error}
    <p class="bad">{error}</p>
    <button onclick={preview}>Try again</button>
  {:else}
    <p class="sub">Nothing has changed yet. Check the proposed list against the current one: edit, add or delete lines
      (one fact per line), then apply. Apply is refused if a memory about {subject} was saved meanwhile.</p>
    <div class="cols">
      <div>
        <h3>Now ({current.length})</h3>
        <ul class="now">{#each current as m (m.id)}<li>{m.fact}</li>{/each}</ul>
      </div>
      <div>
        <h3>After ({facts.length}{perSubject ? ` / ${perSubject}` : ""})</h3>
        <textarea bind:value={text} rows={Math.min(20, Math.max(6, facts.length + 2))} aria-label="Compacted facts, one per line"></textarea>
        {#if tooMany}<p class="bad">Over the limit of {perSubject}.</p>{/if}
      </div>
    </div>
    <div class="row">
      <button onclick={apply} disabled={!facts.length || tooMany}>Apply</button>
      <button onclick={preview}>Ask again</button>
    </div>
  {/if}
</Card>

<style>
  .cols { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }
  @media (max-width: 760px) { .cols { grid-template-columns: 1fr; } }
  h3 { font-size: 13px; color: var(--muted); font-weight: 600; margin: 0 0 6px; }
  .now { margin: 0; padding-left: 18px; font-size: 14px; }
  .now li { margin-bottom: 4px; overflow-wrap: anywhere; }
  textarea { font: inherit; font-size: 14px; color: var(--fg); background: var(--bg); border: 1px solid var(--line);
    border-radius: 7px; padding: 6px 10px; width: 100%; resize: vertical; }
  .row { display: flex; gap: 6px; margin-top: 10px; }
  p.sub { margin: 0 0 10px; }
  .bad { color: var(--bad); }
</style>
