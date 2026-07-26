// Pre-seed Zod v4 global config before any module script evaluates.
// This file is loaded as a classic (non-module) script in index.html so it
// runs synchronously before the vendor chunk (which contains Zod). Zod reads
// globalThis.__zod_globalConfig at module init; jitless prevents the JIT
// probe (Function("")) from firing under strict CSP (script-src 'self').
globalThis.__zod_globalConfig = { jitless: true };
