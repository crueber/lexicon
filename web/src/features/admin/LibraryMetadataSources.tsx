import {
  type Component,
  createResource,
  createSignal,
  Show,
  For,
  Suspense,
} from "solid-js";
import { SlidersHorizontal, Save, RotateCcw, Library } from "lucide-solid";
import { api } from "../../shared/api/client";
import { showToast } from "../../shared/ui/Toast";
import { t } from "../../shared/i18n/i18n";

// --- Types ---

interface LibraryItem {
  id: number;
  name: string;
}

interface MetadataSource {
  provider: string;
  fieldPriority: number;
}

// --- Constants ---

const KNOWN_PROVIDERS: { id: string; name: string }[] = [
  { id: "google_books", name: "Google Books" },
  { id: "open_library", name: "Open Library" },
  { id: "hardcover", name: "Hardcover" },
  { id: "comic_vine", name: "Comic Vine" },
  { id: "audible", name: "Audible" },
  { id: "douban", name: "Douban" },
  { id: "lubimyczytac", name: "LubimyCzytac" },
  { id: "ranobedb", name: "RanobeDB" },
];

const DEFAULT_PRIORITY = 5;

// --- API ---

async function fetchLibraries(): Promise<LibraryItem[]> {
  return api<LibraryItem[]>("/libraries");
}

async function fetchLibrarySources(libraryId: number): Promise<MetadataSource[]> {
  return api<MetadataSource[]>(`/libraries/${libraryId}/metadata-sources`);
}

async function saveLibrarySources(libraryId: number, sources: MetadataSource[]): Promise<void> {
  await api(`/libraries/${libraryId}/metadata-sources`, {
    method: "PUT",
    body: JSON.stringify({ sources }),
  });
}

// --- Component ---

const LibraryMetadataSources: Component = () => {
  const [libraries] = createResource(fetchLibraries);
  const [selectedLibraryId, setSelectedLibraryId] = createSignal<number | null>(null);
  const [sources, setSources] = createSignal<Record<string, number>>({});
  const [saving, setSaving] = createSignal(false);

  createResource(selectedLibraryId, async (id) => {
    if (!id) return;
    const data = await fetchLibrarySources(id);
    const map: Record<string, number> = {};
    for (const p of KNOWN_PROVIDERS) {
      const found = data.find((d) => d.provider === p.id);
      map[p.id] = found ? found.fieldPriority : 0;
    }
    setSources(map);
  });

  function toggleProvider(provider: string) {
    setSources((prev) => {
      const current = prev[provider] ?? 0;
      return { ...prev, [provider]: current > 0 ? 0 : DEFAULT_PRIORITY };
    });
  }

  function setPriority(provider: string, value: number) {
    setSources((prev) => ({ ...prev, [provider]: value }));
  }

  function handleReset() {
    const reset: Record<string, number> = {};
    for (const p of KNOWN_PROVIDERS) {
      reset[p.id] = 0;
    }
    setSources(reset);
  }

  async function handleSave() {
    const id = selectedLibraryId();
    if (!id) return;
    setSaving(true);
    try {
      const payload: MetadataSource[] = [];
      for (const p of KNOWN_PROVIDERS) {
        const priority = sources()[p.id] ?? 0;
        if (priority > 0) {
          payload.push({ provider: p.id, fieldPriority: priority });
        }
      }
      await saveLibrarySources(id, payload);
      showToast(t("admin.librarySourcesSaved"), "success");
    } catch (err: unknown) {
      showToast(
        err instanceof Error ? err.message : t("admin.failedToSaveLibrarySources"),
        "error",
      );
    } finally {
      setSaving(false);
    }
  }

  return (
    <div class="flex flex-1 flex-col">
      {/* Page header */}
      <div class="flex items-center justify-between border-b border-slate-800 px-6 py-5">
        <div>
          <h1 class="text-xl font-bold text-slate-100">
            {t("admin.libraryMetadataSources")}
          </h1>
          <p class="mt-1 text-sm text-slate-400">
            {t("admin.libraryMetadataSourcesSubtitle")}
          </p>
        </div>
      </div>

      {/* Content */}
      <div class="flex-1 overflow-y-auto px-6 py-6">
        <Suspense
          fallback={
            <div class="flex items-center justify-center py-16">
              <SlidersHorizontal class="h-8 w-8 animate-pulse text-indigo-400" />
            </div>
          }
        >
          <Show when={!libraries.loading}>
            <div class="mx-auto max-w-2xl flex flex-col gap-6">
              {/* Library selector */}
              <div class="flex flex-col gap-2">
                <label class="text-sm font-medium text-slate-300">
                  {t("admin.selectLibrary")}
                </label>
                <select
                  value={selectedLibraryId() ?? ""}
                  onChange={(e) => {
                    const v = e.currentTarget.value;
                    setSelectedLibraryId(v === "" ? null : Number(v));
                  }}
                  class="w-full rounded-lg border border-slate-700 bg-slate-800 px-3 py-2 text-sm text-slate-200 focus:border-indigo-500 focus:outline-none"
                >
                  <option value="">{t("admin.chooseLibrary")}</option>
                  <For each={libraries()}>
                    {(lib) => <option value={lib.id}>{lib.name}</option>}
                  </For>
                </select>
              </div>

              <Show when={selectedLibraryId()}>
                <div class="flex flex-col gap-3">
                  <For each={KNOWN_PROVIDERS}>
                    {(provider) => {
                      const enabled = () => (sources()[provider.id] ?? 0) > 0;
                      const priority = () => sources()[provider.id] ?? 0;
                      return (
                        <div class="flex items-center gap-4 rounded-lg border border-slate-700 bg-slate-800/50 px-4 py-3">
                          <input
                            type="checkbox"
                            checked={enabled()}
                            onChange={() => toggleProvider(provider.id)}
                            class="h-4 w-4 accent-indigo-400"
                          />
                          <div class="min-w-0 flex-1">
                            <p class="text-sm font-medium text-slate-200">
                              {provider.name}
                            </p>
                            <p class="text-xs text-slate-500">{provider.id}</p>
                          </div>
                          <Show when={enabled()}>
                            <div class="flex items-center gap-3">
                              <input
                                type="range"
                                min="1"
                                max="10"
                                step="1"
                                value={priority()}
                                onInput={(e) =>
                                  setPriority(provider.id, Number(e.currentTarget.value))
                                }
                                class="w-24 accent-indigo-400"
                              />
                              <span class="w-6 text-right text-sm font-medium text-slate-200">
                                {priority()}
                              </span>
                            </div>
                          </Show>
                        </div>
                      );
                    }}
                  </For>
                </div>

                <div class="flex items-center gap-2">
                  <button
                    onClick={handleReset}
                    class="flex items-center gap-2 rounded-lg border border-slate-700 bg-slate-800 px-3 py-2 text-sm text-slate-200 hover:bg-slate-700 transition-colors"
                  >
                    <RotateCcw class="h-4 w-4" />
                    {t("common.resetDefaults")}
                  </button>
                  <button
                    onClick={handleSave}
                    disabled={saving()}
                    class="flex items-center gap-2 rounded-lg bg-indigo-600 px-3 py-2 text-sm font-medium text-white hover:bg-indigo-500 disabled:opacity-50 transition-colors"
                  >
                    <Save class="h-4 w-4" />
                    {saving() ? t("common.saving") : t("common.save")}
                  </button>
                </div>
              </Show>
            </div>
          </Show>
        </Suspense>
      </div>
    </div>
  );
};

export default LibraryMetadataSources;
