import {
  type Component,
  createResource,
  For,
  Show,
  Suspense,
  ErrorBoundary,
} from "solid-js";
import { useNavigate } from "@solidjs/router";
import { Library as LibraryIcon } from "lucide-solid";
import { api } from "../../shared/api/client";
import Skeleton from "../../shared/ui/Skeleton";
import { t } from "../../shared/i18n/i18n";
import type { Library } from "./types";

async function fetchLibraries(): Promise<Library[]> {
  return api<Library[]>("/libraries");
}

// Skeleton card shown while libraries are loading.
const LibraryCardSkeleton: Component = () => (
  <div class="flex flex-col gap-3 rounded-xl bg-slate-800 p-5">
    <Skeleton class="h-8 w-8 rounded-lg" />
    <Skeleton class="h-5 w-3/4 rounded" />
    <Skeleton class="h-4 w-1/2 rounded" />
  </div>
);

// A single library card.
const LibraryCard: Component<{ library: Library; onClick: () => void }> = (
  props,
) => {
  return (
    <button
      onClick={props.onClick}
      class="flex flex-col gap-3 rounded-xl bg-slate-800 p-5 text-left transition-colors hover:bg-slate-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500 focus-visible:ring-offset-2 focus-visible:ring-offset-slate-900"
    >
      {/* Icon */}
      <div
        class="flex h-10 w-10 items-center justify-center rounded-lg"
        style={
          props.library.iconColor
            ? { "background-color": props.library.iconColor + "33" }
            : {}
        }
      >
        <LibraryIcon
          class="h-5 w-5"
          style={
            props.library.iconColor ? { color: props.library.iconColor } : {}
          }
        />
      </div>

      {/* Name */}
      <div>
        <h3 class="font-semibold text-slate-100">{props.library.name}</h3>
        <p class="mt-0.5 text-sm text-slate-400">
          {props.library.paths.length === 1
            ? `1 ${t("common.path")}`
            : `${props.library.paths.length} ${t("common.paths")}`}
        </p>
      </div>
    </button>
  );
};

// Inner component that uses createResource — must be inside Suspense.
const LibraryListInner: Component = () => {
  const navigate = useNavigate();
  const [libraries] = createResource(fetchLibraries);

  return (
    <Show
      when={!libraries.loading}
      fallback={
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <LibraryCardSkeleton />
          <LibraryCardSkeleton />
          <LibraryCardSkeleton />
        </div>
      }
    >
      <Show
        when={(libraries() ?? []).length > 0}
        fallback={
          <div class="flex flex-col items-center justify-center gap-4 py-20 text-center">
            <LibraryIcon class="h-16 w-16 text-slate-600" />
            <div>
              <p class="text-lg font-medium text-slate-300">{t("library.noLibrariesYet")}</p>
              <p class="mt-1 text-sm text-slate-500">
                {t("library.askAdminToCreate")}
              </p>
            </div>
          </div>
        }
      >
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <For each={libraries()}>
            {(library) => (
              <LibraryCard
                library={library}
                onClick={() => navigate(`/libraries/${library.id}/books`)}
              />
            )}
          </For>
        </div>
      </Show>
    </Show>
  );
};

const LibraryList: Component = () => {
  return (
    <div class="flex flex-1 flex-col">
      {/* Page header */}
      <div class="border-b border-slate-800 px-6 py-5">
        <h1 class="text-xl font-bold text-slate-100">{t("library.librariesTitle")}</h1>
        <p class="mt-1 text-sm text-slate-400">
          {t("library.librariesSubtitle")}
        </p>
      </div>

      {/* Content */}
      <div class="flex-1 p-6">
        <ErrorBoundary
          fallback={(err) => (
            <div class="flex flex-col items-center justify-center gap-3 py-20 text-center">
              <p class="text-lg font-medium text-red-400">
                {t("library.failedToLoadLibraries")}
              </p>
              <p class="text-sm text-slate-500">{err.message}</p>
            </div>
          )}
        >
          <Suspense
            fallback={
              <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
                <LibraryCardSkeleton />
                <LibraryCardSkeleton />
                <LibraryCardSkeleton />
              </div>
            }
          >
            <LibraryListInner />
          </Suspense>
        </ErrorBoundary>
      </div>
    </div>
  );
};

export default LibraryList;
