<script lang="ts">
  import type { Dashboard } from "../lib/dashboard.svelte";
  import type { MzConsole } from "./console.svelte";
  import { kinds, type Earlier, type Memory, type Profile as ProfileT, type Subject } from "./types";
  import { when } from "../lib/format";
  import Badge from "../components/Badge.svelte";
  import Card from "../components/Card.svelte";
  import SelfNotes from "./SelfNotes.svelte";
  import Compact from "./Compact.svelte";
  import Profile from "./Profile.svelte";
  import Sort from "./Sort.svelte";
  let { mz, board }: { mz: MzConsole; board: Dashboard } = $props();

  let networks = $derived(board.status?.networks.map((n) => n.name) ?? []);
  let net = $state<string | null>(null);
  $effect(() => { if (networks.length && (net === null || !networks.includes(net))) net = networks[0]; });

  let subjects = $state<Subject[]>([]);
  let perSubject = $state(0);
  let unsorted = $state(0);
  let sortingOpen = $state(false);
  let selected = $state(""); // a subject, or "" with a search
  let query = $state("");
  let searched = $state("");
  let list = $state<Memory[] | null>(null);
  let earlier = $state<Earlier[]>([]);
  let editing = $state<number | null>(null);
  let editText = $state("");
  let editKind = $state("");
  let editAlso = $state("");
  let newKind = $state("");
  let newAlso = $state("");
  let profiles = $state<ProfileT[]>([]);
  let openNick = $state("");
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
    unsorted = r.unsorted;
    profiles = await mz.api.profiles(net);
  }

  async function loadList() {
    if (net === null || (!selected && !searched)) { list = null; return; }
    list = await mz.api.memories(net, selected ? { subject: selected } : { q: searched });
    earlier = selected ? await mz.api.memoryHistory(net, selected) : [];
  }

  $effect(() => { if (net !== null) void loadSubjects().catch(() => {}); });
  $effect(() => { void selected; void searched; void net; void loadList().catch(() => {}); });

  const reload = async () => { await loadSubjects(); await loadList(); };
  const change = (run: () => Promise<unknown>, done: string) => mz.act(async () => { await run(); await reload(); }, done);

  function open(s: string) { selected = s; searched = ""; query = ""; newSubject = s; editing = null; compacting = false; }
  function openOne(e: SubmitEvent) { e.preventDefault(); const s = openNick.trim().toLowerCase(); if (s) { open(s); openNick = ""; } }
  const names = (s: string) => s.split(",").map((a) => a.trim()).filter(Boolean);
  let profile = $derived(profiles.find((p) => p.subject === selected) ?? null);
  let isRoom = $derived(subjects.some((s) => s.subject === selected && s.room));
  const sharedWith = (m: Memory) => [m.subject, ...(m.also ?? [])].filter((s) => s !== selected).join(", ");
  const sourceText = (m: Memory) =>
    m.source === "told" ? `told by ${m.author.split(" ")[0]}` : m.source === "guessed" ? "guessed from chat" : m.source;

  function search(e: SubmitEvent) { e.preventDefault(); if (query.trim()) { searched = query.trim(); selected = ""; editing = null; } }

  function add(e: SubmitEvent) {
    e.preventDefault();
    const subject = newSubject.trim(), fact = newFact.trim();
    if (!subject || !fact || net === null) return;
    const n = net;
    let merged = 0;
    void mz.act(async () => {
      const r = await mz.api.remember(n, subject, fact, newKind, names(newAlso));
      if (r.merged) merged = r.id;
      await reload();
    }, `saved about ${subject}`).then((ok) => {
      if (!ok) return;
      newFact = "";
      newAlso = "";
      if (merged) mz.say(`Already had that as #${merged}; kept one copy with the longer wording.`);
    });
  }

  async function save(m: Memory) {
    if (net === null) return;
    const n = net;
    const also = names(editAlso);
    const metaChanged = editKind !== (m.kind ?? "") || also.join(",") !== (m.also ?? []).join(",");
    if (await mz.act(async () => {
      if (editText.trim() !== m.fact) await mz.api.editMemory(n, m.id, editText);
      if (metaChanged) await mz.api.memoryMeta(n, m.id, editKind, also);
      await reload();
    }, `#${m.id} saved`)) editing = null;
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
    {#if unsorted}
      <Card title="Sort">
        <p class="sub">{unsorted} {unsorted === 1 ? "memory has" : "memories have"} no kind yet.</p>
        <button onclick={() => (sortingOpen = true)} disabled={sortingOpen}>Suggest kinds and people</button>
      </Card>
    {/if}
    <Card title="Room memory">
      <p class="sub">About her and the channel; sent with every request.</p>
      {#each room as s (s.subject)}
        <button class="subject" class:on={selected === s.subject} onclick={() => open(s.subject)}>
          {s.subject} <span class="sub">{s.count}</span></button>
      {:else}
        <p class="empty">None yet.</p>
      {/each}
    </Card>
    <Card title="Profiles">
      {#each profiles as p (p.subject)}
        <button class="subject" class:on={selected === p.subject} onclick={() => open(p.subject)}>
          {p.subject} <span class="sub">{p.runBy ? `bot · ${p.runBy}` : p.bot ? "bot" : p.pronouns}</span></button>
      {:else}
        <p class="empty">None yet.</p>
      {/each}
      <form onsubmit={openOne}>
        <input class="grow" bind:value={openNick} placeholder="open a nick" aria-label="Open a nick" maxlength="64" />
        <button type="submit">Open</button>
      </form>
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
    {#if sortingOpen && net !== null}
      <Sort {mz} network={net} onclose={(applied) => { sortingOpen = false; if (applied) void reload(); }} />
    {/if}
    {#if selected && !isRoom && net !== null}
      <Profile {mz} network={net} subject={selected} {profile} onchange={() => void reload()} />
    {/if}
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
                  <div class="row">
                    <select bind:value={editKind} aria-label="Kind"><option value="">no kind</option>{#each kinds as k (k)}<option value={k}>{k}</option>{/each}</select>
                    <input class="grow" bind:value={editAlso} placeholder="also about (nicks, comma-separated)" aria-label="Also about" />
                  </div>
                  <div class="row"><button onclick={() => save(m)}>Save</button><button onclick={() => (editing = null)}>Cancel</button></div>
                {:else}
                  <div>{#if m.kind}<Badge text={m.kind} tone="busy" />{/if}{#if m.faded}<Badge text="faded" />{/if}{m.fact}</div>
                  <div class="sub">#{m.id}{#if !selected} · {m.subject}{/if} · by {m.author || "?"} ({sourceText(m)}) · {when(m.created)}{#if m.also?.length} · shared with {sharedWith(m)}{/if}{#if m.used} · came up {when(m.used)}{/if}{#if m.edited} · edited {when(m.edited)}{/if}
                    {#if m.locked}<Badge text="locked" tone="good" />{/if}
                    {#if m.sameAs}<Badge text={`same as #${m.sameAs}`} tone="busy" />{/if}</div>
                {/if}
              </div>
              {#if editing !== m.id}
                <button onclick={() => { editing = m.id; editText = m.fact; editKind = m.kind ?? ""; editAlso = (m.also ?? []).join(", "); }}>Edit</button>
                <button onclick={() => lock(m)} title={m.locked ? "Allow forgetting again" : "Keep it: nothing in IRC can forget it, and Forget here needs Unlock first"}>{m.locked ? "Unlock" : "Lock"}</button>
                <button class="danger" onclick={() => forget(m)} disabled={m.locked} title={m.locked ? "Unlock it first" : ""}>Forget</button>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </Card>

    {#if selected && earlier.length}
      <Card title={`Earlier versions (${earlier.length})`}>
        <p class="sub">Wording a newer memory replaced: a look merged into the current one, a like that was contradicted, or facts compacted. Not sent to her.</p>
        <ul class="rows">
          {#each earlier as e (e.id)}
            <li><div class="grow">
              <div>{#if e.kind}<Badge text={e.kind} />{/if}{e.fact}</div>
              <div class="sub">was #{e.memoryId} · by {e.author || "?"} · {when(e.created)} · {e.why} {when(e.replaced)}{#if e.replacedBy} into #{e.replacedBy}{/if}</div>
            </div></li>
          {/each}
        </ul>
      </Card>
    {/if}

    <Card title="Remember">
      <form class="add" onsubmit={add}>
        <input bind:value={newSubject} placeholder="about (a nick, or Mizira, or the channel)" aria-label="Subject" maxlength="64" />
        <textarea bind:value={newFact} rows="2" placeholder="the fact, naming who it's about" aria-label="Fact" maxlength="400"></textarea>
        <div class="row">
          <select bind:value={newKind} aria-label="Kind"><option value="">no kind</option>{#each kinds as k (k)}<option value={k}>{k}</option>{/each}</select>
          <input class="grow" bind:value={newAlso} placeholder="also about (nicks, comma-separated)" aria-label="Also about" />
        </div>
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
  .row { display: flex; gap: 6px; margin-top: 6px; flex-wrap: wrap; }
  .row .grow { flex: 1; min-width: 140px; }
  li { align-items: flex-start; }
  .sub { overflow-wrap: anywhere; }
  p.sub { margin: 0 0 8px; }
  .net { font-size: 13px; color: var(--muted); margin: 0 0 12px; }
  select { font: inherit; color: var(--fg); background: var(--bg); border: 1px solid var(--line); border-radius: 7px; padding: 5px 8px; }
</style>
