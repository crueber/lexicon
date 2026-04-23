import { createSignal } from "solid-js";

// Import the English locale (default).
import en from "./en.json";

type LocaleData = Record<string, Record<string, string>>;

const locales: Record<string, LocaleData> = {
  en,
};

const [currentLocale, setLocale] = createSignal<string>("en");

// t translates a key in the format "namespace.key" or "namespace.key.nested".
// If the key is missing, returns the key itself as fallback.
export function t(key: string): string {
  const locale = currentLocale();
  const data = locales[locale] ?? locales["en"];
  const parts = key.split(".");

  let value: any = data;
  for (const part of parts) {
    if (value && typeof value === "object" && part in value) {
      value = value[part];
    } else {
      return key;
    }
  }

  return typeof value === "string" ? value : key;
}

export { currentLocale, setLocale };
