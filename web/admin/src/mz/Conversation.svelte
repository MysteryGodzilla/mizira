<script lang="ts">
  import type { MzConsole } from "./console.svelte";
  import type { Conversation } from "./types";
  import { Poll } from "./poll.svelte";
  import { ago } from "../lib/format";
  import Badge from "../components/Badge.svelte";
  import Card from "../components/Card.svelte";
  let { mz }: { mz: MzConsole } = $props();

  const poll = new Poll(() => mz.api.conversations());
  $effect(() => poll.start());
  let now = $state(Date.now() / 1000);
  $effect(() => {
    const t = setInterval(() => (now = Date.now() / 1000), 5000);
    return () => clearInterval(t);
  });

  // "14:32" today, "6 Oct 14:32" before; lines saved before times were kept show none.
  function clock(unix?: number): string {
    if (!unix) return "";
    const d = new Date(unix * 1000);
    const t = d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
    return d.toDateString() === new Date().toDateString() ? t : `${d.toLocaleDateString(undefined, { day: "numeric", month: "short" })} ${t}`;
  }

  const where = (c: Conversation) => (c.network ? `${c.channel} on ${c.network}` : c.channel);

  function reset(c: Conversation) {
    if (confirm(`Reset the conversation in ${where(c)}?\n\nHistory is cleared, any persona removed and the model put back to config.yml's. The recap stays. Same as ~reset.`)) {
      void mz.act(async () => { await mz.api.reset(c.network); await poll.refresh(); }, `reset ${where(c)}`);
    }
  }

  let folding = $state("");
  let folded = $state("");
  function fold(c: Conversation) {
    if (!confirm(`Fold ${where(c)} now?

Everything but the last 6 turns is summarised into the recap, then self-notes and people-notes are proposed, as after any fold. It waits for the model, so it can take a minute. Same as ~recap fold.`)) return;
    folding = c.network;
    folded = "";
    void mz.act(async () => {
      const r = await mz.api.fold(c.network);
      folded = `Folded ${r.folded} messages into the recap; kept the last ${r.kept}.`;
      await poll.refresh();
    }, `folded ${where(c)}`).finally(() => (folding = ""));
  }

  function clearRecap(c: Conversation) {
    if (confirm(`Clear the recap of ${where(c)}?\n\nIt's the summary of everything older than the history and can't be brought back. Same as ~recap clear.`)) {
      void mz.act(async () => { await mz.api.clearRecap(c.network); await poll.refresh(); }, `cleared the recap of ${where(c)}`);
    }
  }
</script>

{#if !poll.data}
  <Card title="Conversation"><p class="empty">{poll.error || "Loading…"}</p></Card>
{:else}
  {#each poll.data as c (c.network)}
    <Card title={`Conversation · ${where(c)}`}>
      {#snippet actions()}
        <button class="danger" onclick={() => reset(c)}>Reset</button>
      {/snippet}
      <dl>
        <div><dt>messages</dt><dd>{c.messages}</dd></div>
        <div><dt>history / fold at</dt><dd>{c.tokens} / {c.maxContext}</dd></div>
        <div><dt>last used</dt><dd>{c.lastUsed > 0 ? ago(now - c.lastUsed) : "—"}</dd></div>
      </dl>
      {#if c.maxContext > 0}
        <div class="bar" title="history size; past the limit it is folded into the recap">
          <span style:width="{Math.min(100, (100 * c.tokens) / c.maxContext)}%"></span>
        </div>
      {/if}
      {#if c.persona}
        <p class="persona"><Badge text="persona" tone="busy" />set by {c.personaBy}; tools are off until Reset.</p>
        <blockquote>{c.persona}</blockquote>
      {/if}
    </Card>

    <Card title="Recap">
      {#snippet actions()}
        <button onclick={() => fold(c)} disabled={folding !== ""}>{folding === c.network ? "Folding…" : "Fold now"}</button>
        {#if c.recap}<button class="danger" onclick={() => clearRecap(c)}>Clear recap</button>{/if}
      {/snippet}
      {#if folded}<p class="sub">{folded}</p>{/if}
      {#if c.recap}
        <p class="sub">{c.recap.length} characters, sent with every request in this channel.</p>
        <blockquote class="recap">{c.recap}</blockquote>
      {:else}
        <p class="empty">No recap yet: nothing has been folded.</p>
      {/if}
    </Card>

    <Card title="Recent history">
      {#if c.recent.length}
        <ul class="rows lines">
          {#each c.recent as line, i (i)}
            <li><span class="time" title={line.at ? new Date(line.at * 1000).toLocaleString() : "saved before times were kept"}>{clock(line.at)}</span><span class="role {line.role}">{line.role}</span><span class="grow">{line.text}</span></li>
          {/each}
        </ul>
      {:else}
        <p class="empty">Empty.</p>
      {/if}
    </Card>
  {/each}
{/if}

<style>
  dl { display: flex; flex-wrap: wrap; gap: 8px 28px; margin: 0; }
  dd { margin: 0; font-size: 20px; font-variant-numeric: tabular-nums; }
  dt { order: 2; font-size: 13px; color: var(--muted); }
  dl div { display: flex; flex-direction: column; }
  .bar { height: 4px; background: var(--line); border-radius: 2px; margin-top: 10px; overflow: hidden; }
  .bar span { display: block; height: 100%; background: var(--accent); }
  .persona { margin: 12px 0 4px; }
  blockquote { margin: 0; padding: 8px 10px; border-left: 3px solid var(--line); white-space: pre-wrap;
    overflow-wrap: anywhere; font-size: 14px; }
  .recap { max-height: 320px; overflow-y: auto; }
  .sub { margin: 0 0 6px; }
  .lines { max-height: 520px; overflow-y: auto; }
  .lines li { align-items: flex-start; font-size: 14px; white-space: pre-wrap; }
  .time { flex: none; width: 88px; font-size: 12px; color: var(--muted); padding-top: 2px; font-variant-numeric: tabular-nums; }
  .role { flex: none; width: 68px; font-size: 12px; color: var(--muted); padding-top: 2px; }
  .role.assistant { color: var(--accent); }
</style>
