<script lang="ts">
  import type { MzConsole } from "./console.svelte";
  import type { ToolSetting } from "./types";
  import Badge from "../components/Badge.svelte";
  import Card from "../components/Card.svelte";
  let { mz }: { mz: MzConsole } = $props();

  let settings = $state<ToolSetting[]>([]);
  let failed = $state(false);
  // Edits in progress, by key; a key not here shows its live value.
  let draft = $state<Record<string, string>>({});

  async function load() {
    try {
      settings = await mz.api.toolSettings();
      failed = false;
    } catch {
      failed = true;
    }
  }
  void load();

  let groups = $derived([...new Set(settings.map((s) => s.group))].map((g) => ({
    name: g, items: settings.filter((s) => s.group === g),
  })));

  const shown = (s: ToolSetting) => draft[s.key] ?? s.value;
  const dirty = (s: ToolSetting) => s.key in draft && draft[s.key] !== s.value;

  async function save(s: ToolSetting) {
    const value = shown(s);
    const ok = await mz.act(() => mz.api.setToolSetting(s.key, value), `${s.key} set to ${value || "empty"}`);
    if (ok) {
      delete draft[s.key];
      await load();
    }
  }

  async function reset(s: ToolSetting) {
    const ok = await mz.act(() => mz.api.resetToolSetting(s.key), `${s.key} back to config.yml (${s.default || "empty"})`);
    if (ok) {
      delete draft[s.key];
      await load();
    }
  }
</script>

{#each groups as g (g.name)}
  <Card title={g.name}>
    <p class="sub">Tool settings from <code>env:</code> in config.yml. A change applies from the next picture and is kept
      across restarts until reset. Keys and safety policies stay in config.yml.</p>
    <ul class="rows">
      {#each g.items as s (s.key)}
        <li>
          <div class="grow">
            <div><strong>{s.key}</strong>{#if s.overridden} <Badge text="changed" tone="busy" />{/if}</div>
            <div class="sub">{s.help}{#if s.overridden}{" · config.yml: "}<code>{s.default || "empty"}</code>{/if}</div>
          </div>
          <div class="edit" class:full={s.kind === "lines"}>
            <form onsubmit={(e) => { e.preventDefault(); void save(s); }}>
              {#if s.kind === "lines"}
                <textarea rows="6" spellcheck="false" value={shown(s)} oninput={(e) => (draft[s.key] = e.currentTarget.value)}
                  aria-label={s.key}></textarea>
              {:else}
                <input class:wide={s.kind === "text"} value={shown(s)} oninput={(e) => (draft[s.key] = e.currentTarget.value)}
                  aria-label={s.key} inputmode={s.kind === "number" ? "numeric" : undefined} />
              {/if}
              <button type="submit" disabled={!dirty(s)}>Save</button>
            </form>
            {#if s.overridden}<button onclick={() => reset(s)} title="back to config.yml's value">Reset</button>{/if}
          </div>
        </li>
      {/each}
    </ul>
  </Card>
{:else}
  {#if failed}<Card title="Tool settings"><p class="empty">Couldn't load the tool settings.</p></Card>{/if}
{/each}

<style>
  .sub { margin: 2px 0 0; }
  p.sub { margin: 0 0 10px; }
  .edit { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; justify-content: flex-end; }
  form { display: flex; gap: 6px; }
  form input { width: 90px; }
  form input.wide { width: min(420px, 70vw); }
  .edit.full { flex-basis: 100%; }
  .edit.full form { flex: 1; align-items: flex-start; }
  textarea { flex: 1; min-width: 0; font: 12px/1.4 ui-monospace, monospace; white-space: pre; overflow-x: auto; }
  code { font-size: 12px; }
  li { flex-wrap: wrap; }
</style>
