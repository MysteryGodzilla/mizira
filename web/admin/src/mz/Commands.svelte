<script lang="ts">
  import type { MzConsole } from "./console.svelte";
  import type { Cheatsheet, CommandInfo } from "./types";
  import Badge from "../components/Badge.svelte";
  import Card from "../components/Card.svelte";
  let { mz }: { mz: MzConsole } = $props();

  let sheet = $state<Cheatsheet | null>(null);
  let error = $state("");
  $effect(() => {
    mz.api.commands().then((s) => (sheet = s), (e) => (error = e instanceof Error ? e.message : String(e)));
  });

  let filter = $state("");
  let adminOnly = $state(false);
  const matches = (c: CommandInfo, f: string) =>
    !f || [c.name, c.text, ...c.usage].some((s) => s.toLowerCase().includes(f));
  let shown = $derived((sheet?.commands ?? []).filter((c) => matches(c, filter.trim().toLowerCase()) && (!adminOnly || c.admin)));
  let groups = $derived([...(sheet?.groups ?? []), "Other"].filter((g) => shown.some((c) => c.group === g)));
  const off = (c: CommandInfo) => !!c.feature && mz.feature(c.feature)?.active === false;
  // How to type it in the channel: her name first when commands need it.
  const typed = (u: string) => (sheet?.needName ? `${sheet.name} ${u}` : u);
</script>

<Card title="IRC commands">
  <p class="sub">Everything you can type in the channel, from her own command list, so it stays current.
    {#if sheet?.needName}Commands work only after her name: <code>{typed(sheet.commands[0]?.usage[0] ?? "~help")}</code>.{/if}
    <Badge text="admin only" tone="busy" /> marks commands only admins can run; others are open to everyone.</p>
  <div class="row">
    <input class="grow" bind:value={filter} placeholder="Find a command or what it does, e.g. memory, ignore, fold" aria-label="Filter commands" />
    <label class="toggle"><input type="checkbox" bind:checked={adminOnly} /> admin only</label>
  </div>
  {#if error}<p class="bad">{error}</p>{:else if !sheet}<p class="empty">Loading…</p>{:else if !shown.length}<p class="empty">Nothing matches.</p>{/if}
</Card>

{#each groups as g (g)}
  <Card title={g}>
    <ul class="rows">
      {#each shown.filter((c) => c.group === g) as c (c.name)}
        <li class:dim={off(c)}>
          <div class="grow">
            <div class="head"><strong>{c.name}</strong>
              {#if c.admin}<Badge text="admin only" tone="busy" />{/if}
              {#if off(c)}<Badge text="feature off" />{/if}</div>
            {#if c.text}<div>{c.text}</div>{/if}
            <div class="usage">{#each c.usage as u (u)}<code>{typed(u)}</code>{/each}</div>
          </div>
        </li>
      {/each}
    </ul>
  </Card>
{/each}

<style>
  .row { display: flex; gap: 10px; align-items: center; margin-top: 8px; }
  .head { display: flex; gap: 8px; align-items: center; margin-bottom: 2px; }
  .usage { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 4px; }
  .usage code { font-size: 13px; padding: 1px 6px; border: 1px solid var(--line); border-radius: 4px; }
  .dim { opacity: 0.55; }
  .toggle { white-space: nowrap; font-size: 14px; }
</style>
