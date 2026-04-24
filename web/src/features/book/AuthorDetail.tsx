import {
  type Component,
  createResource,
  createMemo,
  Show,
  For,
  Suspense,
  ErrorBoundary,
} from "solid-js";
import { useParams, useNavigate } from "@solidjs/router";
import { User, BookOpen } from "lucide-solid";
import { api } from "../../shared/api/client";
import { t } from "../../shared/i18n/i18n";
import Skeleton from "../../shared/ui/Skeleton";
import BookCard from "./BookCard";
import type { Book } from "../library/types";

// --- Types ---

interface AuthorDetail {
  id: number;
  name: string;
}

// --- API ---

async function fetchAuthor(id: number): Promise<AuthorDetail> {
  return api<AuthorDetail>(`/authors/${id}`);
}

async function fetchAuthorBooks(id: number): Promise<Book[]> {
  return api<Book[]>(`/authors/${id}/books`);
}

// --- Skeleton ---

const AuthorDetailSkeleton: Component = () => (
  <div class="flex flex-1 flex-col">
    <div class="border-b border-slate-800 px-6 py-5">
      <Skeleton class="h-7 w-48 rounded" />
      <Skeleton class="mt-2 h-4 w-32 rounded" />
    </div>
    <div class="p-6">
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
        <For each={Array.from({ length: 12 })}>
          {() => (
            <div class="flex flex-col gap-2 rounded-lg bg-slate-800 p-2">
              <Skeleton class="aspect-[2/3] w-full rounded-md" />
              <Skeleton class="h-3 w-full rounded" />
            </div>
          )}
        </For>
      </div>
    </div>
  </div>
);

// --- Inner component ---

const AuthorDetailInner: Component<{ authorId: number }> = (props) => {
  const navigate = useNavigate();
  const [author] = createResource(() => props.authorId, fetchAuthor);
  const [books] = createResource(() => props.authorId, fetchAuthorBooks);

  const bookCount = createMemo(() => (books() ?? []).length);

  return (
    <Show when={author()} fallback={<AuthorDetailSkeleton />}>
      {(a) => (
        <div class="flex flex-1 flex-col">
          {/* Header */}
          <div class="flex items-start justify-between border-b border-slate-800 px-6 py-5">
            <div class="flex items-center gap-4">
              <button
                onClick={() => navigate("/authors")}
                class="text-sm text-slate-400 hover:text-slate-200 transition-colors"
              >
                {t("common.back")}
              </button>
              <div class="flex items-center gap-3">
                <div class="flex h-10 w-10 items-center justify-center rounded-full bg-indigo-600/20 text-indigo-300">
                  <User class="h-5 w-5" />
                </div>
                <div>
                  <h1 class="text-xl font-bold text-slate-100">{a().name}</h1>
                  <p class="text-sm text-slate-400">
                    {bookCount()} {bookCount() === 1 ? t("common.book") : t("common.books")}
                  </p>
                </div>
              </div>
            </div>
          </div>

          {/* Book grid */}
          <div class="flex-1 p-6">
            <Show
              when={!books.loading}
              fallback={
                <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
                  <For each={Array.from({ length: 12 })}>
                    {() => (
                      <div class="flex flex-col gap-2 rounded-lg bg-slate-800 p-2">
                        <Skeleton class="aspect-[2/3] w-full rounded-md" />
                        <Skeleton class="h-3 w-full rounded" />
                      </div>
                    )}
                  </For>
                </div>
              }
            >
              <Show
                when={(books() ?? []).length > 0}
                fallback={
                  <div class="flex flex-col items-center justify-center gap-4 py-20 text-center">
                    <BookOpen class="h-12 w-12 text-slate-600" />
                    <p class="text-lg font-medium text-slate-300">{t("common.noBooksYet")}</p>
                  </div>
                }
              >
                <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
                  <For each={books() ?? []}>
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
          </div>
        </div>
      )}
    </Show>
  );
};

// --- Page wrapper ---

const AuthorDetail: Component = () => {
  const params = useParams<{ id: string }>();
  const authorId = createMemo(() => parseInt(params.id, 10));

  return (
    <ErrorBoundary
      fallback={(err) => (
        <div class="flex flex-1 flex-col items-center justify-center gap-3 p-8 text-center">
          <p class="text-lg font-medium text-red-400">
            {err.message.includes("404") ? t("common.notFound") : t("common.failedToLoad")}
          </p>
          <p class="text-sm text-slate-500">{err.message}</p>
        </div>
      )}
    >
      <Suspense fallback={<AuthorDetailSkeleton />}>
        <AuthorDetailInner authorId={authorId()} />
      </Suspense>
    </ErrorBoundary>
  );
};

export default AuthorDetail;
