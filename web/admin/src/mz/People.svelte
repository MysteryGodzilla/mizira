<script lang="ts">
  import type { Dashboard } from "../lib/dashboard.svelte";
  import type { MzConsole } from "./console.svelte";
  import type { Ignore, Score } from "./types";
  import { Poll } from "./poll.svelte";
  import { ago, when } from "../lib/format";
  import Badge from "../components/Badge.svelte";
  import Card from "../components/Card.svelte";
  let { mz, board }: { mz: MzConsole; board: Dashboard } = $props();

  const poll = new Poll(() => mz.api.people());
  $effect(() => poll.start());
  let p = $derived(poll.data);
  let networks = $derived(board.status?.networks.map((n) => n.name) ?? []);
  let now = $derived(board.status?.now ?? Date.now() / 1000);

  // The network only needs choosing with more than one.
  let net = $state("");
  $effect(() => { if (networks.length && !networks.includes(net)) net = networks[0]; });
  let ignoreNick = $state("");
  let ignoreMinutes = $state(60);
  let ignoreReason = $state("");
  let screenNick = $state("");
  const durations: [number, string][] = [[30, "30 minutes"], [60, "1 hour"], [360, "6 hours"], [1440, "1 day"], [10080, "1 week"]];

  const change = (run: () => Promise<unknown>, done: string) =>
    mz.act(async () => { await run(); await poll.refresh(); }, done);

  function addIgnore(e: SubmitEvent) {
    e.preventDefault();
    const nick = ignoreNick.trim();
    if (!nick) return;
    void change(() => mz.api.ignore(net, nick, ignoreMinutes, ignoreReason), `ignoring ${nick}`);
    ignoreNick = "";
    ignoreReason = "";
  }

  function addScreen(e: SubmitEvent) {
    e.preventDefault();
    const nick = screenNick.trim();
    if (!nick) return;
    if (confirm(`Screen ${nick}?\n\nTheir messages go through the gatekeeper, replies to them are checked, and their earlier turns are dropped from the conversation. Same as ~screen.`)) {
      void change(() => mz.api.screen(net, nick), `screening ${nick}`);
      screenNick = "";
    }
  }

  const how = (i: Ignore) => (i.kind === "bot" ? `by Mizira, during ${i.by}'s message` : i.kind === "flood" ? "flood" : `by ${i.by}`);
  const tone = (s: Score, at: number) => (s.score >= at ? "bad" : s.score >= at / 2 ? "busy" : "plain");
  const has = (list: string[], nick: string) => list.some((n) => n.toLowerCase() === nick);
  let screened = $derived(p ? [...new Set([...p.screened.in, ...p.screened.out].map((n) => n.toLowerCase()))] : []);
</script>

{#if networks.length > 1}
  <p class="net">Network for new entries:
    <select bind:value={net}>{#each networks as n (n)}<option value={n}>{n}</option>{/each}</select>
  </p>
{/if}

<Card title="Ignored">
  <form onsubmit={addIgnore}>
    <input class="nick" bind:value={ignoreNick} placeholder="nick" aria-label="Nick to ignore" maxlength="64" />
    <select bind:value={ignoreMinutes} aria-label="For how long">
      {#each durations as [m, label] (m)}<option value={m}>{label}</option>{/each}
    </select>
    <input class="grow" bind:value={ignoreReason} placeholder="reason (optional)" aria-label="Reason" maxlength="200" />
    <button type="submit">Ignore</button>
  </form>
  {#if p?.ignores.length}
    <ul class="rows">
      {#each p.ignores as i (i.network + i.nick)}
        <li>
          <div class="grow">
            <div><strong>{i.nick}</strong> <span class="sub">{ago(i.until - now)} left · {how(i)}{#if i.network} · {i.network}{/if}</span></div>
            {#if i.reason}<div class="sub">“{i.reason}”</div>{/if}
          </div>
          <span class="sub" title="until">{when(i.until)}</span>
          <button onclick={() => change(() => mz.api.unignore(i.network, i.nick), `no longer ignoring ${i.nick}`)}>Remove</button>
        </li>
      {/each}
    </ul>
  {:else}
    <p class="empty">{p ? "Nobody is ignored." : poll.error || "Loading…"}</p>
  {/if}
</Card>

<Card title="Screened">
  <form onsubmit={addScreen}>
    <input class="grow" bind:value={screenNick} placeholder="nick" aria-label="Nick to screen" maxlength="64" />
    <button type="submit">Screen</button>
  </form>
  <p class="sub">With <code>screenall</code> on, everyone but admins is screened anyway; this list matters when it's off.</p>
  {#if screened.length}
    <ul class="rows">
      {#each screened as nick (nick)}
        <li>
          <span class="grow"><strong>{nick}</strong>
            {#if p && !has(p.screened.in, nick)}<Badge text="replies only" />{/if}
            {#if p && !has(p.screened.out, nick)}<Badge text="messages only" />{/if}
          </span>
          <button onclick={() => change(() => mz.api.unscreen(nick), `no longer screening ${nick}`)}>Remove</button>
        </li>
      {/each}
    </ul>
  {:else if p}
    <p class="empty">Nobody is on the list.</p>
  {/if}
</Card>

<Card title="Suspicion">
  {#if p}
    <p class="sub">Halves every 10 minutes. At {p.quarantineAt} a speaker's recent turns are dropped from the conversation.</p>
    {#if p.suspicion.length}
      <ul class="rows">
        {#each p.suspicion as s (s.network + s.key)}
          <li>
            <span class="grow"><strong>{s.key}</strong>{#if s.network} <span class="sub">· {s.network}</span>{/if}</span>
            <Badge text={s.score.toFixed(1)} tone={tone(s, p.quarantineAt)} />
            <button onclick={() => change(() => mz.api.clearSuspicion(s.network, s.key), `cleared ${s.key}'s score`)}>Clear</button>
          </li>
        {/each}
      </ul>
    {:else}
      <p class="empty">No one has a score right now.</p>
    {/if}
  {:else}
    <p class="empty">{poll.error || "Loading…"}</p>
  {/if}
</Card>

<style>
  form { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 8px; }
  .nick { width: 160px; }
  form .grow { flex: 1; min-width: 140px; }
  select { font: inherit; color: var(--fg); background: var(--bg); border: 1px solid var(--line); border-radius: 7px; padding: 5px 8px; }
  .net { font-size: 13px; color: var(--muted); margin: 0 0 12px; }
  p.sub { margin: 0 0 6px; }
  code { font-size: 12px; }
</style>
