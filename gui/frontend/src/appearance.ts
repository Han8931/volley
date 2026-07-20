import { useLayoutEffect, useState } from "react";

export type ThemeID = "aurora" | "midnight" | "graphite" | "daylight";
export type Density = "comfortable" | "compact";
export type CodeSize = "small" | "medium" | "large";

export interface Appearance {
  theme: ThemeID;
  density: Density;
  codeSize: CodeSize;
}

export const DEFAULT_APPEARANCE: Appearance = {
  theme: "aurora",
  density: "comfortable",
  codeSize: "medium",
};

// colors are [--bg, --accent, --ok] of the theme's actual tokens — previews
// must not drift from what the theme renders.
export const THEMES: { id: ThemeID; name: string; description: string; colors: string[] }[] = [
  {
    id: "aurora",
    name: "Aurora",
    description: "Deep violet with a cool glow",
    colors: ["#0c0e15", "#8b7cf6", "#35d0a6"],
  },
  {
    id: "midnight",
    name: "Midnight",
    description: "Focused blue on deep navy",
    colors: ["#07111f", "#60a5fa", "#34d399"],
  },
  {
    id: "graphite",
    name: "Graphite",
    description: "Quiet neutral with mint accents",
    colors: ["#101211", "#55d6a9", "#67c9df"],
  },
  {
    id: "daylight",
    name: "Daylight",
    description: "Clean porcelain and indigo",
    colors: ["#f4f6fb", "#635bff", "#078f69"],
  },
];

const STORAGE_KEY = "volley.appearance.v1";

function loadAppearance(): Appearance {
  try {
    const saved = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "{}") as Partial<Appearance>;
    return {
      theme: THEMES.some((theme) => theme.id === saved.theme) ? saved.theme! : DEFAULT_APPEARANCE.theme,
      density: saved.density === "compact" ? "compact" : "comfortable",
      codeSize: saved.codeSize === "small" || saved.codeSize === "large" ? saved.codeSize : "medium",
    };
  } catch {
    return DEFAULT_APPEARANCE;
  }
}

export function useAppearance() {
  const [appearance, setAppearance] = useState<Appearance>(loadAppearance);

  // Apply before paint so a saved light theme never flashes the dark default.
  useLayoutEffect(() => {
    const root = document.documentElement;
    root.dataset.theme = appearance.theme;
    root.dataset.density = appearance.density;
    root.dataset.codeSize = appearance.codeSize;
    root.style.colorScheme = appearance.theme === "daylight" ? "light" : "dark";
    localStorage.setItem(STORAGE_KEY, JSON.stringify(appearance));
  }, [appearance]);

  return [appearance, setAppearance] as const;
}
