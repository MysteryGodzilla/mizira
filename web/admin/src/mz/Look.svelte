<script lang="ts">
  import type { MzConsole } from "./console.svelte";
  import type { Look, LookVariant } from "./types";
  import Card from "../components/Card.svelte";
  let { mz, network, subject, look, onchange }:
    { mz: MzConsole; network: string; subject: string; look: Look | null; onchange: () => void } = $props();

  let editing = $state(false);
  let appearance = $state("");
  let hair = $state("");
  let outfit = $state("");
  let defaultName = $state("");
  let pictureName = $state("");
  let reference = $state("");
  let variants = $state<LookVariant[]>([]);

  $effect(() => {
    void subject; void look; editing = false;
  });

  function edit() {
    appearance = look?.appearance ?? "";
    hair = look?.hair ?? "";
    outfit = look?.outfit ?? "";
    defaultName = look?.default_name ?? "";
    pictureName = look?.picture_name ?? "";
    reference = look?.reference ?? "";
    variants = (look?.variants ?? []).map((v) => ({ ...v }));
    editing = true;
  }

  const current = (): Look => ({
    appearance: appearance.trim(), hair: hair.trim(), outfit: outfit.trim(), default_name: defaultName.trim(),
    picture_name: pictureName.trim(), reference: reference.trim(),
    variants: variants.filter((v) => v.name.trim() || v.hair.trim() || v.outfit.trim())
      .map((v) => ({ name: v.name.trim(), hair: v.hair.trim(), outfit: v.outfit.trim() })),
  });

  async function save(e: SubmitEvent) {
    e.preventDefault();
    if (await mz.act(() => mz.api.setLook(network, subject, current()), `look saved for ${subject}`)) { editing = false; onchange(); }
  }

  async function remove() {
    const empty: Look = { appearance: "", hair: "", outfit: "", reference: "", variants: [] };
    if (confirm(`Remove ${subject}'s look? Pictures go back to their look memories.`) &&
      await mz.act(() => mz.api.setLook(network, subject, empty), `look removed for ${subject}`)) onchange();
  }
</script>

<Card title={`Look for pictures: ${subject}`}>
  {#snippet actions()}
    {#if !editing}<button onclick={edit}>{look ? "Edit" : "Add look"}</button>{/if}
    {#if look && !editing}<button class="danger" onclick={remove}>Remove</button>{/if}
  {/snippet}
  {#if editing}
    <form onsubmit={save}>
      <label class="wide">Appearance, always used word for word (age, build, face, eyes, ears, skin)
        <textarea bind:value={appearance} rows="2" maxlength="400" placeholder="adult woman in her mid-20s, slim, green eyes, pointed elf ears, fair skin"></textarea></label>
      <label class="wide">Hair (default)
        <input bind:value={hair} maxlength="400" placeholder="short lavender bob with a black flower hairpin" /></label>
      <label class="wide">Outfit (default; used unless the request names other clothes)
        <textarea bind:value={outfit} rows="2" maxlength="400" placeholder="lilac cardigan over a cream shirt, pleated skirt"></textarea></label>
      <label class="wide">Name of the default hair and outfit, for asking for them ("casual hori"; "default", "normal" and "usual" work too)
        <input bind:value={defaultName} maxlength="20" placeholder="casual" /></label>
      <label class="wide">Name in pictures, when theirs would be drawn (blank: {subject})
        <input bind:value={pictureName} maxlength="30" placeholder="Mystery" /></label>
      <label class="wide">Reference picture (a link, kept for later)
        <input bind:value={reference} maxlength="300" placeholder="https://…" /></label>
      <div class="wide">
        <div class="sub">Variants: named in a request ("in her business look", "hair in a bun"), they swap the hair and/or the outfit.</div>
        {#each variants as v, i (i)}
          <div class="variant">
            <div class="head">
              <input bind:value={v.name} maxlength="20" placeholder="name, e.g. business" aria-label="variant name" />
              <button type="button" onclick={() => variants.splice(i, 1)} title="remove this variant">✕</button>
            </div>
            <label>Hair (blank: the default)
              <textarea bind:value={v.hair} rows="2" maxlength="400" aria-label="variant hair"></textarea></label>
            <label>Outfit (blank: the default)
              <textarea bind:value={v.outfit} rows="3" maxlength="400" aria-label="variant outfit"></textarea></label>
          </div>
        {/each}
        {#if variants.length < 8}<button type="button" onclick={() => variants.push({ name: "", hair: "", outfit: "" })}>Add variant</button>{/if}
      </div>
      <div class="row"><button type="submit">Save</button><button type="button" onclick={() => (editing = false)}>Cancel</button></div>
    </form>
  {:else if look}
    <dl>
      <dt>Appearance</dt><dd>{look.appearance}</dd>
      {#if look.hair}<dt>Hair</dt><dd>{look.hair}</dd>{/if}
      {#if look.outfit}<dt>Outfit ({look.default_name || "casual"})</dt><dd>{look.outfit}</dd>{/if}
      {#if look.picture_name}<dt>Name in pictures</dt><dd>{look.picture_name}</dd>{/if}
      {#each look.variants as v (v.name)}
        <dt>{v.name}</dt><dd>{[v.hair && `hair: ${v.hair}`, v.outfit && `outfit: ${v.outfit}`].filter(Boolean).join(" · ")}</dd>
      {/each}
      {#if look.reference}<dt>Reference</dt><dd><a href={look.reference} target="_blank" rel="noreferrer">{look.reference}</a></dd>{/if}
    </dl>
    <p class="sub">Used for every new picture that names {subject}, instead of their look memories.</p>
  {:else}
    <p class="empty">No look set: pictures use {subject}'s look memories, if any.</p>
  {/if}
</Card>

<style>
  form { display: grid; gap: 8px; }
  label { display: flex; flex-direction: column; gap: 3px; font-size: 12px; color: var(--muted); }
  textarea, input { font: inherit; }
  textarea { width: 100%; box-sizing: border-box; resize: vertical; min-height: 2.6em; }
  .variant { display: grid; gap: 6px; margin: 6px 0; padding: 8px; border: 1px solid var(--line); border-radius: 6px; }
  .variant .head { display: flex; gap: 6px; }
  .variant .head input { flex: 1; max-width: 240px; }
  .row { display: flex; gap: 6px; }
  dl { display: grid; grid-template-columns: max-content 1fr; gap: 3px 12px; margin: 0 0 6px; }
  dt { color: var(--muted); font-size: 12px; }
  dd { margin: 0; overflow-wrap: anywhere; }
</style>
