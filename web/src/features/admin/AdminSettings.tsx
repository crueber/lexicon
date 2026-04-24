import {
  type Component,
  createResource,
  createSignal,
  createEffect,
  Show,
  For,
  Suspense,
  ErrorBoundary,
} from "solid-js";
import { createStore, produce } from "solid-js/store";
import { Settings, Plus, Trash2, Save } from "lucide-solid";
import { api } from "../../shared/api/client";
import Button from "../../shared/ui/Button";
import Input from "../../shared/ui/Input";
import { showToast } from "../../shared/ui/Toast";
import { t } from "../../shared/i18n/i18n";

// --- Types ---

interface SettingsMap {
  [key: string]: string;
}

// --- API ---

async function fetchSettings(): Promise<SettingsMap> {
  return api<SettingsMap>("/admin/settings");
}

async function saveSettings(settings: SettingsMap): Promise<void> {
  await api("/admin/settings", {
    method: "PUT",
    body: JSON.stringify(settings),
  });
}

// --- Main ---

const AdminSettings: Component = () => {
  const [settings, { refetch }] = createResource(fetchSettings);
  const [form, setForm] = createStore<SettingsMap>({});
  const [newKey, setNewKey] = createSignal("");
  const [newValue, setNewValue] = createSignal("");
  const [saving, setSaving] = createSignal(false);

  // Initialize form when settings load.
  createEffect(() => {
    const s = settings();
    if (s) {
      setForm(produce((draft) => {
        Object.assign(draft, s);
      }));
    }
  });

  function addSetting() {
    const key = newKey().trim();
    if (!key) {
      showToast(t("admin.keyRequired"), "error");
      return;
    }
    setForm(key, newValue());
    setNewKey("");
    setNewValue("");
  }

  function removeSetting(key: string) {
    setForm(produce((draft) => {
      delete draft[key];
    }));
  }

  async function handleSave() {
    setSaving(true);
    try {
      await saveSettings({ ...form });
      showToast(t("admin.settingsSaved"), "success");
      void refetch();
    } catch (err: unknown) {
      showToast(err instanceof Error ? err.message : t("admin.failedToSaveSettings"), "error");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div class="flex flex-1 flex-col">
      {/* Page header */}
      <div class="flex items-center justify-between border-b border-slate-800 px-6 py-5">
        <div>
          <h1 class="text-xl font-bold text-slate-100">{t("admin.settings")}</h1>
          <p class="mt-1 text-sm text-slate-400">{t("admin.manageAppSettings")}</p>
        </div>
        <Button onClick={handleSave} loading={saving()}>
          <Save class="h-4 w-4" />
          {t("common.save")}
        </Button>
      </div>

      {/* Content */}
      <div class="flex-1 p-6">
        <ErrorBoundary
          fallback={(err) => (
            <div class="flex flex-col items-center justify-center gap-3 py-20 text-center">
              <p class="text-lg font-medium text-red-400">{t("admin.failedToLoadSettings")}</p>
              <p class="text-sm text-slate-500">{err.message}</p>
            </div>
          )}
        >
          <Suspense fallback={<p class="text-slate-400">{t("common.loading")}</p>}>
            <Show when={!settings.loading} fallback={<p class="text-slate-400">{t("common.loading")}</p>}>
              <div class="mx-auto max-w-2xl flex flex-col gap-6">
                {/* Existing settings */}
                <Show
                  when={Object.keys(form).length > 0}
                  fallback={
                    <div class="flex flex-col items-center justify-center gap-4 py-12 text-center">
                      <Settings class="h-12 w-12 text-slate-600" />
                      <p class="text-slate-400">{t("admin.noSettingsYet")}</p>
                    </div>
                  }
                >
                  <div class="flex flex-col gap-3">
                    <For each={Object.entries(form)}>
                      {([key, value]) => (
                        <div class="flex items-center gap-3 rounded-lg border border-slate-700 bg-slate-800/50 px-4 py-3">
                          <div class="min-w-0 flex-1">
                            <p class="text-xs font-medium text-slate-500">{key}</p>
                            <Input
                              value={value}
                              onInput={(e) => setForm(key, e.currentTarget.value)}
                              class="mt-1"
                            />
                          </div>
                          <button
                            onClick={() => removeSetting(key)}
                            class="rounded p-1.5 text-slate-400 hover:bg-red-600/20 hover:text-red-400 transition-colors"
                            title={t("common.remove")}
                          >
                            <Trash2 class="h-4 w-4" />
                          </button>
                        </div>
                      )}
                    </For>
                  </div>
                </Show>

                {/* Add new setting */}
                <div class="rounded-lg border border-slate-700 bg-slate-800/50 p-4">
                  <p class="mb-3 text-sm font-medium text-slate-300">{t("admin.addSetting")}</p>
                  <div class="flex flex-col gap-3 sm:flex-row">
                    <Input
                      placeholder={t("admin.settingKey")}
                      value={newKey()}
                      onInput={(e) => setNewKey(e.currentTarget.value)}
                      class="flex-1"
                    />
                    <Input
                      placeholder={t("admin.settingValue")}
                      value={newValue()}
                      onInput={(e) => setNewValue(e.currentTarget.value)}
                      class="flex-1"
                    />
                    <Button variant="secondary" onClick={addSetting}>
                      <Plus class="h-4 w-4" />
                      {t("common.add")}
                    </Button>
                  </div>
                </div>
              </div>
            </Show>
          </Suspense>
        </ErrorBoundary>
      </div>
    </div>
  );
};

export default AdminSettings;
