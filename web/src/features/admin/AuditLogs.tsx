import {
  type Component,
  createResource,
  createSignal,
  For,
  Show,
  Suspense,
  ErrorBoundary,
  createMemo,
} from "solid-js";
import { createStore } from "solid-js/store";
import { ClipboardList, ChevronLeft, ChevronRight } from "lucide-solid";
import { api } from "../../shared/api/client";
import Button from "../../shared/ui/Button";
import Input from "../../shared/ui/Input";

// --- Types ---

interface AuditLogItem {
  id: number;
  user_id?: number;
  username?: string;
  action: string;
  resource_type?: string;
  resource_id?: number;
  details?: string;
  ip_address?: string;
  country?: string;
  created_at: string;
}

interface AuditLogsResponse {
  logs: AuditLogItem[];
  total: number;
  page: number;
  size: number;
}

// --- Action options ---

const actionOptions = [
  { value: "", label: "All Actions" },
  { value: "USER_LOGIN", label: "User Login" },
  { value: "USER_LOGOUT", label: "User Logout" },
  { value: "USER_CREATED", label: "User Created" },
  { value: "USER_UPDATED", label: "User Updated" },
  { value: "USER_DELETED", label: "User Deleted" },
  { value: "BOOK_DOWNLOADED", label: "Book Downloaded" },
  { value: "BOOK_METADATA_UPDATED", label: "Metadata Updated" },
  { value: "BOOK_DELETED", label: "Book Deleted" },
  { value: "LIBRARY_CREATED", label: "Library Created" },
  { value: "LIBRARY_UPDATED", label: "Library Updated" },
  { value: "LIBRARY_DELETED", label: "Library Deleted" },
  { value: "LIBRARY_SCANNED", label: "Library Scanned" },
  { value: "SHELF_CREATED", label: "Shelf Created" },
  { value: "SHELF_DELETED", label: "Shelf Deleted" },
];

// --- API helper ---

async function fetchAuditLogs(params: {
  page: number;
  size: number;
  action: string;
  userId: string;
  from: string;
  to: string;
}): Promise<AuditLogsResponse> {
  const query = new URLSearchParams();
  query.set("page", String(params.page));
  query.set("size", String(params.size));
  if (params.action) query.set("action", params.action);
  if (params.userId) query.set("userId", params.userId);
  if (params.from) query.set("from", params.from);
  if (params.to) query.set("to", params.to);
  return api<AuditLogsResponse>(`/admin/audit-logs?${query.toString()}`);
}

// --- Format helpers ---

function formatDate(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleString();
  } catch {
    return iso;
  }
}

function truncate(str: string | undefined, max: number): string {
  if (!str) return "—";
  return str.length > max ? str.slice(0, max) + "…" : str;
}

// --- Main Component ---

const AuditLogs: Component = () => {
  const [filters, setFilters] = createStore({
    action: "",
    userId: "",
    from: "",
    to: "",
  });

  const [appliedFilters, setAppliedFilters] = createSignal({
    action: "",
    userId: "",
    from: "",
    to: "",
  });

  const [page, setPage] = createSignal(1);
  const [size] = createSignal(50);

  const params = createMemo(() => ({
    page: page(),
    size: size(),
    action: appliedFilters().action,
    userId: appliedFilters().userId,
    from: appliedFilters().from,
    to: appliedFilters().to,
  }));

  const [data, { refetch }] = createResource(params, fetchAuditLogs);

  const totalPages = createMemo(() => {
    const total = data()?.total ?? 0;
    return Math.max(1, Math.ceil(total / size()));
  });

  function applyFilters() {
    setAppliedFilters({
      action: filters.action,
      userId: filters.userId,
      from: filters.from,
      to: filters.to,
    });
    setPage(1);
    refetch();
  }

  function goToPage(p: number) {
    if (p < 1 || p > totalPages()) return;
    setPage(p);
  }

  return (
    <div class="flex flex-1 flex-col">
      {/* Page header */}
      <div class="flex items-center justify-between border-b border-slate-800 px-6 py-5">
        <div>
          <h1 class="text-xl font-bold text-slate-100">Audit Logs</h1>
          <p class="mt-1 text-sm text-slate-400">
            Track significant actions across the system
          </p>
        </div>
      </div>

      {/* Filters */}
      <div class="border-b border-slate-800 px-6 py-4">
        <div class="flex flex-wrap items-end gap-3">
          <div class="flex flex-col gap-1.5">
            <label class="text-xs font-medium text-slate-400">Action</label>
            <select
              value={filters.action}
              onChange={(e) => setFilters("action", e.currentTarget.value)}
              class="rounded-lg border border-slate-700 bg-slate-800 px-3 py-2 text-sm text-slate-100 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 focus:ring-offset-slate-900"
            >
              <For each={actionOptions}>
                {(opt) => <option value={opt.value}>{opt.label}</option>}
              </For>
            </select>
          </div>

          <div class="w-32">
            <Input
              label="User ID"
              type="number"
              value={filters.userId}
              onInput={(e) => setFilters("userId", e.currentTarget.value)}
              placeholder="ID"
            />
          </div>

          <div class="flex flex-col gap-1.5">
            <label class="text-xs font-medium text-slate-400">From</label>
            <input
              type="date"
              value={filters.from}
              onInput={(e) => setFilters("from", e.currentTarget.value)}
              class="rounded-lg border border-slate-700 bg-slate-800 px-3 py-2 text-sm text-slate-100 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 focus:ring-offset-slate-900"
            />
          </div>

          <div class="flex flex-col gap-1.5">
            <label class="text-xs font-medium text-slate-400">To</label>
            <input
              type="date"
              value={filters.to}
              onInput={(e) => setFilters("to", e.currentTarget.value)}
              class="rounded-lg border border-slate-700 bg-slate-800 px-3 py-2 text-sm text-slate-100 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 focus:ring-offset-slate-900"
            />
          </div>

          <Button onClick={applyFilters}>Filter</Button>
        </div>
      </div>

      {/* Content */}
      <div class="flex-1 p-6">
        <ErrorBoundary
          fallback={(err) => (
            <div class="flex flex-col items-center justify-center gap-3 py-20 text-center">
              <p class="text-lg font-medium text-red-400">Failed to load audit logs</p>
              <p class="text-sm text-slate-500">{err.message}</p>
            </div>
          )}
        >
          <Suspense fallback={<p class="text-slate-400">Loading audit logs...</p>}>
            <Show when={!data.loading} fallback={<p class="text-slate-400">Loading audit logs...</p>}>
              <Show
                when={(data()?.logs ?? []).length > 0}
                fallback={
                  <div class="flex flex-col items-center justify-center gap-4 py-20 text-center">
                    <ClipboardList class="h-16 w-16 text-slate-600" />
                    <p class="text-lg font-medium text-slate-300">No audit logs found</p>
                  </div>
                }
              >
                <div class="overflow-x-auto rounded-xl border border-slate-700">
                  <table class="w-full text-sm">
                    <thead>
                      <tr class="border-b border-slate-700 bg-slate-800/50">
                        <th class="px-4 py-3 text-left font-medium text-slate-400">Action</th>
                        <th class="px-4 py-3 text-left font-medium text-slate-400">User</th>
                        <th class="px-4 py-3 text-left font-medium text-slate-400">Resource</th>
                        <th class="px-4 py-3 text-left font-medium text-slate-400">Details</th>
                        <th class="px-4 py-3 text-left font-medium text-slate-400">IP</th>
                        <th class="px-4 py-3 text-left font-medium text-slate-400">Date</th>
                      </tr>
                    </thead>
                    <tbody>
                      <For each={data()?.logs}>
                        {(log) => (
                          <tr class="border-b border-slate-800 hover:bg-slate-800/30 transition-colors">
                            <td class="px-4 py-3">
                              <span class="inline-flex items-center rounded-full bg-slate-700 px-2 py-0.5 text-xs font-medium text-slate-200">
                                {log.action}
                              </span>
                            </td>
                            <td class="px-4 py-3 text-slate-300">
                              {log.username ?? <span class="text-slate-500">—</span>}
                              {log.user_id != null && (
                                <span class="ml-1 text-xs text-slate-500">(#{log.user_id})</span>
                              )}
                            </td>
                            <td class="px-4 py-3 text-slate-300">
                              {log.resource_type ? (
                                <span>
                                  {log.resource_type}
                                  {log.resource_id != null && ` #${log.resource_id}`}
                                </span>
                              ) : (
                                <span class="text-slate-500">—</span>
                              )}
                            </td>
                            <td class="px-4 py-3 text-slate-300 max-w-xs truncate" title={log.details}>
                              {truncate(log.details, 60)}
                            </td>
                            <td class="px-4 py-3 text-slate-300">
                              {log.ip_address ?? <span class="text-slate-500">—</span>}
                            </td>
                            <td class="px-4 py-3 text-slate-300 whitespace-nowrap">
                              {formatDate(log.created_at)}
                            </td>
                          </tr>
                        )}
                      </For>
                    </tbody>
                  </table>
                </div>

                {/* Pagination */}
                <div class="mt-4 flex items-center justify-between">
                  <p class="text-sm text-slate-400">
                    Page {page()} of {totalPages()} ({data()?.total ?? 0} total)
                  </p>
                  <div class="flex items-center gap-2">
                    <Button
                      variant="secondary"
                      onClick={() => goToPage(page() - 1)}
                      disabled={page() <= 1}
                    >
                      <ChevronLeft class="h-4 w-4" />
                      Prev
                    </Button>
                    <Button
                      variant="secondary"
                      onClick={() => goToPage(page() + 1)}
                      disabled={page() >= totalPages()}
                    >
                      Next
                      <ChevronRight class="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              </Show>
            </Show>
          </Suspense>
        </ErrorBoundary>
      </div>
    </div>
  );
};

export default AuditLogs;
