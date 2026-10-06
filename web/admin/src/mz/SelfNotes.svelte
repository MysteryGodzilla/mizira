<script lang="ts">
  import type { MzConsole } from "./console.svelte";
  import type { SelfNote } from "./types";
  import { when } from "../lib/format";
  import Badge from "../components/Badge.svelte";
  import Card from "../components/Card.svelte";
  let { mz, network, onchange }: { mz: MzConsole; network: string; onchange: () => void } = $props();

  let showDecided = $state(false);
  let notes = $state<SelfNote[]>([]);
  let editing = $state<number | null>(null);
  let editText = $state("");

  async function load() {
    notes = await mz.api.selfNotes(network, showDecided ? "" : "pending");
  }
  $effect(() => { void network; void showDecided; void mz.pendingNotes.length; void load().catch(() => {}); });

  async function approve(n: SelfNote, text = "") {
    let merged = 0;
    const ok = await mz.act(async () => {
      const r = await mz.api.approveSelfNote(network, n.id, text);
      if (r.merged) merged = r.memoryId;
    }, `approved self-note #${n.id}: now room memory`);
    if (!ok) return;
    editing = null;
    await load();
    onchange();
    if (merged) mz.say(`Approved #${n.id}; she already knew that, so memory #${merged} keeps one copy.`);
  }

  async function deny(n: SelfNote) {
    if (await mz.act(() => mz.api.denySelfNote(network, n.id), `denied self-note #${n.id}`)) await load();
  }
</script>

<Card title={`Notes from folds${mz.pendingNotes.length ? ` (${mz.pendingNotes.length} waiting)` : ""}`}>
  {#snippet actions()}
    <label class="toggle"><input type="checkbox" bind:checked={showDecided} /> show decided</label>
  {/snippet}
  <p class="sub">What she noticed about herself, and about the people who spoke, when chat was folded into the recap.
    Nothing here reaches her until you approve it; then it becomes a memory about that person (room memory, for a note
    about her). Proposals come from chat, so read them as suggestions, not facts.</p>
  {#if !notes.length}
    <p class="empty">{showDecided ? "No notes yet." : "Nothing waiting."}</p>
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
              <div class="sub">#{n.id} · about {n.subject} · {when(n.created)}{#if n.why} · from “{n.why}”{/if}
                {#if n.status === "approved"}<Badge text={`approved → memory #${n.memoryId}`} tone="good" />{/if}
                {#if n.status === "denied"}<Badge text="denied" />{/if}
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
  {/if}
</Card>

<style>
  p.sub { margin: 0 0 8px; }
  .sub { overflow-wrap: anywhere; }
  .toggle { font-size: 13px; color: var(--muted); display: flex; gap: 6px; align-items: center; }
  textarea { font: inherit; color: var(--fg); background: var(--bg); border: 1px solid var(--line); border-radius: 7px;
    padding: 6px 10px; width: 100%; resize: vertical; }
  .row { display: flex; gap: 6px; margin-top: 6px; flex-wrap: wrap; }
  li { align-items: flex-start; flex-wrap: wrap; }
</style>
