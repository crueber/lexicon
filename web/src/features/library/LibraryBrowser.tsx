import {
  type Component,
  createResource,
  createSignal,
  createMemo,
  For,
  Show,
  Suspense,
  ErrorBoundary,
} from "solid-js";
import { useParams, useNavigate } from "@solidjs/router";
import { ChevronLeft, ChevronRight, ScanLine } from "lucide-solid";
import { api } from "../../shared/api/client";
import { useAuth } from "../auth/AuthProvider";
import Button from "../../shared/ui/Button";
import Skeleton from "../../shared/ui/Skeleton";
import BookCard from "../book/BookCard";
import type { Library, BooksResponse } from "./types";

const PAGE_SIZE = 24;

type BookTypeFilter = "ALL" | "EBOOK" | "AUDIOBOOK" | "COMIC";
type SortOption = "addedDate_DESC" | "addedDate_ASC" | "title_ASC" | "title_DESC";

interface BooksQueryParams {
  libraryId: number;
  page: number;
  bookType: BookTypeFilter;
  sort: SortOption;
}

async function fetchLibrary(id: number): Promise<Library> {
  return api<Library>(`/libraries/${id}`);
}

async function fetchBooks(params: BooksQueryParams): Promise<BooksResponse> {
  const qs = new URLSearchParams();
  qs.set("libraryId", String(params.libraryId));
  qs.set("page", String(params.page));
  qs.set("size", String(PAGE_SIZE));

  if (params.bookType !== "ALL") {
    qs.set("bookType", params.bookType);
  }

  const [sortBy, sortDir] = params.sort.split("_") as [string, string];
  qs.set("sortBy", sortBy);
  qs.set("sortDir", sortDir);

  return api<BooksResponse>(`/books?${qs.toString()}`);
}

// Skeleton grid shown while books are loading.
const BookGridSkeleton: Component = () => (
  <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
    <For each={Array.from({ length: PAGE_SIZE })}>
      {() => (
        <div class="flex flex-col gap-2 rounded-lg bg-slate-800 p-2">
          <Skeleton class="aspect-[2/3] w-full rounded-md" />
          <Skeleton class="h-3 w-full rounded" />
          <Skeleton class="h-3 w-2/3 rounded" />
        </div>
      )}
    </For>
  </div>
);

// Filter button group for book type.
const BookTypeFilterBar: Component<{
  value: BookTypeFilter;
  onChange: (v: BookTypeFilter) => void;
}> = (props) => {
  const options: { label: string; value: BookTypeFilter }[] = [
    { label: "All", value: "ALL" },
    { label: "Ebooks", value: "EBOOK" },
    { label: "Audiobooks", value: "AUDIOBOOK" },
    { label: "Comics", value: "COMIC" },
  ];

  return (
    <div class="flex rounded-lg bg-slate-800 p-1 gap-1">
      <For each={options}>
        {(opt) => (
          <button
            onClick={() => props.onChange(opt.value)}
            class={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
              props.value === opt.value
                ? "bg-indigo-600 text-white"
                : "text-slate-400 hover:text-slate-200"
            }`}
          >
            {opt.label}
          </button>
        )}
      </For>
    </div>
  );
};

// Sort dropdown.
const SortSelect: Component<{
  value: SortOption;
  onChange: (v: SortOption) => void;
}> = (props) => {
  const options: { label: string; value: SortOption }[] = [
    { label: "Recently Added", value: "addedDate_DESC" },
    { label: "Oldest First", value: "addedDate_ASC" },
    { label: "Title A–Z", value: "title_ASC" },
    { label: "Title Z–A", value: "title_DESC" },
  ];

  return (
    <select
      value={props.value}
      onChange={(e) => props.onChange(e.currentTarget.value as SortOption)}
      class="rounded-lg border border-slate-700 bg-slate-800 px-3 py-2 text-sm text-slate-100 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 focus:ring-offset-slate-900"
    >
      <For each={options}>
        {(opt) => <option value={opt.value}>{opt.label}</option>}
      </For>
    </select>
  );
};

// Pagination controls.
const Pagination: Component<{
  page: number;
  totalPages: number;
  onPrev: () => void;
  onNext: () => void;
}> = (props) => (
  <Show when={props.totalPages > 1}>
    <div class="flex items-center justify-center gap-3 py-6">
      <Button
        variant="secondary"
        size="sm"
        onClick={props.onPrev}
        disabled={props.page <= 1}
      >
        <ChevronLeft class="h-4 w-4" />
        Prev
      </Button>
      <span class="text-sm text-slate-400">
        Page {props.page} of {props.totalPages}
      </span>
      <Button
        variant="secondary"
        size="sm"
        onClick={props.onNext}
        disabled={props.page >= props.totalPages}
      >
        Next
        <ChevronRight class="h-4 w-4" />
      </Button>
    </div>
  </Show>
);

// Inner component that uses createResource — must be inside Suspense.
const LibraryBrowserInner: Component<{ libraryId: number }> = (props) => {
  const navigate = useNavigate();
  const auth = useAuth();

  const [page, setPage] = createSignal(1);
  const [bookTypeFilter, setBookTypeFilter] = createSignal<BookTypeFilter>("ALL");
  const [sort, setSort] = createSignal<SortOption>("addedDate_DESC");
  const [scanning, setScanning] = createSignal(false);

  // Fetch library metadata.
  const [library] = createResource(() => props.libraryId, fetchLibrary);

  // Reactive query params — when filters change, reset to page 1.
  const booksParams = createMemo<BooksQueryParams>(() => ({
    libraryId: props.libraryId,
    page: page(),
    bookType: bookTypeFilter(),
    sort: sort(),
  }));

  // Fetch books — refetches whenever booksParams changes.
  const [booksData] = createResource(booksParams, fetchBooks);

  const totalPages = createMemo(() => {
    const data = booksData();
    if (!data) return 1;
    return Math.max(1, Math.ceil(data.total / PAGE_SIZE));
  });

  function handleFilterChange(value: BookTypeFilter) {
    setBookTypeFilter(value);
    setPage(1);
  }

  function handleSortChange(value: SortOption) {
    setSort(value);
    setPage(1);
  }

  async function handleScan() {
    setScanning(true);
    try {
      await api(`/libraries/${props.libraryId}/scan`, { method: "POST" });
    } catch {
      // Scan errors are non-fatal from the UI perspective.
    } finally {
      setScanning(false);
    }
  }

  return (
    <div class="flex flex-1 flex-col">
      {/* Header */}
      <div class="border-b border-slate-800 px-6 py-5">
        <div class="flex items-start justify-between gap-4">
          <div class="min-w-0">
            <Show when={library()} fallback={<Skeleton class="h-7 w-48 rounded" />}>
              {(lib) => (
                <>
                  <h1 class="truncate text-xl font-bold text-slate-100">
                    {lib().name}
                  </h1>
                  <Show when={booksData()}>
                    {(data) => (
                      <p class="mt-1 text-sm text-slate-400">
                        {data().total.toLocaleString()}{" "}
                        {data().total === 1 ? "book" : "books"}
                      </p>
                    )}
                  </Show>
                </>
              )}
            </Show>
          </div>

          {/* Scan button — admin only */}
          <Show when={auth.isAdmin()}>
            <Button
              variant="secondary"
              size="sm"
              onClick={handleScan}
              loading={scanning()}
            >
              <ScanLine class="h-4 w-4" />
              Scan
            </Button>
          </Show>
        </div>

        {/* Filter bar */}
        <div class="mt-4 flex flex-wrap items-center gap-3">
          <BookTypeFilterBar
            value={bookTypeFilter()}
            onChange={handleFilterChange}
          />
          <SortSelect value={sort()} onChange={handleSortChange} />
        </div>
      </div>

      {/* Book grid */}
      <div class="flex-1 p-6">
        <Show
          when={!booksData.loading}
          fallback={<BookGridSkeleton />}
        >
          <Show
            when={(booksData()?.books ?? []).length > 0}
            fallback={
              <div class="flex flex-col items-center justify-center gap-4 py-20 text-center">
                <p class="text-lg font-medium text-slate-300">No books found</p>
                <p class="text-sm text-slate-500">
                  Try changing the filters or scan the library.
                </p>
              </div>
            }
          >
            <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
              <For each={booksData()?.books ?? []}>
                {(book) => (
                  <BookCard
                    book={book}
                    onClick={() => navigate(`/books/${book.id}`)}
                  />
                )}
              </For>
            </div>
          </Show>
        </Show>

        {/* Pagination */}
        <Pagination
          page={page()}
          totalPages={totalPages()}
          onPrev={() => setPage((p) => Math.max(1, p - 1))}
          onNext={() => setPage((p) => Math.min(totalPages(), p + 1))}
        />
      </div>
    </div>
  );
};

const LibraryBrowser: Component = () => {
  const params = useParams<{ id: string }>();
  const libraryId = createMemo(() => parseInt(params.id, 10));

  return (
    <ErrorBoundary
      fallback={(err) => (
        <div class="flex flex-1 flex-col items-center justify-center gap-3 p-8 text-center">
          <p class="text-lg font-medium text-red-400">
            Failed to load library
          </p>
          <p class="text-sm text-slate-500">{err.message}</p>
        </div>
      )}
    >
      <Suspense
        fallback={
          <div class="flex flex-1 flex-col">
            <div class="border-b border-slate-800 px-6 py-5">
              <Skeleton class="h-7 w-48 rounded" />
              <Skeleton class="mt-2 h-4 w-24 rounded" />
            </div>
            <div class="p-6">
              <BookGridSkeleton />
            </div>
          </div>
        }
      >
        <LibraryBrowserInner libraryId={libraryId()} />
      </Suspense>
    </ErrorBoundary>
  );
};

export default LibraryBrowser;
