<script lang="ts">
  import type { MzConsole } from "./console.svelte";
  import { Poll } from "./poll.svelte";
  import Card from "../components/Card.svelte";
  let { mz }: { mz: MzConsole } = $props();

  const poll = new Poll(() => mz.api.bots());
  $effect(() => poll.start());
  let b = $derived(poll.data);

  let kind = $state<"nick" | "prefix">("nick");
  let value = $state("");

  const change = (run: () => Promise<unknown>, done: string) =>
    mz.act(async () => { await run(); await poll.refresh(); }, done);

  function add(e: SubmitEvent) {
    e.preventDefault();
    const v = value.trim();
    if (!v) return;
    void change(() => mz.api.addBot(kind, v), `now treating ${kind} ${v} as a bot`);
    value = "";
  }
</script>

<Card title="Other bots">
  <p class="sub">Lines from these get at most <strong>{b?.replyLimit ?? "…"}</strong> replies in a row, reset by a person
    talking or <strong>{b?.cooldown ?? "…"}</strong> of quiet (<code>botreplylimit</code> and <code>botcooldown</code> on the
    Settings tab). Same as <code>~bots</code>.</p>
  <form onsubmit={add}>
    <select bind:value={kind} aria-label="Recognise by">
      <option value="nick">by nick</option>
      <option value="prefix">by line prefix</option>
    </select>
    <input class="grow" bind:value={value} placeholder={kind === "nick" ? "nick, e.g. a bot with its own account" : "prefix, e.g. [botname]"}
      aria-label="Nick or prefix" maxlength="64" />
    <button type="submit">Add</button>
  </form>
  {#if !b}
    <p class="empty">{poll.error || "Loading…"}</p>
  {:else}
    <h3>By nick</h3>
    {#if b.nicks.length}
      <ul class="rows">
        {#each b.nicks as n (n)}
          <li><span class="grow">{n}</span>
            <button onclick={() => change(() => mz.api.removeBot("nick", n), `no longer treating ${n} as a bot`)}>Remove</button></li>
        {/each}
      </ul>
    {:else}<p class="empty">None.</p>{/if}
    <h3>By line prefix</h3>
    {#if b.prefixes.length}
      <ul class="rows">
        {#each b.prefixes as p (p)}
          <li><span class="grow"><code>{p}</code></span>
            <button onclick={() => change(() => mz.api.removeBot("prefix", p), `no longer treating ${p} as a bot`)}>Remove</button></li>
        {/each}
      </ul>
    {:else}<p class="empty">None.</p>{/if}
  {/if}
</Card>

<style>
  p.sub { margin: 0 0 10px; }
  form { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 6px; }
  form .grow { flex: 1; min-width: 160px; }
  select { font: inherit; color: var(--fg); background: var(--bg); border: 1px solid var(--line); border-radius: 7px; padding: 5px 8px; }
  h3 { font-size: 13px; color: var(--muted); font-weight: 600; margin: 14px 0 4px; }
  code { font-size: 13px; }
</style>
