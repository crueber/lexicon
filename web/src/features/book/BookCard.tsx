import { type Component, createSignal, Show } from "solid-js";
import { BookOpen, Headphones, BookImage } from "lucide-solid";
import { t } from "../../shared/i18n/i18n";
import type { Book } from "../library/types";

interface BookCardProps {
  book: Book;
  onClick?: () => void;
}

// Badge shown for non-EBOOK types.
const BookTypeBadge: Component<{ bookType: Book["bookType"] }> = (props) => {
  return (
    <Show when={props.bookType !== "EBOOK"}>
      <span
        class={`absolute top-1.5 right-1.5 rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide ${
          props.bookType === "AUDIOBOOK"
            ? "bg-purple-600/90 text-purple-100"
            : "bg-amber-600/90 text-amber-100"
        }`}
      >
        {props.bookType === "AUDIOBOOK" ? t("common.audio") : t("common.comic")}
      </span>
    </Show>
  );
};

// Placeholder shown when no cover is available or cover fails to load.
const CoverPlaceholder: Component<{ bookType: Book["bookType"] }> = (props) => (
  <div class="flex h-full w-full items-center justify-center bg-slate-700">
    <Show
      when={props.bookType === "AUDIOBOOK"}
      fallback={
        <Show
          when={props.bookType === "COMIC"}
          fallback={<BookOpen class="h-10 w-10 text-slate-500" />}
        >
          <BookImage class="h-10 w-10 text-slate-500" />
        </Show>
      }
    >
      <Headphones class="h-10 w-10 text-slate-500" />
    </Show>
  </div>
);

const BookCard: Component<BookCardProps> = (props) => {
  const [imgError, setImgError] = createSignal(false);

  // Audiobooks use a square aspect ratio; books/comics use 2:3.
  const isAudiobook = () => props.book.bookType === "AUDIOBOOK";

  return (
    <button
      onClick={props.onClick}
      class="group flex flex-col gap-2 rounded-lg bg-slate-800 p-2 text-left transition-colors hover:bg-slate-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500 focus-visible:ring-offset-2 focus-visible:ring-offset-slate-900"
    >
      {/* Cover image */}
      <div
        class={`relative w-full overflow-hidden rounded-md bg-slate-700 ${
          isAudiobook() ? "aspect-square" : "aspect-[2/3]"
        }`}
      >
        <Show
          when={props.book.id && !imgError()}
          fallback={<CoverPlaceholder bookType={props.book.bookType} />}
        >
          <img
            src={`/api/books/${props.book.id}/cover/thumbnail`}
            alt={props.book.title ?? t("common.bookCover")}
            loading="lazy"
            class="h-full w-full object-cover transition-transform duration-200 group-hover:scale-105"
            onError={() => setImgError(true)}
          />
        </Show>
        <BookTypeBadge bookType={props.book.bookType} />
      </div>

      {/* Title */}
      <div class="min-w-0 flex-1">
        <p class="line-clamp-2 text-xs font-medium leading-tight text-slate-100">
          {props.book.title ?? t("common.untitled")}
        </p>
        <Show when={props.book.authors.length > 0}>
          <p class="mt-0.5 truncate text-[11px] text-slate-400">
            {props.book.authors.join(", ")}
          </p>
        </Show>
      </div>
    </button>
  );
};

export default BookCard;
