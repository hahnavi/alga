/**
 * Tiny Node ESM loader that resolves the @/ → apps/frontend/src/ alias
 * used in source files. Without it, the @/lib/* imports that the
 * composables carry can't be loaded under `node --test`.
 *
 * Used only by the test runner (see package.json scripts / docs); not
 * part of the build chain.
 */
import { pathToFileURL } from "node:url";
import { resolve as resolvePath } from "node:path";
import { existsSync, statSync } from "node:fs";

const PROJECT_SRC = resolvePath(process.cwd(), "apps/frontend/src");

export function resolve(specifier, context, nextResolve) {
  if (specifier.startsWith("@/")) {
    const rel = specifier.slice(2);
    const candidates = [
      resolvePath(PROJECT_SRC, `${rel}.ts`),
      resolvePath(PROJECT_SRC, `${rel}.tsx`),
      resolvePath(PROJECT_SRC, rel, "index.ts"),
    ];
    for (const c of candidates) {
      if (existsSync(c) && statSync(c).isFile()) {
        return { url: pathToFileURL(c).href, shortCircuit: true };
      }
    }
  }
  return nextResolve(specifier, context);
}
