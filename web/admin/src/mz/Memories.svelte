<script lang="ts">
  import type { Dashboard } from "../lib/dashboard.svelte";
  import type { MzConsole } from "./console.svelte";
  import type { Memory, Subject } from "./types";
  import { when } from "../lib/format";
  import Badge from "../components/Badge.svelte";
  import Card from "../components/Card.svelte";
  import SelfNotes from "./SelfNotes.svelte";
  import Compact from "./Compact.svelte";
  let { mz, board }: { mz: MzConsole; board: Dashboard } = $props();

  let networks = $derived(board.status?.networks.map((n) => n.name) ?? []);
  let net = $state<string | null>(null);
  $effect(() => { if (networks.length && (net === null || !networks.includes(net))) net = networks[0]; });

  let subjects = $state<Subject[]>([]);
  let perSubject = $state(0);
  let selected = $state(""); // a subject, or "" with a search
  let query = $state("");
  let searched = $state("");
  let list = $state<Memory[] | null>(null);
  let editing = $state<number | null>(null);
  let editText = $state("");
  let newSubject = $state("");
  let newFact = $state("");
  let compacting = $state(false);

  let room = $derived(subjects.filter((s) => s.room));
  let people = $derived(subjects.filter((s) => !s.room));

  async function loadSubjects() {
    if (net === null) return;
    const r = await mz.api.memorySubjects(net);
    subjects = r.subjects;
    perSubject = r.perSubject;
  }

  async function loadList() {
    if (net === null || (!selected && !searched)) { list = null; return; }
    list = await mz.api.memories(net, selected ? { subject: selected } : { q: searched });
  }

  $effect(() => { if (net !== null) void loadSubjects().catch(() => {}); });
  $effect(() => { void selected; void searched; void net; void loadList().catch(() => {}); });

  const reload = async () => { await loadSubjects(); await loadList(); };
  const change = (run: () => Promise<unknown>, done: string) => mz.act(async () => { await run(); await reload(); }, done);

  function open(s: string) { selected = s; searched = ""; query = ""; newSubject = s; editing = null; compacting = false; }
  function search(e: SubmitEvent) { e.preventDefault(); if (query.trim()) { searched = query.trim(); selected = ""; editing = null; } }

  function add(e: SubmitEvent) {
    e.preventDefault();
    const subject = newSubject.trim(), fact = newFact.trim();
    if (!subject || !fact || net === null) return;
    const n = net;
    let merged = 0;
    void mz.act(async () => {
      const r = await mz.api.remember(n, subject, fact);
      if (r.merged) merged = r.id;
      await reload();
    }, `saved about ${subject}`).then((ok) => {
      if (!ok) return;
      newFact = "";
      if (merged) mz.say(`Already had that as #${merged}; kept one copy with the longer wording.`);
    });
  }

  async function save(m: Memory) {
    if (net === null) return;
    const n = net;
    if (await mz.act(async () => { await mz.api.editMemory(n, m.id, editText); await reload(); }, `#${m.id} edited`)) editing = null;
  }

  function forget(m: Memory) {
    if (net !== null && confirm(`Forget #${m.id} about ${m.subject}?\n\n${m.fact}`)) {
      const n = net;
      void change(() => mz.api.forgetMemory(n, m.id), `forgot #${m.id}`);
    }
  }

  function lock(m: Memory) {
    if (net === null) return;
    const n = net;
    void change(() => mz.api.lockMemory(n, m.id, !m.locked), m.locked ? `unlocked #${m.id}` : `locked #${m.id}`);
  }

  const full = (s: Subject) => perSubject > 0 && s.count >= perSubject;
  let dupes = $derived(list?.filter((m) => m.sameAs).length ?? 0);
</script>

{#if networks.length > 1}
  <p class="net">Network: <select bind:value={net}>{#each networks as n (n)}<option value={n}>{n}</option>{/each}</select></p>
{/if}

<div class="layout">
  <div class="side">
    <Card title="Find">
      <form onsubmit={search}>
        <input class="grow" bind:value={query} placeholder="search facts" aria-label="Search memories" />
        <button type="submit">Search</button>
      </form>
    </Card>
    <Card title="Room memory">
      <p class="sub">About her and the channel; sent with every request.</p>
      {#each room as s (s.subject)}
        <button class="subject" class:on={selected === s.subject} onclick={() => open(s.subject)}>
          {s.subject} <span class="sub">{s.count}</span></button>
      {:else}
        <p class="empty">None yet.</p>
      {/each}
    </Card>
    <Card title={`People and things (${people.length})`}>
      {#each people as s (s.subject)}
        <button class="subject" class:on={selected === s.subject} onclick={() => open(s.subject)}>
          {s.subject} <span class="sub" class:full={full(s)}>{s.count}{perSubject ? ` / ${perSubject}` : ""}</span></button>
      {:else}
        <p class="empty">Nothing remembered yet.</p>
      {/each}
    </Card>
  </div>

  <div class="main">
    {#if net !== null}<SelfNotes {mz} network={net} onchange={() => void reload()} />{/if}
    {#if compacting && selected && net !== null && list}
      <Compact {mz} network={net} subject={selected} current={list} {perSubject}
        onclose={(applied) => { compacting = false; if (applied) void reload(); }} />
    {/if}
    <Card title={selected ? `About ${selected}` : searched ? `Facts matching “${searched}”` : "Memories"}>
      {#snippet actions()}
        {#if selected && (list?.length ?? 0) >= 2 && !compacting}
          <button onclick={() => (compacting = true)} title="ask the model for a merged list to review">Compact</button>
        {/if}
      {/snippet}
      {#if list === null}
        <p class="empty">Pick a subject or search.</p>
      {:else if !list.length}
        <p class="empty">Nothing here.</p>
      {:else}
        {#if dupes}<p class="sub">{dupes} say the same as another one (saved before repeats were merged): edit the one to keep, forget the rest.</p>{/if}
        <ul class="rows">
          {#each list as m (m.id)}
            <li>
              <div class="grow">
                {#if editing === m.id}
                  <textarea bind:value={editText} rows="2" maxlength="400" aria-label="Fact"></textarea>
                  <div class="row"><button onclick={() => save(m)}>Save</button><button onclick={() => (editing = null)}>Cancel</button></div>
                {:else}
                  <div>{m.fact}</div>
                  <div class="sub">#{m.id}{#if !selected} · {m.subject}{/if} · by {m.author || "?"} · {when(m.created)}
                    {#if m.locked}<Badge text="locked" tone="good" />{/if}
                    {#if m.sameAs}<Badge text={`same as #${m.sameAs}`} tone="busy" />{/if}</div>
                {/if}
              </div>
              {#if editing !== m.id}
                <button onclick={() => { editing = m.id; editText = m.fact; }}>Edit</button>
                <button onclick={() => lock(m)} title={m.locked ? "Allow forgetting again" : "Keep it: nothing in IRC can forget it, and Forget here needs Unlock first"}>{m.locked ? "Unlock" : "Lock"}</button>
                <button class="danger" onclick={() => forget(m)} disabled={m.locked} title={m.locked ? "Unlock it first" : ""}>Forget</button>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </Card>

    <Card title="Remember">
      <form class="add" onsubmit={add}>
        <input bind:value={newSubject} placeholder="about (a nick, or Mizira, or the channel)" aria-label="Subject" maxlength="64" />
        <textarea bind:value={newFact} rows="2" placeholder="the fact, naming who it's about" aria-label="Fact" maxlength="400"></textarea>
        <button type="submit">Remember</button>
      </form>
      <p class="sub">Your own words: saved without the memory policy check, but the per-person limit applies and a repeat
        merges into the fact already held.</p>
    </Card>
  </div>
</div>

<style>
  .layout { display: grid; grid-template-columns: 260px 1fr; gap: 0 18px; align-items: start; }
  @media (max-width: 760px) { .layout { grid-template-columns: 1fr; } }
  .side :global(section), .main :global(section) { margin-bottom: 14px; }
  .subject { display: flex; justify-content: space-between; width: 100%; border: 0; border-radius: 6px; padding: 5px 8px; text-align: left; }
  .subject.on { background: var(--line); }
  .subject .sub { font-variant-numeric: tabular-nums; }
  .full { color: var(--bad); }
  form { display: flex; gap: 8px; flex-wrap: wrap; }
  form .grow { flex: 1; min-width: 120px; }
  .add { flex-direction: column; align-items: stretch; }
  .add button { align-self: flex-start; }
  textarea { font: inherit; color: var(--fg); background: var(--bg); border: 1px solid var(--line); border-radius: 7px;
    padding: 6px 10px; width: 100%; resize: vertical; }
  .row { display: flex; gap: 6px; margin-top: 6px; }
  li { align-items: flex-start; }
  .sub { overflow-wrap: anywhere; }
  p.sub { margin: 0 0 8px; }
  .net { font-size: 13px; color: var(--muted); margin: 0 0 12px; }
  select { font: inherit; color: var(--fg); background: var(--bg); border: 1px solid var(--line); border-radius: 7px; padding: 5px 8px; }
</style>
