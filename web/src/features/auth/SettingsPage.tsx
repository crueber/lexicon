import {
  type Component,
  createResource,
  createSignal,
  createEffect,
  Show,
  For,
} from "solid-js";
import { createStore, produce } from "solid-js/store";
import { User, Lock, Palette, Save, Shield, Trash2 } from "lucide-solid";
import { api } from "../../shared/api/client";
import { useAuth } from "./AuthProvider";
import Button from "../../shared/ui/Button";
import Input from "../../shared/ui/Input";
import { showToast } from "../../shared/ui/Toast";
import { t } from "../../shared/i18n/i18n";

// --- Types ---

interface UserSettings {
  theme?: string;
  bookCardsPerRow?: number;
}

interface UserProfile {
  id: number;
  username: string;
  email?: string;
  name?: string;
  enabled: boolean;
  createdAt: string;
}

interface ContentRestriction {
  id: number;
  userId: number;
  restrictionType: string;
  value: string;
  mode: string;
}

// --- API helpers ---

async function fetchSettings(): Promise<UserSettings> {
  return api<UserSettings>("/users/me/settings");
}

async function fetchProfile(): Promise<UserProfile> {
  return api<UserProfile>("/users/me");
}

async function fetchContentRestrictions(): Promise<ContentRestriction[]> {
  return api<ContentRestriction[]>("/users/me/content-restrictions");
}

async function createContentRestriction(data: {
  restrictionType: string;
  value: string;
  mode: string;
}): Promise<void> {
  await api("/users/me/content-restrictions", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

async function deleteContentRestriction(id: number): Promise<void> {
  await api(`/users/me/content-restrictions/${id}`, {
    method: "DELETE",
  });
}

// --- Profile Section ---

const ProfileSection: Component = () => {
  const [profile] = createResource(fetchProfile);
  const [form, setForm] = createStore({ name: "", email: "" });
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal("");

  // Initialize form once when profile data arrives.
  createEffect(() => {
    const p = profile();
    if (p) {
      setForm(produce((s) => {
        s.name = p.name ?? "";
        s.email = p.email ?? "";
      }));
    }
  });

  async function handleSave(e: Event) {
    e.preventDefault();
    setLoading(true);
    setError("");
    try {
      await api("/users/me", {
        method: "PATCH",
        body: JSON.stringify({
          name: form.name || null,
          email: form.email || null,
        }),
      });
      showToast(t("common.profileUpdated"), "success");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("common.failedToUpdateProfile"));
    } finally {
      setLoading(false);
    }
  }

  return (
    <section class="rounded-xl border border-slate-700 bg-slate-800/50 p-6">
      <h2 class="mb-4 flex items-center gap-2 text-base font-semibold text-slate-100">
        <User class="h-5 w-5 text-indigo-400" />
        {t("common.profile")}
      </h2>
      <Show when={!profile.loading} fallback={<p class="text-sm text-slate-400">{t("common.loading")}</p>}>
        <form onSubmit={handleSave} class="flex flex-col gap-4">
          <div class="flex flex-col gap-1.5">
            <label class="text-sm font-medium text-slate-300">{t("common.username")}</label>
            <p class="rounded-lg border border-slate-700 bg-slate-800 px-3 py-2 text-sm text-slate-400">
              @{profile()?.username}
            </p>
          </div>
          <Input
            label={t("common.displayName")}
            value={form.name}
            onInput={(e) => setForm("name", e.currentTarget.value)}
            placeholder={t("common.yourName")}
          />
          <Input
            label={t("common.email")}
            type="email"
            value={form.email}
            onInput={(e) => setForm("email", e.currentTarget.value)}
            placeholder={t("common.youAtExample")}
          />
          <Show when={error()}>
            <p class="text-sm text-red-400">{error()}</p>
          </Show>
          <div class="flex justify-end">
            <Button type="submit" loading={loading()}>
              <Save class="h-4 w-4" />
              {t("common.saveProfile")}
            </Button>
          </div>
        </form>
      </Show>
    </section>
  );
};

// --- Password Section ---

const PasswordSection: Component = () => {
  const [form, setForm] = createStore({
    currentPassword: "",
    newPassword: "",
    confirmPassword: "",
  });
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal("");

  async function handleSave(e: Event) {
    e.preventDefault();
    if (form.newPassword !== form.confirmPassword) {
      setError(t("common.newPasswordsDoNotMatch"));
      return;
    }
    if (form.newPassword.length < 6) {
      setError(t("common.passwordMinLength"));
      return;
    }
    setLoading(true);
    setError("");
    try {
      await api("/users/me/password", {
        method: "PATCH",
        body: JSON.stringify({
          currentPassword: form.currentPassword,
          newPassword: form.newPassword,
        }),
      });
      setForm(produce((s) => {
        s.currentPassword = "";
        s.newPassword = "";
        s.confirmPassword = "";
      }));
      showToast(t("common.passwordChanged"), "success");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("common.failedToChangePassword"));
    } finally {
      setLoading(false);
    }
  }

  return (
    <section class="rounded-xl border border-slate-700 bg-slate-800/50 p-6">
      <h2 class="mb-4 flex items-center gap-2 text-base font-semibold text-slate-100">
        <Lock class="h-5 w-5 text-indigo-400" />
        {t("common.password")}
      </h2>
      <form onSubmit={handleSave} class="flex flex-col gap-4">
        <Input
          label={t("common.currentPassword")}
          type="password"
          value={form.currentPassword}
          onInput={(e) => setForm("currentPassword", e.currentTarget.value)}
          placeholder="••••••••"
          required
        />
        <Input
          label={t("common.newPassword")}
          type="password"
          value={form.newPassword}
          onInput={(e) => setForm("newPassword", e.currentTarget.value)}
          placeholder="••••••••"
          required
        />
        <Input
          label={t("common.confirmNewPassword")}
          type="password"
          value={form.confirmPassword}
          onInput={(e) => setForm("confirmPassword", e.currentTarget.value)}
          placeholder="••••••••"
          required
        />
        <Show when={error()}>
          <p class="text-sm text-red-400">{error()}</p>
        </Show>
        <div class="flex justify-end">
          <Button type="submit" loading={loading()}>
            <Save class="h-4 w-4" />
            {t("common.changePassword")}
          </Button>
        </div>
      </form>
    </section>
  );
};

// --- Appearance Section ---

const AppearanceSection: Component = () => {
  const [settings] = createResource(fetchSettings);
  const [form, setForm] = createStore({
    theme: "dark",
    bookCardsPerRow: 4,
  });
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal("");

  // Initialize form once when settings data arrives.
  createEffect(() => {
    const s = settings();
    if (s !== undefined) {
      setForm(produce((f) => {
        f.theme = s.theme ?? "dark";
        f.bookCardsPerRow = s.bookCardsPerRow ?? 4;
      }));
    }
  });

  async function handleSave(e: Event) {
    e.preventDefault();
    setLoading(true);
    setError("");
    try {
      await api("/users/me/settings", {
        method: "PUT",
        body: JSON.stringify({
          theme: form.theme,
          bookCardsPerRow: form.bookCardsPerRow,
        }),
      });
      showToast(t("common.appearanceSaved"), "success");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("common.failedToSaveSettings"));
    } finally {
      setLoading(false);
    }
  }

  return (
    <section class="rounded-xl border border-slate-700 bg-slate-800/50 p-6">
      <h2 class="mb-4 flex items-center gap-2 text-base font-semibold text-slate-100">
        <Palette class="h-5 w-5 text-indigo-400" />
        {t("common.appearance")}
      </h2>
      <Show when={!settings.loading} fallback={<p class="text-sm text-slate-400">{t("common.loading")}</p>}>
        <form onSubmit={handleSave} class="flex flex-col gap-4">
              <div class="flex flex-col gap-1.5">
                <label class="text-sm font-medium text-slate-300">{t("common.theme")}</label>
                <div class="flex gap-3">
                  <ThemeOption
                    value="dark"
                    label={t("common.dark")}
                    selected={form.theme === "dark"}
                    onSelect={() => setForm("theme", "dark")}
                  />
                  <ThemeOption
                    value="light"
                    label={t("common.light")}
                    selected={form.theme === "light"}
                    onSelect={() => setForm("theme", "light")}
                  />
                  <ThemeOption
                    value="system"
                    label={t("common.system")}
                    selected={form.theme === "system"}
                    onSelect={() => setForm("theme", "system")}
                  />
                </div>
              </div>

              <div class="flex flex-col gap-2">
                <div class="flex items-center justify-between">
                  <label class="text-sm font-medium text-slate-300">
                    {t("common.booksPerRow")}
                  </label>
                  <span class="text-sm font-medium text-indigo-400">
                    {form.bookCardsPerRow}
                  </span>
                </div>
                <input
                  type="range"
                  min="2"
                  max="8"
                  step="1"
                  value={form.bookCardsPerRow}
                  onInput={(e) =>
                    setForm("bookCardsPerRow", parseInt(e.currentTarget.value, 10))
                  }
                  class="w-full accent-indigo-500"
                />
                <div class="flex justify-between text-xs text-slate-500">
                  <span>2</span>
                  <span>8</span>
                </div>
              </div>

              <Show when={error()}>
                <p class="text-sm text-red-400">{error()}</p>
              </Show>
              <div class="flex justify-end">
                <Button type="submit" loading={loading()}>
                  <Save class="h-4 w-4" />
                  {t("common.saveAppearance")}
                </Button>
              </div>
            </form>
      </Show>
    </section>
  );
};

// --- Theme Option ---

const ThemeOption: Component<{
  value: string;
  label: string;
  selected: boolean;
  onSelect: () => void;
}> = (props) => (
  <button
    type="button"
    onClick={props.onSelect}
    class={`flex-1 rounded-lg border px-3 py-2 text-sm font-medium transition-colors ${
      props.selected
        ? "border-indigo-500 bg-indigo-600/20 text-indigo-300"
        : "border-slate-700 bg-slate-800 text-slate-400 hover:border-slate-600 hover:text-slate-300"
    }`}
  >
    {props.label}
  </button>
);

// --- Content Restrictions Section ---

const ContentRestrictionsSection: Component = () => {
  const [restrictions, { refetch }] = createResource(fetchContentRestrictions);
  const [form, setForm] = createStore({
    restrictionType: "CATEGORY",
    value: "",
    mode: "EXCLUDE",
  });
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal("");

  async function handleAdd(e: Event) {
    e.preventDefault();
    if (!form.value.trim()) {
      setError(t("common.valueRequired"));
      return;
    }
    setLoading(true);
    setError("");
    try {
      await createContentRestriction({
        restrictionType: form.restrictionType,
        value: form.value.trim(),
        mode: form.mode,
      });
      setForm(produce((s) => {
        s.value = "";
      }));
      showToast(t("common.restrictionAdded"), "success");
      refetch();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("common.failedToAddRestriction"));
    } finally {
      setLoading(false);
    }
  }

  async function handleDelete(id: number) {
    try {
      await deleteContentRestriction(id);
      showToast(t("common.restrictionRemoved"), "success");
      refetch();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("common.failedToRemoveRestriction"));
    }
  }

  return (
    <section class="rounded-xl border border-slate-700 bg-slate-800/50 p-6">
      <h2 class="mb-4 flex items-center gap-2 text-base font-semibold text-slate-100">
        <Shield class="h-5 w-5 text-indigo-400" />
        {t("common.contentRestrictions")}
      </h2>
      <Show when={!restrictions.loading} fallback={<p class="text-sm text-slate-400">{t("common.loading")}</p>}>
        <div class="flex flex-col gap-4">
          <Show when={(restrictions() ?? []).length > 0}>
            <div class="flex flex-col gap-2">
              <For each={restrictions() ?? []}>
                {(r) => (
                  <div class="flex items-center justify-between rounded-lg border border-slate-700 bg-slate-800 px-3 py-2">
                    <div class="flex flex-col">
                      <span class="text-sm font-medium text-slate-200">
                        {r.restrictionType}: {r.value}
                      </span>
                      <span class="text-xs text-slate-400">{r.mode}</span>
                    </div>
                    <button
                      type="button"
                      onClick={() => handleDelete(r.id)}
                      class="rounded p-1 text-slate-400 hover:bg-red-600/20 hover:text-red-400"
                      title={t("common.removeRestriction")}
                    >
                      <Trash2 class="h-4 w-4" />
                    </button>
                  </div>
                )}
              </For>
            </div>
          </Show>

          <form onSubmit={handleAdd} class="flex flex-col gap-3">
            <div class="grid grid-cols-2 gap-3">
              <div class="flex flex-col gap-1.5">
                <label class="text-sm font-medium text-slate-300">{t("common.type")}</label>
                <select
                  value={form.restrictionType}
                  onChange={(e) => setForm("restrictionType", e.currentTarget.value)}
                  class="rounded-lg border border-slate-700 bg-slate-800 px-3 py-2 text-sm text-slate-200"
                >
                  <option value="CATEGORY">{t("common.category")}</option>
                  <option value="TAG">{t("common.tag")}</option>
                  <option value="MOOD">{t("common.mood")}</option>
                  <option value="AGE_RATING">{t("common.ageRating")}</option>
                  <option value="CONTENT_RATING">{t("common.contentRating")}</option>
                </select>
              </div>
              <div class="flex flex-col gap-1.5">
                <label class="text-sm font-medium text-slate-300">{t("common.mode")}</label>
                <select
                  value={form.mode}
                  onChange={(e) => setForm("mode", e.currentTarget.value)}
                  class="rounded-lg border border-slate-700 bg-slate-800 px-3 py-2 text-sm text-slate-200"
                >
                  <option value="EXCLUDE">{t("common.exclude")}</option>
                  <option value="ALLOW_ONLY">{t("common.allowOnly")}</option>
                </select>
              </div>
            </div>
            <Input
              label={t("common.value")}
              value={form.value}
              onInput={(e) => setForm("value", e.currentTarget.value)}
              placeholder={t("common.placeholderGenre")}
            />
            <Show when={error()}>
              <p class="text-sm text-red-400">{error()}</p>
            </Show>
            <div class="flex justify-end">
              <Button type="submit" loading={loading()}>
                <Save class="h-4 w-4" />
                {t("common.addRestriction")}
              </Button>
            </div>
          </form>
        </div>
      </Show>
    </section>
  );
};

// --- Settings Page ---

const SettingsPage: Component = () => {
  return (
    <div class="flex flex-1 flex-col">
      {/* Page header */}
      <div class="border-b border-slate-800 px-6 py-5">
        <h1 class="text-xl font-bold text-slate-100">{t("auth.settingsTitle")}</h1>
        <p class="mt-1 text-sm text-slate-400">
          {t("auth.settingsSubtitle")}
        </p>
      </div>

      {/* Content */}
      <div class="flex-1 p-6">
        <div class="mx-auto max-w-2xl flex flex-col gap-6">
          <ProfileSection />
          <PasswordSection />
          <AppearanceSection />
          <ContentRestrictionsSection />
        </div>
      </div>
    </div>
  );
};

export default SettingsPage;
