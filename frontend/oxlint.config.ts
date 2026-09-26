import { existsSync, readdirSync } from "node:fs";
import { posix } from "node:path";

import { defineConfig } from "oxlint";

const root = new URL("./", import.meta.url);
const layerOrder = ["app", "pages", "widgets", "features", "entities", "shared"] as const;

function directories(path: string): string[] {
  const directory = new URL(path, root);
  if (!existsSync(directory)) return [];

  return readdirSync(directory, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => `${path}/${entry.name}`)
    .sort();
}

function descendants(path: string): string[] {
  return [path, ...directories(path).flatMap(descendants)];
}

function escapeRegex(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function restriction(directory: string, target: string, message: string) {
  const relative = posix.relative(directory, target);
  const source = relative.startsWith(".") ? relative : `./${relative}`;
  const alias = target.startsWith("src/") ? target.replace(/^src\//, "@/") : target;

  return {
    regex: `^(?:${escapeRegex(alias)}|${escapeRegex(source)})(?:/|$)`,
    message,
    allowTypeImports: false,
  };
}

function bindingsRestriction(directory: string) {
  const relative = posix.relative(directory, "bindings");
  const source = relative.startsWith(".") ? relative : `./${relative}`;

  return {
    regex: `^(?:@/\\.\\./bindings|${escapeRegex(source)})(?:/|$)`,
    message: "Wails Bindingはsrc/shared/api/wailsだけから参照してください。",
    allowTypeImports: false,
  };
}

const boundaries: NonNullable<Parameters<typeof defineConfig>[0]["overrides"]> = descendants(
  "src",
).flatMap((directory) => {
  const rootDirectory = directory.split("/")[1];
  const layer = rootDirectory === "components" ? "shared" : rootDirectory;
  const layerIndex = layerOrder.indexOf(layer as (typeof layerOrder)[number]);
  const patterns = [];

  if (layerIndex >= 0) {
    for (const forbidden of layerOrder.slice(0, layerIndex)) {
      patterns.push(
        restriction(
          directory,
          `src/${forbidden}`,
          `${layer}から${forbidden}への逆参照は禁止です。`,
        ),
      );
    }
  }

  if (!directory.startsWith("src/shared/api/wails")) {
    patterns.push(bindingsRestriction(directory));
  }

  if (patterns.length === 0) return [];

  return [
    {
      files: [`${directory}/*.{ts,tsx,js,jsx,mts,mjs,cts,cjs}`],
      rules: {
        "eslint/no-restricted-imports": ["error", { patterns }],
      },
    },
  ];
});

export default defineConfig({
  plugins: ["typescript", "unicorn", "oxc", "react", "import", "promise"],
  categories: { correctness: "error" },
  options: { typeAware: true },
  rules: {
    "import/no-cycle": "error",
    "import/no-self-import": "error",
    "promise/no-multiple-resolved": "error",
    "promise/no-return-in-finally": "error",
    "promise/valid-params": "error",
    "react/rules-of-hooks": "error",
    "react/exhaustive-deps": "error",
    "typescript/no-explicit-any": "error",
    "typescript/ban-ts-comment": [
      "error",
      { "ts-ignore": true, "ts-nocheck": true, "ts-expect-error": "allow-with-description" },
    ],
    "typescript/no-floating-promises": ["error", { ignoreVoid: false }],
    "typescript/no-misused-promises": "error",
    "typescript/no-unsafe-assignment": "error",
    "typescript/switch-exhaustiveness-check": [
      "error",
      { considerDefaultExhaustiveForUnions: false },
    ],
  },
  overrides: [
    ...boundaries,
    {
      files: ["**/*.test.ts", "**/*.test.tsx"],
      plugins: ["typescript", "unicorn", "oxc", "react", "import", "promise", "vitest"],
      rules: {
        "vitest/no-focused-tests": "error",
        "vitest/valid-expect": "error",
      },
    },
  ],
  ignorePatterns: ["dist/**", "storybook-static/**", "bindings/**"],
});
