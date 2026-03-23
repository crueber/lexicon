import {
  type Component,
  createSignal,
  createResource,
  createEffect,
  Show,
  For,
  onCleanup,
} from "solid-js";
import { createStore, produce } from "solid-js/store";
import { useParams, useNavigate } from "@solidjs/router";
import { Plus, Trash2, X, Wand2, ChevronDown } from "lucide-solid";
import { api } from "../../shared/api/client";
import Button from "../../shared/ui/Button";
import Input from "../../shared/ui/Input";
import type { MagicShelf, RuleGroup, RuleItem } from "../library/types";

// ---- Field and operator definitions ----

interface FieldDef {
  value: string;
  label: string;
  type: "text" | "number" | "date";
}

const FIELDS: FieldDef[] = [
  { value: "title", label: "Title", type: "text" },
  { value: "author", label: "Author", type: "text" },
  { value: "category", label: "Category", type: "text" },
  { value: "tag", label: "Tag", type: "text" },
  { value: "series", label: "Series", type: "text" },
  { value: "language", label: "Language", type: "text" },
  { value: "book_type", label: "Book Type", type: "text" },
  { value: "format", label: "Format", type: "text" },
  { value: "publisher", label: "Publisher", type: "text" },
  { value: "added_date", label: "Added Date", type: "date" },
  { value: "page_count", label: "Page Count", type: "number" },
];

interface OperatorDef {
  value: string;
  label: string;
  types: ("text" | "number" | "date")[];
  noValue?: boolean;
}

const OPERATORS: OperatorDef[] = [
  { value: "contains", label: "Contains", types: ["text"] },
  { value: "equals", label: "Equals", types: ["text", "number", "date"] },
  { value: "starts_with", label: "Starts with", types: ["text"] },
  { value: "ends_with", label: "Ends with", types: ["text"] },
  { value: "greater_than", label: "After / Greater than", types: ["number", "date"] },
  { value: "less_than", label: "Before / Less than", types: ["number", "date"] },
  { value: "is_empty", label: "Is empty", types: ["text", "number", "date"], noValue: true },
  { value: "is_not_empty", label: "Is not empty", types: ["text", "number", "date"], noValue: true },
];

const SORT_FIELDS = [
  { value: "added_date", label: "Added Date" },
  { value: "title", label: "Title" },
  { value: "author", label: "Author" },
  { value: "page_count", label: "Page Count" },
];

// ---- API ----

async function fetchMagicShelf(id: number): Promise<MagicShelf> {
  return api<MagicShelf>(`/magic-shelves/${id}`);
}

async function createMagicShelf(params: {
  name: string;
  description: string;
  icon: string;
  iconColor: string;
  rules: string;
  sortField: string;
  sortDir: string;
  limitCount?: number;
}): Promise<MagicShelf> {
  return api<MagicShelf>("/magic-shelves", {
    method: "POST",
    body: JSON.stringify(params),
  });
}

async function updateMagicShelf(
  id: number,
  params: {
    name: string;
    description: string;
    icon: string;
    iconColor: string;
    rules: string;
    sortField: string;
    sortDir: string;
    limitCount?: number;
  }
): Promise<void> {
  await api(`/magic-shelves/${id}`, {
    method: "PUT",
    body: JSON.stringify(params),
  });
}

async function fetchPreviewCount(id: number): Promise<{ count: number }> {
  return api<{ count: number }>(`/magic-shelves/${id}/count`);
}

// ---- Rule Item Editor ----

const RuleItemEditor: Component<{
  item: RuleItem;
  depth: number;
  onUpdate: (item: RuleItem) => void;
  onRemove: () => void;
}> = (props) => {
  const fieldDef = () => FIELDS.find((f) => f.value === props.item.field) ?? FIELDS[0];
  const availableOperators = () =>
    OPERATORS.filter((op) => op.types.includes(fieldDef().type));
  const currentOperator = () =>
    OPERATORS.find((op) => op.value === props.item.operator);

  function handleFieldChange(field: string) {
    const newFieldDef = FIELDS.find((f) => f.value === field) ?? FIELDS[0];
    const validOps = OPERATORS.filter((op) => op.types.includes(newFieldDef.type));
    const currentOp = props.item.operator ?? "";
    const opStillValid = validOps.some((op) => op.value === currentOp);
    props.onUpdate({
      ...props.item,
      field,
      operator: opStillValid ? currentOp : (validOps[0]?.value ?? "equals"),
    });
  }

  if (props.item.type === "group" && props.item.group) {
    return (
      <div class="rounded-lg border border-slate-600 bg-slate-800/50 p-3">
        <div class="mb-2 flex items-center justify-between">
          <RuleGroupEditor
            group={props.item.group}
            depth={props.depth + 1}
            onUpdate={(g) => props.onUpdate({ ...props.item, group: g })}
          />
          <button
            onClick={props.onRemove}
            class="ml-2 shrink-0 rounded p-1 text-slate-400 hover:bg-slate-700 hover:text-red-400 transition-colors"
            title="Remove group"
          >
            <Trash2 class="h-4 w-4" />
          </button>
        </div>
      </div>
    );
  }

  return (
    <div class="flex items-center gap-2 rounded-lg bg-slate-800/50 p-2">
      {/* Field selector */}
      <div class="relative min-w-0 flex-1">
        <select
          value={props.item.field ?? "title"}
          onChange={(e) => handleFieldChange(e.currentTarget.value)}
          class="w-full appearance-none rounded-md bg-slate-700 px-3 py-1.5 pr-8 text-sm text-slate-100 focus:outline-none focus:ring-2 focus:ring-indigo-500"
        >
          <For each={FIELDS}>
            {(f) => <option value={f.value}>{f.label}</option>}
          </For>
        </select>
        <ChevronDown class="pointer-events-none absolute right-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
      </div>

      {/* Operator selector */}
      <div class="relative min-w-0 flex-1">
        <select
          value={props.item.operator ?? "contains"}
          onChange={(e) =>
            props.onUpdate({ ...props.item, operator: e.currentTarget.value })
          }
          class="w-full appearance-none rounded-md bg-slate-700 px-3 py-1.5 pr-8 text-sm text-slate-100 focus:outline-none focus:ring-2 focus:ring-indigo-500"
        >
          <For each={availableOperators()}>
            {(op) => <option value={op.value}>{op.label}</option>}
          </For>
        </select>
        <ChevronDown class="pointer-events-none absolute right-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
      </div>

      {/* Value input */}
      <Show when={!currentOperator()?.noValue}>
        <input
          type={fieldDef().type === "number" ? "number" : fieldDef().type === "date" ? "date" : "text"}
          value={props.item.value ?? ""}
          onInput={(e) =>
            props.onUpdate({ ...props.item, value: e.currentTarget.value })
          }
          placeholder="Value"
          class="min-w-0 flex-1 rounded-md bg-slate-700 px-3 py-1.5 text-sm text-slate-100 placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-indigo-500"
        />
      </Show>

      {/* Remove button */}
      <button
        onClick={props.onRemove}
        class="shrink-0 rounded p-1 text-slate-400 hover:bg-slate-700 hover:text-red-400 transition-colors"
        title="Remove condition"
      >
        <X class="h-4 w-4" />
      </button>
    </div>
  );
};

// ---- Rule Group Editor ----

const RuleGroupEditor: Component<{
  group: RuleGroup;
  depth: number;
  onUpdate: (group: RuleGroup) => void;
}> = (props) => {
  function addCondition() {
    props.onUpdate({
      ...props.group,
      rules: [
        ...props.group.rules,
        { type: "condition", field: "title", operator: "contains", value: "" },
      ],
    });
  }

  function addGroup() {
    props.onUpdate({
      ...props.group,
      rules: [
        ...props.group.rules,
        {
          type: "group",
          group: { operator: "AND", rules: [] },
        },
      ],
    });
  }

  function updateItem(index: number, item: RuleItem) {
    const rules = [...props.group.rules];
    rules[index] = item;
    props.onUpdate({ ...props.group, rules });
  }

  function removeItem(index: number) {
    const rules = props.group.rules.filter((_, i) => i !== index);
    props.onUpdate({ ...props.group, rules });
  }

  return (
    <div class="flex flex-1 flex-col gap-2">
      {/* AND/OR toggle */}
      <div class="flex items-center gap-2">
        <span class="text-xs font-medium text-slate-400">Match</span>
        <div class="flex rounded-md overflow-hidden border border-slate-600">
          <button
            type="button"
            onClick={() => props.onUpdate({ ...props.group, operator: "AND" })}
            class={`px-3 py-1 text-xs font-medium transition-colors ${
              props.group.operator === "AND"
                ? "bg-indigo-600 text-white"
                : "bg-slate-700 text-slate-400 hover:text-slate-200"
            }`}
          >
            ALL (AND)
          </button>
          <button
            type="button"
            onClick={() => props.onUpdate({ ...props.group, operator: "OR" })}
            class={`px-3 py-1 text-xs font-medium transition-colors ${
              props.group.operator === "OR"
                ? "bg-indigo-600 text-white"
                : "bg-slate-700 text-slate-400 hover:text-slate-200"
            }`}
          >
            ANY (OR)
          </button>
        </div>
        <span class="text-xs text-slate-400">of the following rules</span>
      </div>

      {/* Rule items */}
      <div class="flex flex-col gap-2">
        <For each={props.group.rules}>
          {(item, index) => (
            <RuleItemEditor
              item={item}
              depth={props.depth}
              onUpdate={(updated) => updateItem(index(), updated)}
              onRemove={() => removeItem(index())}
            />
          )}
        </For>
      </div>

      {/* Add buttons */}
      <div class="flex items-center gap-2">
        <button
          type="button"
          onClick={addCondition}
          class="flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium text-indigo-400 hover:bg-slate-700 hover:text-indigo-300 transition-colors"
        >
          <Plus class="h-3.5 w-3.5" />
          Add condition
        </button>
        <Show when={props.depth < 2}>
          <button
            type="button"
            onClick={addGroup}
            class="flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium text-slate-400 hover:bg-slate-700 hover:text-slate-200 transition-colors"
          >
            <Plus class="h-3.5 w-3.5" />
            Add group
          </button>
        </Show>
      </div>
    </div>
  );
};

// ---- Main Builder Component ----

const MagicShelfBuilderInner: Component<{ editId?: number }> = (props) => {
  const navigate = useNavigate();

  const [existing] = createResource(
    () => props.editId,
    (id) => (id ? fetchMagicShelf(id) : undefined)
  );

  const [name, setName] = createSignal("");
  const [description, setDescription] = createSignal("");
  const [icon, setIcon] = createSignal("");
  const [iconColor, setIconColor] = createSignal("");
  const [sortField, setSortField] = createSignal("added_date");
  const [sortDir, setSortDir] = createSignal<"ASC" | "DESC">("DESC");
  const [saving, setSaving] = createSignal(false);
  const [error, setError] = createSignal("");
  const [savedId, setSavedId] = createSignal<number | null>(null);
  const [previewCount, setPreviewCount] = createSignal<number | null>(null);
  const [previewLoading, setPreviewLoading] = createSignal(false);

  const [rules, setRules] = createStore<RuleGroup>({
    operator: "AND",
    rules: [],
  });

  // Populate form when editing an existing shelf.
  createEffect(() => {
    const ms = existing();
    if (!ms) return;
    setName(ms.name);
    setDescription(ms.description ?? "");
    setIcon(ms.icon ?? "");
    setIconColor(ms.iconColor ?? "");
    setSortField(ms.sortField);
    setSortDir(ms.sortDir);
    try {
      const parsed = JSON.parse(ms.rules) as RuleGroup;
      setRules(produce(() => parsed));
    } catch {
      // Keep default empty rules.
    }
  });

  // Debounced preview count — only works after save (needs a shelf ID).
  let previewTimer: ReturnType<typeof setTimeout> | undefined;
  createEffect(() => {
    // Track rules changes by serializing them.
    const rulesJson = JSON.stringify(rules);
    const id = savedId() ?? props.editId;
    if (!id) return;

    clearTimeout(previewTimer);
    setPreviewLoading(true);
    previewTimer = setTimeout(async () => {
      try {
        // First save the current rules, then fetch count.
        const result = await fetchPreviewCount(id);
        setPreviewCount(result.count);
      } catch {
        setPreviewCount(null);
      } finally {
        setPreviewLoading(false);
      }
    }, 500);

    // Suppress unused variable warning — rulesJson is used for tracking.
    void rulesJson;
  });

  onCleanup(() => clearTimeout(previewTimer));

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!name().trim()) {
      setError("Name is required");
      return;
    }

    setSaving(true);
    setError("");

    const params = {
      name: name().trim(),
      description: description().trim(),
      icon: icon().trim(),
      iconColor: iconColor().trim(),
      rules: JSON.stringify(rules),
      sortField: sortField(),
      sortDir: sortDir(),
    };

    try {
      const editId = props.editId;
      if (editId) {
        await updateMagicShelf(editId, params);
        navigate(`/magic-shelves/${editId}`);
      } else {
        const created = await createMagicShelf(params);
        setSavedId(created.id);
        navigate(`/magic-shelves/${created.id}`);
      }
    } catch {
      setError("Failed to save magic shelf. Please try again.");
    } finally {
      setSaving(false);
    }
  }

  const isEditing = () => !!props.editId;

  return (
    <div class="flex flex-1 flex-col">
      {/* Header */}
      <div class="flex items-center justify-between border-b border-slate-800 px-6 py-5">
        <div class="flex items-center gap-3">
          <button
            onClick={() => navigate("/shelves")}
            class="text-sm text-slate-400 hover:text-slate-200 transition-colors"
          >
            ← Shelves
          </button>
          <div class="flex items-center gap-2">
            <Wand2 class="h-5 w-5 text-indigo-400" />
            <h1 class="text-xl font-bold text-slate-100">
              {isEditing() ? "Edit Magic Shelf" : "New Magic Shelf"}
            </h1>
          </div>
        </div>
      </div>

      {/* Form */}
      <form onSubmit={handleSubmit} class="flex flex-1 flex-col gap-6 overflow-y-auto p-6">
        {/* Basic info */}
        <div class="rounded-xl bg-slate-800 p-5">
          <h2 class="mb-4 text-sm font-semibold uppercase tracking-wider text-slate-400">
            Basic Info
          </h2>
          <div class="flex flex-col gap-4">
            <Input
              label="Name"
              placeholder="My Magic Shelf"
              value={name()}
              onInput={(e) => setName(e.currentTarget.value)}
              required
            />
            <Input
              label="Description (optional)"
              placeholder="Books matching my criteria"
              value={description()}
              onInput={(e) => setDescription(e.currentTarget.value)}
            />
            <div class="grid grid-cols-2 gap-4">
              <Input
                label="Icon (emoji, optional)"
                placeholder="✨"
                value={icon()}
                onInput={(e) => setIcon(e.currentTarget.value)}
              />
              <Input
                label="Color (hex, optional)"
                placeholder="#6366f1"
                value={iconColor()}
                onInput={(e) => setIconColor(e.currentTarget.value)}
              />
            </div>
          </div>
        </div>

        {/* Rules */}
        <div class="rounded-xl bg-slate-800 p-5">
          <h2 class="mb-4 text-sm font-semibold uppercase tracking-wider text-slate-400">
            Rules
          </h2>
          <RuleGroupEditor
            group={rules}
            depth={0}
            onUpdate={(g) => setRules(produce(() => g))}
          />
        </div>

        {/* Sort & Limit */}
        <div class="rounded-xl bg-slate-800 p-5">
          <h2 class="mb-4 text-sm font-semibold uppercase tracking-wider text-slate-400">
            Sort & Limit
          </h2>
          <div class="grid grid-cols-2 gap-4">
            <div class="flex flex-col gap-1.5">
              <label class="text-sm font-medium text-slate-300">Sort by</label>
              <div class="relative">
                <select
                  value={sortField()}
                  onChange={(e) => setSortField(e.currentTarget.value)}
                  class="w-full appearance-none rounded-md bg-slate-700 px-3 py-2 pr-8 text-sm text-slate-100 focus:outline-none focus:ring-2 focus:ring-indigo-500"
                >
                  <For each={SORT_FIELDS}>
                    {(f) => <option value={f.value}>{f.label}</option>}
                  </For>
                </select>
                <ChevronDown class="pointer-events-none absolute right-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
              </div>
            </div>
            <div class="flex flex-col gap-1.5">
              <label class="text-sm font-medium text-slate-300">Direction</label>
              <div class="flex rounded-md overflow-hidden border border-slate-600">
                <button
                  type="button"
                  onClick={() => setSortDir("DESC")}
                  class={`flex-1 py-2 text-sm font-medium transition-colors ${
                    sortDir() === "DESC"
                      ? "bg-indigo-600 text-white"
                      : "bg-slate-700 text-slate-400 hover:text-slate-200"
                  }`}
                >
                  Descending
                </button>
                <button
                  type="button"
                  onClick={() => setSortDir("ASC")}
                  class={`flex-1 py-2 text-sm font-medium transition-colors ${
                    sortDir() === "ASC"
                      ? "bg-indigo-600 text-white"
                      : "bg-slate-700 text-slate-400 hover:text-slate-200"
                  }`}
                >
                  Ascending
                </button>
              </div>
            </div>
          </div>
        </div>

        {/* Preview count */}
        <Show when={savedId() !== null || isEditing()}>
          <div class="rounded-xl bg-slate-800/50 border border-slate-700 p-4 text-center">
            <Show
              when={!previewLoading()}
              fallback={
                <p class="text-sm text-slate-400">Calculating matches…</p>
              }
            >
              <Show
                when={previewCount() !== null}
                fallback={<p class="text-sm text-slate-500">Save to see preview count</p>}
              >
                <p class="text-sm text-slate-300">
                  Matches{" "}
                  <span class="font-bold text-indigo-400">{previewCount()}</span>{" "}
                  {previewCount() === 1 ? "book" : "books"}
                </p>
              </Show>
            </Show>
          </div>
        </Show>

        {/* Error */}
        <Show when={error()}>
          <p class="text-sm text-red-400">{error()}</p>
        </Show>

        {/* Actions */}
        <div class="flex justify-end gap-3 pb-4">
          <Button
            variant="ghost"
            type="button"
            onClick={() => navigate("/shelves")}
          >
            Cancel
          </Button>
          <Button variant="primary" type="submit" loading={saving()}>
            {isEditing() ? "Save Changes" : "Create Magic Shelf"}
          </Button>
        </div>
      </form>
    </div>
  );
};

// ---- Page wrapper ----

const MagicShelfBuilder: Component = () => {
  const params = useParams<{ id?: string }>();
  const editId = () => (params.id ? parseInt(params.id, 10) : undefined);

  return <MagicShelfBuilderInner editId={editId()} />;
};

export default MagicShelfBuilder;
