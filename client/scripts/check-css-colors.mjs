import { readdir, readFile } from "node:fs/promises";
import path from "node:path";

const sourceDirectory = path.resolve("src");
const colorLiteralPattern = /#[\da-f]{3,8}\b|\b(?:rgb|rgba|hsl|hsla|hwb|lab|lch|oklab|oklch|color)\s*\(/i;
const colorPropertyPattern = /^(?:color|background(?:-color)?|border(?:-(?:top|right|bottom|left))?(?:-color)?|outline(?:-color)?|box-shadow|text-shadow|fill|stroke|caret-color|accent-color|column-rule(?:-color)?|text-decoration-color)$/;
const colorlessValues = new Set([
  "0",
  "none",
  "inherit",
  "initial",
  "unset",
  "revert",
  "revert-layer",
]);

async function findCssFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = await Promise.all(
    entries.map((entry) => {
      const entryPath = path.join(directory, entry.name);
      if (entry.isDirectory()) return findCssFiles(entryPath);
      return entry.name.endsWith(".css") ? [entryPath] : [];
    }),
  );

  return files.flat();
}

const violations = [];

for (const file of await findCssFiles(sourceDirectory)) {
  const source = await readFile(file, "utf8");
  const lines = source.split("\n");
  lines.forEach((line, index) => {
    if (colorLiteralPattern.test(line)) {
      violations.push(`${path.relative(process.cwd(), file)}:${index + 1} ${line.trim()}`);
    }
  });

  const sourceWithoutComments = source.replace(/\/\*[\s\S]*?\*\//g, "");
  const declarationPattern = /([\w-]+)\s*:\s*([^;{}]+);/g;
  for (const match of sourceWithoutComments.matchAll(declarationPattern)) {
    const [, property, rawValue] = match;
    const value = rawValue.trim();
    if (
      colorPropertyPattern.test(property) &&
      !colorlessValues.has(value) &&
      !value.includes("var(--color-")
    ) {
      const lineNumber = sourceWithoutComments.slice(0, match.index).split("\n").length;
      violations.push(
        `${path.relative(process.cwd(), file)}:${lineNumber} ${property}: ${value};`,
      );
    }
  }
}

if (violations.length > 0) {
  console.error("CSS colors must use a --color-* theme variable:\n");
  console.error(violations.join("\n"));
  process.exitCode = 1;
}
