import {
  type Component,
  createResource,
  createSignal,
  createMemo,
  Show,
  For,
  Suspense,
} from "solid-js";
import { Save, RotateCcw, SlidersHorizontal } from "lucide-solid";
import { api } from "../../shared/api/client";
import { showToast } from "../../shared/ui/Toast";
import { t } from "../../shared/i18n/i18n";

// --- Types ---

interface ProviderPriority {
  provider: string;
  priority: number;
}

// --- Constants ---

// Known providers with human-readable display names.
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

async function fetchPriorities(): Promise<ProviderPriority[]> {
  return api<ProviderPriority[]>("/metadata/provider-priorities");
}

async function savePriority(provider: string, priority: number): Promise<void> {
  await api(`/metadata/provider-priorities/${encodeURIComponent(provider)}`, {
    method: "PUT",
    body: JSON.stringify({ priority }),
  });
}

// --- Component ---

const ProviderPrioritySettings: Component = () => {
  const [priorities, { refetch }] = createResource(fetchPriorities);
  const [localPriorities, setLocalPriorities] = createSignal<Record<string, number>>({});
  const [saving, setSaving] = createSignal(false);

  // Merge API priorities with defaults for known providers.
  const merged = createMemo(() => {
    const apiData = priorities() ?? [];
    const map: Record<string, number> = {};
    for (const p of KNOWN_PROVIDERS) {
      const apiEntry = apiData.find((a) => a.provider === p.id);
      const localValue = localPriorities()[p.id];
      map[p.id] =
        localValue !== undefined
          ? localValue
          : apiEntry?.priority ?? DEFAULT_PRIORITY;
    }
    return map;
  });

  function handleChange(provider: string, value: number) {
    setLocalPriorities((prev) => ({ ...prev, [provider]: value }));
  }

  function handleReset() {
    const reset: Record<string, number> = {};
    for (const p of KNOWN_PROVIDERS) {
      reset[p.id] = DEFAULT_PRIORITY;
    }
    setLocalPriorities(reset);
  }

  async function handleSave() {
    setSaving(true);
    try {
      const map = merged();
      for (const p of KNOWN_PROVIDERS) {
        const current = map[p.id];
        await savePriority(p.id, current);
      }
      showToast(t("admin.providerPrioritiesSaved"), "success");
      setLocalPriorities({});
      void refetch();
    } catch (err: unknown) {
      showToast(
        err instanceof Error ? err.message : t("admin.failedToSavePriorities"),
        "error",
      );
    } finally {
      setSaving(false);
    }
  }

  const hasChanges = createMemo(() => Object.keys(localPriorities()).length > 0);

  return (
    <div class="flex flex-1 flex-col">
      {/* Page header */}
      <div class="flex items-center justify-between border-b border-slate-800 px-6 py-5">
        <div>
          <h1 class="text-xl font-bold text-slate-100">
            {t("admin.providerPriorities")}
          </h1>
          <p class="mt-1 text-sm text-slate-400">
            {t("admin.providerPrioritiesSubtitle")}
          </p>
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
          <Show when={!priorities.loading}>
            <div class="mx-auto max-w-2xl flex flex-col gap-6">
              <For each={KNOWN_PROVIDERS}>
                {(provider) => {
                  const value = () => merged()[provider.id];
                  return (
                    <div class="flex items-center gap-4 rounded-lg border border-slate-700 bg-slate-800/50 px-4 py-3">
                      <div class="min-w-0 flex-1">
                        <p class="text-sm font-medium text-slate-200">
                          {provider.name}
                        </p>
                        <p class="text-xs text-slate-500">{provider.id}</p>
                      </div>
                      <div class="flex items-center gap-3">
                        <input
                          type="range"
                          min="1"
                          max="10"
                          step="1"
                          value={value()}
                          onInput={(e) =>
                            handleChange(provider.id, Number(e.currentTarget.value))
                          }
                          class="w-32 accent-indigo-400"
                        />
                        <span class="w-8 text-right text-sm font-medium text-slate-200">
                          {value()}
                        </span>
                      </div>
                    </div>
                  );
                }}
              </For>
            </div>
          </Show>
        </Suspense>
      </div>
    </div>
  );
};

export default ProviderPrioritySettings;
