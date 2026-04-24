import {
  type Component,
  createResource,
  createSignal,
  Show,
  For,
  Suspense,
  ErrorBoundary,
} from "solid-js";
import { createStore, produce } from "solid-js/store";
import {
  Mail,
  Plus,
  Pencil,
  Trash2,
  Send,
  Save,
  X,
} from "lucide-solid";
import { api } from "../../shared/api/client";
import Button from "../../shared/ui/Button";
import Input from "../../shared/ui/Input";
import { showToast } from "../../shared/ui/Toast";
import { t } from "../../shared/i18n/i18n";

// --- Types ---

interface EmailProvider {
  id: number;
  name: string;
  host: string;
  port: number;
  username: string;
  password: string;
  fromAddress: string;
  useTls: boolean;
  isDefault: boolean;
  createdAt: string;
}

// --- API ---

async function fetchProviders(): Promise<EmailProvider[]> {
  return api<EmailProvider[]>("/email/providers");
}

async function createProvider(data: Omit<EmailProvider, "id" | "createdAt">): Promise<EmailProvider> {
  return api<EmailProvider>("/email/providers", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

async function updateProvider(id: number, data: Omit<EmailProvider, "id" | "createdAt">): Promise<void> {
  await api(`/email/providers/${id}`, {
    method: "PUT",
    body: JSON.stringify(data),
  });
}

async function deleteProvider(id: number): Promise<void> {
  await api(`/email/providers/${id}`, { method: "DELETE" });
}

async function testProvider(id: number): Promise<void> {
  await api(`/email/providers/${id}/test`, { method: "POST" });
}

// --- Main ---

const EmailSettings: Component = () => {
  const [providers, { refetch }] = createResource(fetchProviders);
  const [showForm, setShowForm] = createSignal(false);
  const [editingId, setEditingId] = createSignal<number | null>(null);
  const [form, setForm] = createStore({
    name: "",
    host: "",
    port: 587,
    username: "",
    password: "",
    fromAddress: "",
    useTls: true,
    isDefault: false,
  });
  const [saving, setSaving] = createSignal(false);
  const [testing, setTesting] = createStore<Record<number, boolean>>({});
  const [deleting, setDeleting] = createStore<Record<number, boolean>>({});

  function resetForm() {
    setForm(produce((s) => {
      s.name = "";
      s.host = "";
      s.port = 587;
      s.username = "";
      s.password = "";
      s.fromAddress = "";
      s.useTls = true;
      s.isDefault = false;
    }));
    setEditingId(null);
  }

  function startEdit(provider: EmailProvider) {
    setForm(produce((s) => {
      s.name = provider.name;
      s.host = provider.host;
      s.port = provider.port;
      s.username = provider.username;
      s.password = provider.password;
      s.fromAddress = provider.fromAddress;
      s.useTls = provider.useTls;
      s.isDefault = provider.isDefault;
    }));
    setEditingId(provider.id);
    setShowForm(true);
  }

  async function handleSave(e: Event) {
    e.preventDefault();
    if (!form.name || !form.host || !form.username || !form.password || !form.fromAddress) {
      showToast(t("admin.allFieldsRequired"), "error");
      return;
    }
    setSaving(true);
    try {
      const payload = {
        name: form.name,
        host: form.host,
        port: form.port || 587,
        username: form.username,
        password: form.password,
        fromAddress: form.fromAddress,
        useTls: form.useTls,
        isDefault: form.isDefault,
      };
      if (editingId() !== null) {
        await updateProvider(editingId()!, payload);
        showToast(t("admin.providerUpdated"), "success");
      } else {
        await createProvider(payload);
        showToast(t("admin.providerCreated"), "success");
      }
      resetForm();
      setShowForm(false);
      void refetch();
    } catch (err: unknown) {
      showToast(err instanceof Error ? err.message : t("admin.failedToSaveProvider"), "error");
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(id: number) {
    setDeleting(id, true);
    try {
      await deleteProvider(id);
      showToast(t("admin.providerDeleted"), "success");
      void refetch();
    } catch (err: unknown) {
      showToast(err instanceof Error ? err.message : t("admin.failedToDeleteProvider"), "error");
    } finally {
      setDeleting(id, false);
    }
  }

  async function handleTest(id: number) {
    setTesting(id, true);
    try {
      await testProvider(id);
      showToast(t("admin.testEmailSent"), "success");
    } catch (err: unknown) {
      showToast(err instanceof Error ? err.message : t("admin.failedToSendTest"), "error");
    } finally {
      setTesting(id, false);
    }
  }

  return (
    <div class="flex flex-1 flex-col">
      {/* Page header */}
      <div class="flex items-center justify-between border-b border-slate-800 px-6 py-5">
        <div>
          <h1 class="text-xl font-bold text-slate-100">{t("admin.emailSettings")}</h1>
          <p class="mt-1 text-sm text-slate-400">{t("admin.manageEmailProviders")}</p>
        </div>
        <Button onClick={() => { resetForm(); setShowForm(true); }}>
          <Plus class="h-4 w-4" />
          {t("admin.addProvider")}
        </Button>
      </div>

      {/* Content */}
      <div class="flex-1 p-6">
        <ErrorBoundary
          fallback={(err) => (
            <div class="flex flex-col items-center justify-center gap-3 py-20 text-center">
              <p class="text-lg font-medium text-red-400">{t("admin.failedToLoadProviders")}</p>
              <p class="text-sm text-slate-500">{err.message}</p>
            </div>
          )}
        >
          <Suspense fallback={<p class="text-slate-400">{t("common.loading")}</p>}>
            <Show when={!providers.loading} fallback={<p class="text-slate-400">{t("common.loading")}</p>}>
              {/* Provider form */}
              <Show when={showForm()}>
                <div class="mb-6 rounded-xl border border-slate-700 bg-slate-800/50 p-6">
                  <div class="mb-4 flex items-center justify-between">
                    <h2 class="text-base font-semibold text-slate-100">
                      {editingId() !== null ? t("admin.editProvider") : t("admin.newProvider")}
                    </h2>
                    <button
                      onClick={() => setShowForm(false)}
                      class="rounded-lg p-1.5 text-slate-400 hover:bg-slate-700 hover:text-slate-200 transition-colors"
                    >
                      <X class="h-5 w-5" />
                    </button>
                  </div>
                  <form onSubmit={handleSave} class="flex flex-col gap-4">
                    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
                      <Input
                        label={t("admin.providerName")}
                        value={form.name}
                        onInput={(e) => setForm("name", e.currentTarget.value)}
                        required
                      />
                      <Input
                        label={t("admin.smtpHost")}
                        value={form.host}
                        onInput={(e) => setForm("host", e.currentTarget.value)}
                        required
                      />
                      <Input
                        label={t("admin.smtpPort")}
                        type="number"
                        value={String(form.port)}
                        onInput={(e) => setForm("port", parseInt(e.currentTarget.value, 10) || 587)}
                        required
                      />
                      <Input
                        label={t("admin.fromAddress")}
                        type="email"
                        value={form.fromAddress}
                        onInput={(e) => setForm("fromAddress", e.currentTarget.value)}
                        required
                      />
                      <Input
                        label={t("admin.smtpUsername")}
                        value={form.username}
                        onInput={(e) => setForm("username", e.currentTarget.value)}
                        required
                      />
                      <Input
                        label={t("admin.smtpPassword")}
                        type="password"
                        value={form.password}
                        onInput={(e) => setForm("password", e.currentTarget.value)}
                        required
                      />
                    </div>
                    <div class="flex items-center gap-4">
                      <label class="flex items-center gap-2 text-sm text-slate-300">
                        <input
                          type="checkbox"
                          checked={form.useTls}
                          onChange={(e) => setForm("useTls", e.currentTarget.checked)}
                          class="h-4 w-4 rounded border-slate-600 bg-slate-800 text-indigo-600"
                        />
                        {t("admin.useTLS")}
                      </label>
                      <label class="flex items-center gap-2 text-sm text-slate-300">
                        <input
                          type="checkbox"
                          checked={form.isDefault}
                          onChange={(e) => setForm("isDefault", e.currentTarget.checked)}
                          class="h-4 w-4 rounded border-slate-600 bg-slate-800 text-indigo-600"
                        />
                        {t("admin.defaultProvider")}
                      </label>
                    </div>
                    <div class="flex justify-end gap-3">
                      <Button variant="secondary" type="button" onClick={() => setShowForm(false)}>
                        {t("common.cancel")}
                      </Button>
                      <Button type="submit" loading={saving()}>
                        <Save class="h-4 w-4" />
                        {t("common.save")}
                      </Button>
                    </div>
                  </form>
                </div>
              </Show>

              {/* Providers list */}
              <Show
                when={(providers() ?? []).length > 0}
                fallback={
                  <div class="flex flex-col items-center justify-center gap-4 py-20 text-center">
                    <Mail class="h-16 w-16 text-slate-600" />
                    <p class="text-lg font-medium text-slate-300">{t("admin.noProvidersYet")}</p>
                  </div>
                }
              >
                <div class="flex flex-col gap-3">
                  <For each={providers()}>
                    {(provider) => (
                      <div class="flex items-center justify-between rounded-lg border border-slate-700 bg-slate-800/50 px-4 py-3">
                        <div class="min-w-0 flex-1">
                          <div class="flex items-center gap-2">
                            <p class="text-sm font-medium text-slate-200">{provider.name}</p>
                            <Show when={provider.isDefault}>
                              <span class="rounded-full bg-indigo-600/20 px-2 py-0.5 text-[10px] font-medium text-indigo-300">
                                {t("admin.default")}
                              </span>
                            </Show>
                          </div>
                          <p class="text-xs text-slate-400">
                            {provider.host}:{provider.port} · {provider.fromAddress}
                          </p>
                        </div>
                        <div class="flex items-center gap-1">
                          <button
                            onClick={() => handleTest(provider.id)}
                            disabled={testing[provider.id]}
                            class="rounded p-1.5 text-slate-400 hover:bg-slate-700 hover:text-slate-200 transition-colors disabled:opacity-50"
                            title={t("admin.sendTest")}
                          >
                            <Send class="h-4 w-4" />
                          </button>
                          <button
                            onClick={() => startEdit(provider)}
                            class="rounded p-1.5 text-slate-400 hover:bg-slate-700 hover:text-slate-200 transition-colors"
                            title={t("common.edit")}
                          >
                            <Pencil class="h-4 w-4" />
                          </button>
                          <button
                            onClick={() => handleDelete(provider.id)}
                            disabled={deleting[provider.id]}
                            class="rounded p-1.5 text-slate-400 hover:bg-red-900/50 hover:text-red-300 transition-colors disabled:opacity-50"
                            title={t("common.delete")}
                          >
                            <Trash2 class="h-4 w-4" />
                          </button>
                        </div>
                      </div>
                    )}
                  </For>
                </div>
              </Show>
            </Show>
          </Suspense>
        </ErrorBoundary>
      </div>
    </div>
  );
};

export default EmailSettings;
