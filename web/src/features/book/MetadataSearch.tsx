import {
  type Component,
  createSignal,
  createResource,
  For,
  Show,
  Suspense,
} from "solid-js";
import { Search, Check, X, Lock, Unlock, ChevronDown, ChevronUp } from "lucide-solid";
import { api } from "../../shared/api/client";
import { useAuth } from "../auth/AuthProvider";
import Button from "../../shared/ui/Button";
import Input from "../../shared/ui/Input";

// ---- Types ----

interface MetadataResult {
  providerID: string;
  provider: string;
  title: string;
  subtitle: string;
  authors: string[];
  description: string;
  publisher: string;
  publishDate: string;
  pageCount: number;
  language: string;
  isbn10: string;
  isbn13: string;
  coverURL: string;
  series: string;
  seriesIndex: number;
  categories: string[];
  tags: string[];
  googleBooksID: string;
}

interface Proposal {
  id: number;
  bookID: number;
  provider: string;
  providerID: string;
  status: string;
  data: MetadataResult;
  createdAt: string;
}

// ---- API functions ----

async function searchMetadata(
  title: string,
  author: string,
  isbn: string,
  bookType: string,
): Promise<Record<string, MetadataResult[]>> {
  const params = new URLSearchParams();
  if (title) params.set("title", title);
  if (author) params.set("author", author);
  if (isbn) params.set("isbn", isbn);
  if (bookType) params.set("bookType", bookType);
  return api<Record<string, MetadataResult[]>>(`/metadata/search?${params.toString()}`);
}

async function fetchProposals(bookId: number): Promise<Proposal[]> {
  return api<Proposal[]>(`/metadata/books/${bookId}/proposals`);
}

async function createProposal(
  bookId: number,
  provider: string,
  providerId: string,
): Promise<{ id: number }> {
  return api<{ id: number }>(`/metadata/books/${bookId}/proposals`, {
    method: "POST",
    body: JSON.stringify({ provider, providerId }),
  });
}

async function acceptProposal(proposalId: number): Promise<void> {
  await api(`/metadata/proposals/${proposalId}/accept`, { method: "POST" });
}

async function rejectProposal(proposalId: number): Promise<void> {
  await api(`/metadata/proposals/${proposalId}/reject`, { method: "POST" });
}

async function toggleFieldLock(
  bookId: number,
  field: string,
  locked: boolean,
): Promise<void> {
  await api(`/metadata/books/${bookId}/lock`, {
    method: "PUT",
    body: JSON.stringify({ field, locked }),
  });
}

// ---- Sub-components ----

const ResultCard: Component<{
  result: MetadataResult;
  bookId: number;
  onProposalCreated: () => void;
}> = (props) => {
  const [creating, setCreating] = createSignal(false);
  const [created, setCreated] = createSignal(false);
  const [expanded, setExpanded] = createSignal(false);

  async function handleUseThis() {
    setCreating(true);
    try {
      await createProposal(props.bookId, props.result.provider, props.result.providerID);
      setCreated(true);
      props.onProposalCreated();
    } finally {
      setCreating(false);
    }
  }

  return (
    <div class="rounded-lg border border-slate-700 bg-slate-800 p-4">
      <div class="flex gap-4">
        {/* Cover thumbnail */}
        <Show when={props.result.coverURL}>
          <img
            src={props.result.coverURL}
            alt={props.result.title}
            class="h-20 w-14 shrink-0 rounded object-cover"
            onError={(e) => {
              (e.currentTarget as HTMLImageElement).style.display = "none";
            }}
          />
        </Show>

        {/* Info */}
        <div class="min-w-0 flex-1">
          <p class="font-medium text-slate-100 leading-tight">
            {props.result.title}
            <Show when={props.result.subtitle}>
              <span class="text-slate-400"> — {props.result.subtitle}</span>
            </Show>
          </p>
          <Show when={props.result.authors?.length > 0}>
            <p class="mt-0.5 text-sm text-slate-400">
              {props.result.authors.join(", ")}
            </p>
          </Show>
          <div class="mt-1 flex flex-wrap gap-x-4 gap-y-0.5 text-xs text-slate-500">
            <Show when={props.result.publisher}>
              <span>{props.result.publisher}</span>
            </Show>
            <Show when={props.result.publishDate}>
              <span>{props.result.publishDate}</span>
            </Show>
            <Show when={props.result.pageCount > 0}>
              <span>{props.result.pageCount} pages</span>
            </Show>
            <Show when={props.result.isbn13}>
              <span>ISBN: {props.result.isbn13}</span>
            </Show>
          </div>

          {/* Description toggle */}
          <Show when={props.result.description}>
            <div class="mt-2">
              <p
                class={`text-xs leading-relaxed text-slate-400 ${
                  expanded() ? "" : "line-clamp-2"
                }`}
              >
                {props.result.description}
              </p>
              <button
                onClick={() => setExpanded((v) => !v)}
                class="mt-0.5 flex items-center gap-0.5 text-xs text-indigo-400 hover:text-indigo-300"
              >
                <Show
                  when={expanded()}
                  fallback={
                    <>
                      More <ChevronDown class="h-3 w-3" />
                    </>
                  }
                >
                  <>
                    Less <ChevronUp class="h-3 w-3" />
                  </>
                </Show>
              </button>
            </div>
          </Show>
        </div>

        {/* Action */}
        <div class="shrink-0">
          <Button
            variant={created() ? "secondary" : "primary"}
            size="sm"
            onClick={handleUseThis}
            loading={creating()}
            disabled={created()}
          >
            <Show when={created()} fallback={<>Use this</>}>
              <Check class="h-4 w-4" />
              Added
            </Show>
          </Button>
        </div>
      </div>
    </div>
  );
};

const ProposalCard: Component<{
  proposal: Proposal;
  canEdit: boolean;
  onAction: () => void;
}> = (props) => {
  const [accepting, setAccepting] = createSignal(false);
  const [rejecting, setRejecting] = createSignal(false);

  async function handleAccept() {
    setAccepting(true);
    try {
      await acceptProposal(props.proposal.id);
      props.onAction();
    } finally {
      setAccepting(false);
    }
  }

  async function handleReject() {
    setRejecting(true);
    try {
      await rejectProposal(props.proposal.id);
      props.onAction();
    } finally {
      setRejecting(false);
    }
  }

  const statusColor = () => {
    switch (props.proposal.status) {
      case "ACCEPTED":
        return "text-emerald-400";
      case "REJECTED":
        return "text-red-400";
      default:
        return "text-amber-400";
    }
  };

  return (
    <div class="rounded-lg border border-slate-700 bg-slate-800/50 p-4">
      <div class="flex items-start justify-between gap-4">
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <p class="font-medium text-slate-200">{props.proposal.data.title}</p>
            <span class={`text-xs font-medium ${statusColor()}`}>
              {props.proposal.status}
            </span>
          </div>
          <Show when={props.proposal.data.authors?.length > 0}>
            <p class="text-sm text-slate-400">
              {props.proposal.data.authors.join(", ")}
            </p>
          </Show>
          <p class="mt-0.5 text-xs text-slate-500">
            via {props.proposal.provider} · {props.proposal.createdAt.slice(0, 10)}
          </p>
        </div>

        <Show when={props.proposal.status === "PENDING" && props.canEdit}>
          <div class="flex shrink-0 gap-2">
            <Button
              variant="primary"
              size="sm"
              onClick={handleAccept}
              loading={accepting()}
            >
              <Check class="h-4 w-4" />
              Accept
            </Button>
            <Button
              variant="danger"
              size="sm"
              onClick={handleReject}
              loading={rejecting()}
            >
              <X class="h-4 w-4" />
              Reject
            </Button>
          </div>
        </Show>
      </div>
    </div>
  );
};

// ---- Lock toggles ----

const lockFields = [
  { key: "title", label: "Title" },
  { key: "subtitle", label: "Subtitle" },
  { key: "description", label: "Description" },
  { key: "publisher", label: "Publisher" },
  { key: "publishDate", label: "Publish Date" },
  { key: "pageCount", label: "Page Count" },
  { key: "language", label: "Language" },
  { key: "isbn10", label: "ISBN-10" },
  { key: "isbn13", label: "ISBN-13" },
  { key: "cover", label: "Cover" },
] as const;

const FieldLocks: Component<{ bookId: number }> = (props) => {
  const [locks, setLocks] = createSignal<Record<string, boolean>>({});
  const [saving, setSaving] = createSignal<string | null>(null);

  async function handleToggle(field: string) {
    const current = locks()[field] ?? false;
    setSaving(field);
    try {
      await toggleFieldLock(props.bookId, field, !current);
      setLocks((prev) => ({ ...prev, [field]: !current }));
    } finally {
      setSaving(null);
    }
  }

  return (
    <div class="flex flex-col gap-2">
      <p class="text-xs font-medium uppercase tracking-wide text-slate-500">
        Field Locks
      </p>
      <div class="flex flex-wrap gap-2">
        <For each={lockFields}>
          {(f) => {
            const isLocked = () => locks()[f.key] ?? false;
            const isSaving = () => saving() === f.key;
            return (
              <button
                onClick={() => void handleToggle(f.key)}
                disabled={isSaving()}
                class={`flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium transition-colors disabled:opacity-50 ${
                  isLocked()
                    ? "bg-amber-600/20 text-amber-300 ring-1 ring-amber-600/40"
                    : "bg-slate-700 text-slate-400 hover:bg-slate-600 hover:text-slate-300"
                }`}
              >
                <Show
                  when={isLocked()}
                  fallback={<Unlock class="h-3 w-3" />}
                >
                  <Lock class="h-3 w-3" />
                </Show>
                {f.label}
              </button>
            );
          }}
        </For>
      </div>
    </div>
  );
};

// ---- Main component ----

interface MetadataSearchProps {
  bookId: number;
  bookType: "EBOOK" | "AUDIOBOOK" | "COMIC";
  onClose: () => void;
}

const MetadataSearch: Component<MetadataSearchProps> = (props) => {
  const auth = useAuth();
  const canEdit = () => auth.isAdmin() || false;

  const [title, setTitle] = createSignal("");
  const [author, setAuthor] = createSignal("");
  const [isbn, setIsbn] = createSignal("");
  const [searching, setSearching] = createSignal(false);
  const [searchResults, setSearchResults] = createSignal<
    Record<string, MetadataResult[]>
  >({});
  const [searchError, setSearchError] = createSignal<string | null>(null);
  const [proposalKey, setProposalKey] = createSignal(0);

  const [proposals] = createResource(
    () => ({ bookId: props.bookId, key: proposalKey() }),
    ({ bookId }) => fetchProposals(bookId),
  );

  const hasResults = () => {
    const r = searchResults();
    return Object.values(r).some((arr) => arr.length > 0);
  };

  async function handleSearch() {
    if (!title() && !author() && !isbn()) {
      setSearchError("Enter at least one search term");
      return;
    }
    setSearchError(null);
    setSearching(true);
    try {
      const results = await searchMetadata(title(), author(), isbn(), props.bookType);
      setSearchResults(results);
    } catch {
      setSearchError("Search failed. Please try again.");
    } finally {
      setSearching(false);
    }
  }

  function handleProposalCreated() {
    setProposalKey((k) => k + 1);
  }

  return (
    <div class="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/60 p-4 pt-16">
      <div class="w-full max-w-2xl rounded-xl border border-slate-700 bg-slate-900 shadow-2xl">
        {/* Header */}
        <div class="flex items-center justify-between border-b border-slate-800 px-6 py-4">
          <h2 class="text-lg font-semibold text-slate-100">Find Metadata</h2>
          <button
            onClick={props.onClose}
            class="text-slate-400 hover:text-slate-200 transition-colors"
          >
            <X class="h-5 w-5" />
          </button>
        </div>

        <div class="flex flex-col gap-6 p-6">
          {/* Search form */}
          <div class="flex flex-col gap-4">
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <Input
                label="Title"
                placeholder="e.g. The Way of Kings"
                value={title()}
                onInput={(e) => setTitle(e.currentTarget.value)}
              />
              <Input
                label="Author"
                placeholder="e.g. Brandon Sanderson"
                value={author()}
                onInput={(e) => setAuthor(e.currentTarget.value)}
              />
            </div>
            <Input
              label="ISBN"
              placeholder="e.g. 9780765326355"
              value={isbn()}
              onInput={(e) => setIsbn(e.currentTarget.value)}
            />
            <Show when={searchError()}>
              <p class="text-sm text-red-400">{searchError()}</p>
            </Show>
            <Button
              variant="primary"
              onClick={() => void handleSearch()}
              loading={searching()}
            >
              <Search class="h-4 w-4" />
              Search
            </Button>
          </div>

          {/* Search results */}
          <Show when={hasResults()}>
            <div class="flex flex-col gap-3">
              <p class="text-sm font-medium text-slate-400">Search Results</p>
              <For each={Object.entries(searchResults())}>
                {([provider, results]) => (
                  <Show when={results.length > 0}>
                    <div class="flex flex-col gap-2">
                      <p class="text-xs font-medium uppercase tracking-wide text-slate-500">
                        {provider.replace(/_/g, " ")}
                      </p>
                      <For each={results}>
                        {(result) => (
                          <ResultCard
                            result={result}
                            bookId={props.bookId}
                            onProposalCreated={handleProposalCreated}
                          />
                        )}
                      </For>
                    </div>
                  </Show>
                )}
              </For>
            </div>
          </Show>

          {/* Proposals */}
          <Suspense>
            <Show when={(proposals() ?? []).length > 0}>
              <div class="flex flex-col gap-3">
                <p class="text-sm font-medium text-slate-400">Proposals</p>
                <For each={proposals() ?? []}>
                  {(proposal) => (
                    <ProposalCard
                      proposal={proposal}
                      canEdit={canEdit()}
                      onAction={handleProposalCreated}
                    />
                  )}
                </For>
              </div>
            </Show>
          </Suspense>

          {/* Field locks (admin/canEditMetadata only) */}
          <Show when={canEdit()}>
            <FieldLocks bookId={props.bookId} />
          </Show>
        </div>
      </div>
    </div>
  );
};

export default MetadataSearch;
