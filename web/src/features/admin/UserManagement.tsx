import {
  type Component,
  createResource,
  createSignal,
  createMemo,
  createEffect,
  For,
  Show,
  Suspense,
  ErrorBoundary,
} from "solid-js";
import { createStore, produce } from "solid-js/store";
import {
  Users,
  Plus,
  Pencil,
  Trash2,
  KeyRound,
  Shield,
  Library,
  Check,
  X,
} from "lucide-solid";
import { api } from "../../shared/api/client";
import { useAuth } from "../auth/AuthProvider";
import Button from "../../shared/ui/Button";
import Input from "../../shared/ui/Input";
import { showToast } from "../../shared/ui/Toast";

// --- Types ---

interface UserPermissions {
  role: string;
  canDownload: boolean;
  canUpload: boolean;
  canEmailSend: boolean;
  canEditMetadata: boolean;
  opdsAccess: boolean;
}

interface UserItem {
  id: number;
  username: string;
  email?: string;
  name?: string;
  enabled: boolean;
  createdAt: string;
  permissions?: UserPermissions;
  libraryIds?: number[];
}

interface LibraryItem {
  id: number;
  name: string;
}

// --- API helpers ---

async function fetchUsers(): Promise<UserItem[]> {
  return api<UserItem[]>("/admin/users");
}

async function fetchLibraries(): Promise<LibraryItem[]> {
  return api<LibraryItem[]>("/libraries");
}

async function fetchUser(id: number): Promise<UserItem> {
  return api<UserItem>(`/admin/users/${id}`);
}

// --- Create User Dialog ---

interface CreateUserDialogProps {
  onClose: () => void;
  onCreated: () => void;
}

const CreateUserDialog: Component<CreateUserDialogProps> = (props) => {
  const [form, setForm] = createStore({
    username: "",
    password: "",
    name: "",
    email: "",
    role: "USER",
  });
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal("");

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!form.username || !form.password) {
      setError("Username and password are required");
      return;
    }
    setLoading(true);
    setError("");
    try {
      await api("/admin/users", {
        method: "POST",
        body: JSON.stringify({
          username: form.username,
          password: form.password,
          name: form.name,
          email: form.email,
          role: form.role,
        }),
      });
      showToast(`User "${form.username}" created`, "success");
      props.onCreated();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to create user");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
      <div class="w-full max-w-md rounded-xl bg-slate-900 border border-slate-700 shadow-2xl">
        <div class="flex items-center justify-between border-b border-slate-700 px-6 py-4">
          <h2 class="text-lg font-semibold text-slate-100">Create User</h2>
          <button
            onClick={props.onClose}
            class="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors"
          >
            <X class="h-5 w-5" />
          </button>
        </div>
        <form onSubmit={handleSubmit} class="flex flex-col gap-4 p-6">
          <Input
            label="Username"
            value={form.username}
            onInput={(e) => setForm("username", e.currentTarget.value)}
            placeholder="johndoe"
            required
          />
          <Input
            label="Password"
            type="password"
            value={form.password}
            onInput={(e) => setForm("password", e.currentTarget.value)}
            placeholder="••••••••"
            required
          />
          <Input
            label="Name (optional)"
            value={form.name}
            onInput={(e) => setForm("name", e.currentTarget.value)}
            placeholder="John Doe"
          />
          <Input
            label="Email (optional)"
            type="email"
            value={form.email}
            onInput={(e) => setForm("email", e.currentTarget.value)}
            placeholder="john@example.com"
          />
          <div class="flex flex-col gap-1.5">
            <label class="text-sm font-medium text-slate-300">Role</label>
            <select
              value={form.role}
              onChange={(e) => setForm("role", e.currentTarget.value)}
              class="rounded-lg border border-slate-700 bg-slate-800 px-3 py-2 text-sm text-slate-100 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 focus:ring-offset-slate-900"
            >
              <option value="USER">User</option>
              <option value="ADMIN">Admin</option>
            </select>
          </div>
          <Show when={error()}>
            <p class="text-sm text-red-400">{error()}</p>
          </Show>
          <div class="flex justify-end gap-3 pt-2">
            <Button variant="secondary" type="button" onClick={props.onClose}>
              Cancel
            </Button>
            <Button type="submit" loading={loading()}>
              Create User
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
};

// --- Edit User Dialog ---

interface EditUserDialogProps {
  userId: number;
  onClose: () => void;
  onSaved: () => void;
}

const EditUserDialog: Component<EditUserDialogProps> = (props) => {
  const [user, { refetch }] = createResource(() => props.userId, fetchUser);
  const [libraries] = createResource(fetchLibraries);

  const [form, setForm] = createStore({
    name: "",
    email: "",
    enabled: true,
    role: "USER",
    canDownload: false,
    canUpload: false,
    canEmailSend: false,
    canEditMetadata: false,
    opdsAccess: false,
    libraryIds: [] as number[],
  });
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal("");

  // Initialize form once when user data loads.
  createEffect(() => {
    const u = user();
    if (u) {
      setForm(produce((s) => {
        s.name = u.name ?? "";
        s.email = u.email ?? "";
        s.enabled = u.enabled;
        s.role = u.permissions?.role ?? "USER";
        s.canDownload = u.permissions?.canDownload ?? false;
        s.canUpload = u.permissions?.canUpload ?? false;
        s.canEmailSend = u.permissions?.canEmailSend ?? false;
        s.canEditMetadata = u.permissions?.canEditMetadata ?? false;
        s.opdsAccess = u.permissions?.opdsAccess ?? false;
        s.libraryIds = u.libraryIds ?? [];
      }));
    }
  });

  function toggleLibrary(id: number) {
    setForm(produce((s) => {
      const idx = s.libraryIds.indexOf(id);
      if (idx >= 0) {
        s.libraryIds.splice(idx, 1);
      } else {
        s.libraryIds.push(id);
      }
    }));
  }

  async function handleSave() {
    setLoading(true);
    setError("");
    try {
      const id = props.userId;
      // Update profile.
      await api(`/admin/users/${id}`, {
        method: "PUT",
        body: JSON.stringify({
          name: form.name || null,
          email: form.email || null,
          enabled: form.enabled,
        }),
      });
      // Update permissions.
      await api(`/admin/users/${id}/permissions`, {
        method: "PUT",
        body: JSON.stringify({
          role: form.role,
          canDownload: form.canDownload,
          canUpload: form.canUpload,
          canEmailSend: form.canEmailSend,
          canEditMetadata: form.canEditMetadata,
          opdsAccess: form.opdsAccess,
        }),
      });
      // Update library access.
      await api(`/admin/users/${id}/libraries`, {
        method: "PUT",
        body: JSON.stringify({ libraryIds: form.libraryIds }),
      });
      showToast("User updated", "success");
      props.onSaved();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to save changes");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
      <div class="w-full max-w-lg rounded-xl bg-slate-900 border border-slate-700 shadow-2xl max-h-[90vh] flex flex-col">
        <div class="flex items-center justify-between border-b border-slate-700 px-6 py-4 shrink-0">
          <h2 class="text-lg font-semibold text-slate-100">
            Edit User
            <Show when={user()}>
              <span class="ml-2 text-sm font-normal text-slate-400">
                @{user()!.username}
              </span>
            </Show>
          </h2>
          <button
            onClick={props.onClose}
            class="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors"
          >
            <X class="h-5 w-5" />
          </button>
        </div>

        <div class="flex-1 overflow-y-auto p-6">
          <Show when={!user.loading} fallback={<p class="text-slate-400">Loading...</p>}>
            <div class="flex flex-col gap-6">
              {/* Profile */}
              <section>
                <h3 class="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-slate-500">
                  <Users class="h-4 w-4" />
                  Profile
                </h3>
                <div class="flex flex-col gap-3">
                  <Input
                    label="Name"
                    value={form.name}
                    onInput={(e) => setForm("name", e.currentTarget.value)}
                    placeholder="Full name"
                  />
                  <Input
                    label="Email"
                    type="email"
                    value={form.email}
                    onInput={(e) => setForm("email", e.currentTarget.value)}
                    placeholder="user@example.com"
                  />
                  <div class="flex items-center gap-3">
                    <input
                      type="checkbox"
                      id="enabled"
                      checked={form.enabled}
                      onChange={(e) => setForm("enabled", e.currentTarget.checked)}
                      class="h-4 w-4 rounded border-slate-600 bg-slate-800 text-indigo-600 focus:ring-indigo-500"
                    />
                    <label for="enabled" class="text-sm text-slate-300">
                      Account enabled
                    </label>
                  </div>
                </div>
              </section>

              {/* Permissions */}
              <section>
                <h3 class="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-slate-500">
                  <Shield class="h-4 w-4" />
                  Permissions
                </h3>
                <div class="flex flex-col gap-3">
                  <div class="flex flex-col gap-1.5">
                    <label class="text-sm font-medium text-slate-300">Role</label>
                    <select
                      value={form.role}
                      onChange={(e) => setForm("role", e.currentTarget.value)}
                      class="rounded-lg border border-slate-700 bg-slate-800 px-3 py-2 text-sm text-slate-100 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 focus:ring-offset-slate-900"
                    >
                      <option value="USER">User</option>
                      <option value="ADMIN">Admin</option>
                    </select>
                  </div>
                  <div class="grid grid-cols-2 gap-2">
                    <PermCheckbox
                      id="canDownload"
                      label="Can Download"
                      checked={form.canDownload}
                      onChange={(v) => setForm("canDownload", v)}
                    />
                    <PermCheckbox
                      id="canUpload"
                      label="Can Upload"
                      checked={form.canUpload}
                      onChange={(v) => setForm("canUpload", v)}
                    />
                    <PermCheckbox
                      id="canEmailSend"
                      label="Can Email"
                      checked={form.canEmailSend}
                      onChange={(v) => setForm("canEmailSend", v)}
                    />
                    <PermCheckbox
                      id="canEditMetadata"
                      label="Edit Metadata"
                      checked={form.canEditMetadata}
                      onChange={(v) => setForm("canEditMetadata", v)}
                    />
                    <PermCheckbox
                      id="opdsAccess"
                      label="OPDS Access"
                      checked={form.opdsAccess}
                      onChange={(v) => setForm("opdsAccess", v)}
                    />
                  </div>
                </div>
              </section>

              {/* Library Access */}
              <section>
                <h3 class="mb-3 flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-slate-500">
                  <Library class="h-4 w-4" />
                  Library Access
                </h3>
                <Show
                  when={!libraries.loading}
                  fallback={<p class="text-sm text-slate-400">Loading libraries...</p>}
                >
                  <Show
                    when={(libraries() ?? []).length > 0}
                    fallback={<p class="text-sm text-slate-400">No libraries available</p>}
                  >
                    <div class="flex flex-col gap-2">
                      <For each={libraries()}>
                        {(lib) => {
                          const isSelected = createMemo(() =>
                            form.libraryIds.includes(lib.id)
                          );
                          return (
                            <button
                              type="button"
                              onClick={() => toggleLibrary(lib.id)}
                              class={`flex items-center gap-3 rounded-lg border px-3 py-2 text-sm transition-colors ${
                                isSelected()
                                  ? "border-indigo-500 bg-indigo-600/20 text-indigo-300"
                                  : "border-slate-700 bg-slate-800 text-slate-300 hover:border-slate-600"
                              }`}
                            >
                              <Show
                                when={isSelected()}
                                fallback={
                                  <div class="h-4 w-4 rounded border border-slate-600" />
                                }
                              >
                                <Check class="h-4 w-4 text-indigo-400" />
                              </Show>
                              {lib.name}
                            </button>
                          );
                        }}
                      </For>
                    </div>
                  </Show>
                </Show>
              </section>
            </div>
          </Show>
        </div>

        <div class="border-t border-slate-700 px-6 py-4 shrink-0">
          <Show when={error()}>
            <p class="mb-3 text-sm text-red-400">{error()}</p>
          </Show>
          <div class="flex justify-end gap-3">
            <Button variant="secondary" onClick={props.onClose}>
              Cancel
            </Button>
            <Button onClick={handleSave} loading={loading()}>
              Save Changes
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
};

// --- Reset Password Dialog ---

interface ResetPasswordDialogProps {
  userId: number;
  username: string;
  onClose: () => void;
}

const ResetPasswordDialog: Component<ResetPasswordDialogProps> = (props) => {
  const [password, setPassword] = createSignal("");
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal("");

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (password().length < 6) {
      setError("Password must be at least 6 characters");
      return;
    }
    setLoading(true);
    setError("");
    try {
      await api(`/admin/users/${props.userId}/password`, {
        method: "PUT",
        body: JSON.stringify({ password: password() }),
      });
      showToast(`Password reset for @${props.username}`, "success");
      props.onClose();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to reset password");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
      <div class="w-full max-w-sm rounded-xl bg-slate-900 border border-slate-700 shadow-2xl">
        <div class="flex items-center justify-between border-b border-slate-700 px-6 py-4">
          <h2 class="text-lg font-semibold text-slate-100">Reset Password</h2>
          <button
            onClick={props.onClose}
            class="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors"
          >
            <X class="h-5 w-5" />
          </button>
        </div>
        <form onSubmit={handleSubmit} class="flex flex-col gap-4 p-6">
          <p class="text-sm text-slate-400">
            Set a new password for <span class="font-medium text-slate-200">@{props.username}</span>.
          </p>
          <Input
            label="New Password"
            type="password"
            value={password()}
            onInput={(e) => setPassword(e.currentTarget.value)}
            placeholder="••••••••"
            required
          />
          <Show when={error()}>
            <p class="text-sm text-red-400">{error()}</p>
          </Show>
          <div class="flex justify-end gap-3 pt-2">
            <Button variant="secondary" type="button" onClick={props.onClose}>
              Cancel
            </Button>
            <Button type="submit" loading={loading()}>
              Reset Password
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
};

// --- Delete Confirm Dialog ---

interface DeleteConfirmDialogProps {
  userId: number;
  username: string;
  onClose: () => void;
  onDeleted: () => void;
}

const DeleteConfirmDialog: Component<DeleteConfirmDialogProps> = (props) => {
  const [loading, setLoading] = createSignal(false);

  async function handleDelete() {
    setLoading(true);
    try {
      await api(`/admin/users/${props.userId}`, { method: "DELETE" });
      showToast(`User @${props.username} deleted`, "success");
      props.onDeleted();
    } catch (err: unknown) {
      showToast(
        err instanceof Error ? err.message : "Failed to delete user",
        "error"
      );
    } finally {
      setLoading(false);
    }
  }

  return (
    <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
      <div class="w-full max-w-sm rounded-xl bg-slate-900 border border-slate-700 shadow-2xl">
        <div class="flex items-center justify-between border-b border-slate-700 px-6 py-4">
          <h2 class="text-lg font-semibold text-slate-100">Delete User</h2>
          <button
            onClick={props.onClose}
            class="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors"
          >
            <X class="h-5 w-5" />
          </button>
        </div>
        <div class="flex flex-col gap-4 p-6">
          <p class="text-sm text-slate-300">
            Are you sure you want to delete{" "}
            <span class="font-medium text-slate-100">@{props.username}</span>?
            This action cannot be undone.
          </p>
          <div class="flex justify-end gap-3">
            <Button variant="secondary" onClick={props.onClose}>
              Cancel
            </Button>
            <Button variant="danger" onClick={handleDelete} loading={loading()}>
              Delete User
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
};

// --- Permission Checkbox helper ---

const PermCheckbox: Component<{
  id: string;
  label: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}> = (props) => (
  <div class="flex items-center gap-2">
    <input
      type="checkbox"
      id={props.id}
      checked={props.checked}
      onChange={(e) => props.onChange(e.currentTarget.checked)}
      class="h-4 w-4 rounded border-slate-600 bg-slate-800 text-indigo-600 focus:ring-indigo-500"
    />
    <label for={props.id} class="text-sm text-slate-300">
      {props.label}
    </label>
  </div>
);

// --- Role Badge ---

const RoleBadge: Component<{ role: string }> = (props) => (
  <span
    class={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${
      props.role === "ADMIN"
        ? "bg-indigo-600/20 text-indigo-300"
        : "bg-slate-700 text-slate-300"
    }`}
  >
    {props.role}
  </span>
);

// --- User Table ---

type DialogState =
  | { type: "none" }
  | { type: "create" }
  | { type: "edit"; userId: number }
  | { type: "resetPassword"; userId: number; username: string }
  | { type: "delete"; userId: number; username: string };

// UserManagement is the main page component. It owns the users resource and
// all dialog state so that refetch() always refreshes the visible table.
const UserManagement: Component = () => {
  const auth = useAuth();
  const [users, { refetch }] = createResource(fetchUsers);
  const [dialog, setDialog] = createSignal<DialogState>({ type: "none" });

  const currentUserId = createMemo(() => auth.user()?.id ?? -1);

  function closeDialog() {
    setDialog({ type: "none" });
  }

  function handleMutated() {
    closeDialog();
    refetch();
  }

  return (
    <div class="flex flex-1 flex-col">
      {/* Page header */}
      <div class="flex items-center justify-between border-b border-slate-800 px-6 py-5">
        <div>
          <h1 class="text-xl font-bold text-slate-100">User Management</h1>
          <p class="mt-1 text-sm text-slate-400">
            Manage users, roles, and permissions
          </p>
        </div>
        <Button onClick={() => setDialog({ type: "create" })}>
          <Plus class="h-4 w-4" />
          Create User
        </Button>
      </div>

      {/* Content */}
      <div class="flex-1 p-6">
        <ErrorBoundary
          fallback={(err) => (
            <div class="flex flex-col items-center justify-center gap-3 py-20 text-center">
              <p class="text-lg font-medium text-red-400">Failed to load users</p>
              <p class="text-sm text-slate-500">{err.message}</p>
            </div>
          )}
        >
          <Suspense fallback={<p class="text-slate-400">Loading users...</p>}>
            <Show when={!users.loading} fallback={<p class="text-slate-400">Loading users...</p>}>
              <Show
                when={(users() ?? []).length > 0}
                fallback={
                  <div class="flex flex-col items-center justify-center gap-4 py-20 text-center">
                    <Users class="h-16 w-16 text-slate-600" />
                    <p class="text-lg font-medium text-slate-300">No users yet</p>
                  </div>
                }
              >
                <div class="overflow-x-auto rounded-xl border border-slate-700">
                  <table class="w-full text-sm">
                    <thead>
                      <tr class="border-b border-slate-700 bg-slate-800/50">
                        <th class="px-4 py-3 text-left font-medium text-slate-400">Username</th>
                        <th class="px-4 py-3 text-left font-medium text-slate-400">Name</th>
                        <th class="px-4 py-3 text-left font-medium text-slate-400">Email</th>
                        <th class="px-4 py-3 text-left font-medium text-slate-400">Role</th>
                        <th class="px-4 py-3 text-left font-medium text-slate-400">Status</th>
                        <th class="px-4 py-3 text-right font-medium text-slate-400">Actions</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={users()}>
                        {(user) => (
                          <tr class="border-b border-slate-800 hover:bg-slate-800/30 transition-colors">
                            <td class="px-4 py-3 font-medium text-slate-200">
                              @{user.username}
                            </td>
                            <td class="px-4 py-3 text-slate-300">
                              {user.name ?? <span class="text-slate-500">—</span>}
                            </td>
                            <td class="px-4 py-3 text-slate-300">
                              {user.email ?? <span class="text-slate-500">—</span>}
                            </td>
                            <td class="px-4 py-3">
                              <Show when={user.permissions}>
                                <RoleBadge role={user.permissions!.role} />
                              </Show>
                            </td>
                            <td class="px-4 py-3">
                              <span
                                class={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${
                                  user.enabled
                                    ? "bg-emerald-600/20 text-emerald-300"
                                    : "bg-red-600/20 text-red-300"
                                }`}
                              >
                                {user.enabled ? "Active" : "Disabled"}
                              </span>
                            </td>
                            <td class="px-4 py-3">
                              <div class="flex items-center justify-end gap-1">
                                <button
                                  onClick={() => setDialog({ type: "edit", userId: user.id })}
                                  title="Edit user"
                                  class="rounded-lg p-1.5 text-slate-400 hover:bg-slate-700 hover:text-slate-200 transition-colors"
                                >
                                  <Pencil class="h-4 w-4" />
                                </button>
                                <button
                                  onClick={() =>
                                    setDialog({ type: "resetPassword", userId: user.id, username: user.username })
                                  }
                                  title="Reset password"
                                  class="rounded-lg p-1.5 text-slate-400 hover:bg-slate-700 hover:text-slate-200 transition-colors"
                                >
                                  <KeyRound class="h-4 w-4" />
                                </button>
                                <Show when={user.id !== currentUserId()}>
                                  <button
                                    onClick={() =>
                                      setDialog({ type: "delete", userId: user.id, username: user.username })
                                    }
                                    title="Delete user"
                                    class="rounded-lg p-1.5 text-slate-400 hover:bg-red-900/50 hover:text-red-300 transition-colors"
                                  >
                                    <Trash2 class="h-4 w-4" />
                                  </button>
                                </Show>
                              </div>
                            </td>
                          </tr>
                        )}
                      </For>
                    </tbody>
                  </table>
                </div>
              </Show>
            </Show>
          </Suspense>
        </ErrorBoundary>
      </div>

      {/* Dialogs */}
      <Show when={dialog().type === "create"}>
        <CreateUserDialog onClose={closeDialog} onCreated={handleMutated} />
      </Show>
      <Show when={dialog().type === "edit"}>
        <EditUserDialog
          userId={(dialog() as { type: "edit"; userId: number }).userId}
          onClose={closeDialog}
          onSaved={handleMutated}
        />
      </Show>
      <Show when={dialog().type === "resetPassword"}>
        {(() => {
          const d = dialog() as { type: "resetPassword"; userId: number; username: string };
          return (
            <ResetPasswordDialog
              userId={d.userId}
              username={d.username}
              onClose={closeDialog}
            />
          );
        })()}
      </Show>
      <Show when={dialog().type === "delete"}>
        {(() => {
          const d = dialog() as { type: "delete"; userId: number; username: string };
          return (
            <DeleteConfirmDialog
              userId={d.userId}
              username={d.username}
              onClose={closeDialog}
              onDeleted={handleMutated}
            />
          );
        })()}
      </Show>
    </div>
  );
};

export default UserManagement;
