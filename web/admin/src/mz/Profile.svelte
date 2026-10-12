<script lang="ts">
  import type { MzConsole } from "./console.svelte";
  import type { Profile } from "./types";
  import Card from "../components/Card.svelte";
  let { mz, network, subject, profile, onchange }:
    { mz: MzConsole; network: string; subject: string; profile: Profile | null; onchange: () => void } = $props();

  let editing = $state(false);
  let bot = $state(false);
  let runBy = $state("");
  let pronouns = $state("");
  let avatarPronouns = $state("");
  let aliases = $state("");

  $effect(() => { void subject; editing = false; });

  function edit() {
    bot = profile?.bot ?? false;
    runBy = profile?.runBy ?? "";
    pronouns = profile?.pronouns ?? "";
    avatarPronouns = profile?.avatarPronouns ?? "";
    aliases = (profile?.aliases ?? []).join(", ");
    editing = true;
  }

  async function save(e: SubmitEvent) {
    e.preventDefault();
    const p: Profile = { subject, bot: bot || runBy.trim() !== "", runBy: runBy.trim(), pronouns: pronouns.trim(),
      avatarPronouns: avatarPronouns.trim(), aliases: aliases.split(",").map((a) => a.trim()).filter(Boolean) };
    if (await mz.act(() => mz.api.setProfile(network, p), `profile saved for ${subject}`)) { editing = false; onchange(); }
  }

  async function remove() {
    if (confirm(`Delete ${subject}'s profile? Their memories stay.`) &&
      await mz.act(() => mz.api.deleteProfile(network, subject), `profile deleted for ${subject}`)) onchange();
  }

  let summary = $derived.by(() => {
    if (!profile) return [];
    const out: string[] = [];
    if (profile.runBy) out.push(`a bot run by ${profile.runBy}`);
    else if (profile.bot) out.push("a bot");
    if (profile.pronouns) out.push(profile.pronouns);
    if (profile.avatarPronouns) out.push(`avatar ${profile.avatarPronouns}`);
    if (profile.aliases?.length) out.push(`also called ${profile.aliases.join(", ")}`);
    for (const b of profile.runs ?? []) out.push(`runs the bot ${b}`);
    return out;
  });
</script>

<Card title={`Profile: ${subject}`}>
  {#snippet actions()}
    {#if !editing}<button onclick={edit}>{profile ? "Edit" : "Add profile"}</button>{/if}
    {#if profile && !editing}<button class="danger" onclick={remove}>Delete</button>{/if}
  {/snippet}
  {#if editing}
    <form onsubmit={save}>
      <label>Pronouns <input bind:value={pronouns} maxlength="24" placeholder="she/her" /></label>
      <label>Avatar's pronouns <input bind:value={avatarPronouns} maxlength="24" placeholder="only if different" /></label>
      <label>Also called <input bind:value={aliases} placeholder="other names, comma-separated" /></label>
      <label class="check"><input type="checkbox" bind:checked={bot} /> a bot</label>
      <label>Run by <input bind:value={runBy} maxlength="32" placeholder="the nick that runs it" /></label>
      <div class="row"><button type="submit">Save</button><button type="button" onclick={() => (editing = false)}>Cancel</button></div>
    </form>
  {:else if summary.length}
    <p>{summary.join(" · ")}</p>
    <p class="sub">Sent with their memories when they speak or are named.{#if profile?.avatarPronouns} Looks use the avatar's pronouns.{/if}</p>
  {:else}
    <p class="empty">No profile: pronouns, an avatar that differs, other names, or who runs a bot.</p>
  {/if}
</Card>

<style>
  form { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 8px 12px; align-items: end; }
  label { display: flex; flex-direction: column; gap: 3px; font-size: 12px; color: var(--muted); }
  label.check { flex-direction: row; align-items: center; gap: 6px; font-size: 13px; color: var(--fg); }
  .row { display: flex; gap: 6px; grid-column: 1 / -1; }
  p { margin: 0 0 6px; }
</style>
