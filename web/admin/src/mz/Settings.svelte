<script lang="ts">
  import type { MzConsole } from "./console.svelte";
  import type { Setting } from "./types";
  import { GROUPS, meta } from "./settings";
  import Badge from "../components/Badge.svelte";
  import Card from "../components/Card.svelte";
  import Model from "./Model.svelte";
  let { mz }: { mz: MzConsole } = $props();

  // Edits in progress, by key; a key not here shows its live value.
  let draft = $state<Record<string, string>>({});
  let filter = $state("");

  let groups = $derived(
    GROUPS.map((g) => ({
      name: g,
      items: mz.settings.filter((s) => meta(s.key).group === g && (!filter || s.key.includes(filter.toLowerCase()))),
    })).filter((g) => g.items.length),
  );

  const shown = (s: Setting) => draft[s.key] ?? s.value;
  const dirty = (s: Setting) => s.key in draft && draft[s.key] !== s.value;

  async function save(s: Setting, value: string, box?: HTMLInputElement | HTMLSelectElement) {
    const m = meta(s.key);
    if (m.warn && !confirm(`${s.key} = ${value}\n\n${m.warn}`)) {
      if (box) resync(s, box);
      return;
    }
    const ok = await mz.act(async () => {
      const r = await mz.api.set(s.key, value);
      if (r.warning) throw new Error(`${s.key} ${r.value}: ${r.warning}`);
    }, `${s.key} set to ${value}`);
    if (ok) delete draft[s.key];
    if (box) resync(s, box);
  }

  // A refused change leaves the control showing the live value.
  function resync(s: Setting, box: HTMLInputElement | HTMLSelectElement) {
    const live = mz.settings.find((x) => x.key === s.key)?.value ?? s.value;
    if (box instanceof HTMLInputElement) box.checked = live === "true";
    else box.value = live;
  }

  async function reset(s: Setting) {
    await mz.act(() => mz.api.resetSetting(s.key), `${s.key} back to config.yml (${s.default || "empty"})`);
    delete draft[s.key];
  }
</script>

<Model {mz} />

<Card title="Settings">
  <p class="sub">Every <code>~set</code> setting. A change applies now and is kept across restarts until reset to
    config.yml's value. API keys and passwords aren't here.</p>
  <input class="filter" bind:value={filter} placeholder="filter" aria-label="Filter settings" />
</Card>

{#each groups as g (g.name)}
  <Card title={g.name}>
    <ul class="rows">
      {#each g.items as s (s.key)}
        {@const m = meta(s.key)}
        <li>
          <div class="grow">
            <div><strong>{s.key}</strong>
              {#if s.overridden}<Badge text="changed" tone="busy" />{/if}
              {#if !s.editable}<Badge text="config.yml only" />{/if}
            </div>
            <div class="sub">{m.help}{#if s.overridden} config.yml: <code>{s.default || "empty"}</code>{/if}</div>
          </div>
          <div class="edit">
            {#if !s.editable}
              <code class="value" title={s.value}>{s.value.length > 60 ? s.value.slice(0, 60) + "…" : s.value}</code>
            {:else if m.kind === "bool"}
              <label class="switch"><input type="checkbox" checked={s.value === "true"}
                onchange={(e) => save(s, String(e.currentTarget.checked), e.currentTarget)} /> {s.value === "true" ? "on" : "off"}</label>
            {:else if m.kind === "choice"}
              <select value={s.value} onchange={(e) => save(s, e.currentTarget.value, e.currentTarget)} aria-label={s.key}>
                {#each m.choices ?? [] as c (c)}<option value={c}>{c}</option>{/each}
                {#if !(m.choices ?? []).includes(s.value)}<option value={s.value}>{s.value}</option>{/if}
              </select>
            {:else}
              <form onsubmit={(e) => { e.preventDefault(); void save(s, shown(s)); }}>
                <input value={shown(s)} oninput={(e) => (draft[s.key] = e.currentTarget.value)} aria-label={s.key}
                  inputmode={m.kind === "number" ? "decimal" : undefined} />
                <button type="submit" disabled={!dirty(s)}>Save</button>
              </form>
            {/if}
            {#if s.overridden}<button onclick={() => reset(s)} title="back to config.yml's value">Reset</button>{/if}
          </div>
        </li>
      {/each}
    </ul>
  </Card>
{/each}

<style>
  .sub { margin: 2px 0 0; }
  p.sub { margin: 0 0 10px; }
  .filter { width: 220px; }
  .edit { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; justify-content: flex-end; }
  form { display: flex; gap: 6px; }
  form input { width: 170px; }
  .value { max-width: 280px; overflow-wrap: anywhere; font-size: 12px; color: var(--muted); }
  .switch { display: flex; gap: 6px; align-items: center; font-size: 13px; }
  select { font: inherit; color: var(--fg); background: var(--bg); border: 1px solid var(--line); border-radius: 7px; padding: 5px 8px; }
  code { font-size: 12px; }
  li { flex-wrap: wrap; }
</style>
