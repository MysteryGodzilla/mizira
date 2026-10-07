<script lang="ts">
  import type { MzConsole } from "./console.svelte";
  import type { SelfNote } from "./types";
  import { when } from "../lib/format";
  import Badge from "../components/Badge.svelte";
  import Card from "../components/Card.svelte";
  let { mz, network, onchange }: { mz: MzConsole; network: string; onchange: () => void } = $props();

  let showDecided = $state(false);
  let decidedAs = $state<"decided" | "approved" | "denied" | "expired">("decided");
  let search = $state("");
  let notes = $state<SelfNote[]>([]);
  let more = $state(false);
  let editing = $state<number | null>(null);
  let editText = $state("");

  // Waiting notes, or one page of decided ones at a time: there may be hundreds.
  async function load(append = false) {
    const r = await mz.api.selfNotes(network, showDecided ? decidedAs : "pending", showDecided ? search.trim() : "",
      append ? notes.length : 0, showDecided ? PAGE : 200);
    notes = append ? [...notes, ...r.notes] : r.notes;
    more = r.more;
  }
  const PAGE = 50;
  $effect(() => { void network; void showDecided; void decidedAs; void search; void mz.pendingNotes.length; void load().catch(() => {}); });

  async function approve(n: SelfNote, text = "") {
    let merged = 0;
    const ok = await mz.act(async () => {
      const r = await mz.api.approveSelfNote(network, n.id, text);
      if (r.merged) merged = r.memoryId;
    }, `approved note #${n.id}: now a memory about ${n.subject}`);
    if (!ok) return;
    editing = null;
    await load();
    onchange();
    if (merged) mz.say(`Approved #${n.id}; she already knew that, so memory #${merged} keeps one copy.`);
  }

  async function deny(n: SelfNote) {
    if (await mz.act(() => mz.api.denySelfNote(network, n.id), `denied note #${n.id}`)) await load();
  }
</script>

<Card title={`Notes from folds${mz.pendingNotes.length ? ` (${mz.pendingNotes.length} waiting)` : ""}`}>
  {#snippet actions()}
    <label class="toggle"><input type="checkbox" bind:checked={showDecided} /> show decided</label>
  {/snippet}
  <p class="sub">What she noticed about herself, and about the people who spoke, when chat was folded into the recap.
    Nothing here reaches her until you approve it; then it becomes a memory about that person (room memory, for a note
    about her). Proposals come from chat, so read them as suggestions, not facts.</p>
  {#if showDecided}
    <div class="row filters">
      <select bind:value={decidedAs} aria-label="Which decided notes">
        <option value="decided">all decided</option>
        <option value="approved">approved</option>
        <option value="denied">denied</option>
        <option value="expired">expired</option>
      </select>
      <input class="grow" bind:value={search} placeholder="Find a person or a word" aria-label="Search notes" />
    </div>
    <p class="sub">Pending notes expire after 14 days. Every decision is kept, so the same note isn't proposed again.</p>
  {/if}
  {#if !notes.length}
    <p class="empty">{showDecided ? "No notes match." : "Nothing waiting."}</p>
  {:else}
    <ul class="rows">
      {#each notes as n (n.id)}
        <li>
          <div class="grow">
            {#if editing === n.id}
              <textarea bind:value={editText} rows="2" maxlength="400" aria-label="Note"></textarea>
              <div class="row">
                <button onclick={() => approve(n, editText)} disabled={!editText.trim()}>Approve as edited</button>
                <button onclick={() => (editing = null)}>Cancel</button>
              </div>
            {:else}
              <div>{n.text}</div>
              {#if n.order}<div class="bad">Reads like an order ({n.order}), not a fact: a memory save would refuse it.</div>{/if}
              <div class="sub">#{n.id} · about {n.subject} · {when(n.created)}{#if n.why} · from “{n.why}”{/if}
                {#if n.status === "approved"}<Badge text={`approved → memory #${n.memoryId}`} tone="good" />{/if}
                {#if n.status === "denied"}<Badge text="denied" />{/if}
                {#if n.status === "expired"}<Badge text="expired" />{/if}
                {#if n.status !== "pending"}<span>by {n.decidedBy}</span>{/if}</div>
            {/if}
          </div>
          {#if n.status === "pending" && editing !== n.id}
            <button onclick={() => approve(n)}>Approve</button>
            <button onclick={() => { editing = n.id; editText = n.text; }}>Edit</button>
            <button class="danger" onclick={() => deny(n)}>Deny</button>
          {/if}
        </li>
      {/each}
    </ul>
    {#if more}<button onclick={() => void load(true)}>Show {PAGE} more</button>{/if}
  {/if}
</Card>

<style>
  p.sub { margin: 0 0 8px; }
  .sub { overflow-wrap: anywhere; }
  .toggle { font-size: 13px; color: var(--muted); display: flex; gap: 6px; align-items: center; }
  textarea { font: inherit; color: var(--fg); background: var(--bg); border: 1px solid var(--line); border-radius: 7px;
    padding: 6px 10px; width: 100%; resize: vertical; }
  .row { display: flex; gap: 6px; margin-top: 6px; flex-wrap: wrap; }
  .filters { margin: 0 0 6px; }
  li { align-items: flex-start; flex-wrap: wrap; }
</style>
