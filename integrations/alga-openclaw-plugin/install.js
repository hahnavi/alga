#!/usr/bin/env node
/*
 * Install / sync / uninstall the Alga plugin for OpenClaw.
 *
 * Idempotent: re-running produces the same state without duplicating entries.
 *
 * Usage:
 *   node install.js [options]
 *
 * Options:
 *   --profile <name>        Install to a named OpenClaw profile (default: ~/.openclaw)
 *   --server-url <url>      Alga backend base URL (env: ALGA_SERVER_URL)
 *   --token <token>         Alga agent token (env: ALGA_AGENT_TOKEN)
 *   --allowed-users <list>  Comma-separated OpenClaw user IDs (env: ALGA_ALLOWED_USERS)
 *   --link                  Symlink source into extension dir instead of copying (dev)
 *   --skip-build            Skip `npm install --omit=dev` when node_modules is missing
 *   --status                Check installation status and exit
 *   --uninstall             Remove plugin files and config entries
 *   -h, --help              Show this help
 */

import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import cp from "node:child_process";
import process from "node:process";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const PLUGIN_ID = "alga";
const PLUGIN_NAME = "Alga";

const RED = "\x1b[0;31m";
const GREEN = "\x1b[0;32m";
const YELLOW = "\x1b[1;33m";
const DIM = "\x1b[2m";
const BOLD = "\x1b[1m";
const NC = "\x1b[0m";

const log = {
  info: (m) => console.log(`${GREEN}[alga]${NC} ${m}`),
  warn: (m) => console.log(`${YELLOW}[alga]${NC} ${m}`),
  err: (m) => console.error(`${RED}[alga]${NC} ${m}`),
  die: (m) => {
    console.error(`${RED}[alga]${NC} ${m}`);
    process.exit(1);
  },
};

// Keep in sync with createAlgaCommandTools() in src/agent-tools.ts — the runtime
// allowlist mutation in src/index.ts is the authoritative enforcement path.
const ALGA_TOOL_NAMES = [
  "alga_resolve_alert",
  "alga_reopen_alert",
  "alga_promote_to_incident",
  "alga_set_outcome",
  "alga_cancel_investigation",
  "alga_pause_investigation",
  "alga_search_knowledge",
  "alga_get_knowledge",
  "alga_create_knowledge",
  "alga_list_alerts",
  "alga_triage_feedback",
  "alga_get_incident_context",
  "alga_get_incident_timeline",
  "alga_add_incident_timeline",
  "alga_who_is_on_call",
  "alga_list_services",
  "alga_search_memories",
  "alga_create_memory",
  "alga_peer_ask",
  "alga_assign_investigation",
  "alga_set_incident_priority",
  "alga_set_incident_severity",
  "alga_trigger_escalation",
  "alga_request_status_update",
  "alga_mitigate_incident",
  "alga_resolve_incident",
  "alga_begin_triage",
  "alga_promote_incident",
  "alga_assign_incident_role",
  "alga_post_handoff",
  "alga_publish_status_update",
  "alga_set_incident_resolution_docs",
];

const REQUIRED_PLUGIN_FILES = ["package.json", "openclaw.plugin.json", "dist"];

const RUNTIME_NPM_PACKAGES = ["@sinclair/typebox", "zod", "eventsource2"];

function parseArgs(argv) {
  const opts = {
    profile: "default",
    action: "install",
    serverUrl: null,
    token: null,
    allowedUsers: null,
    link: false,
    skipBuild: false,
  };

  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    switch (a) {
      case "--profile": {
        const v = argv[++i];
        if (!v) log.die("--profile requires a non-empty argument");
        opts.profile = v;
        break;
      }
      case "--server-url": {
        const v = argv[++i];
        if (!v) log.die("--server-url requires a non-empty argument");
        opts.serverUrl = v;
        break;
      }
      case "--token": {
        const v = argv[++i];
        if (!v) log.die("--token requires a non-empty argument");
        opts.token = v;
        break;
      }
      case "--allowed-users": {
        const v = argv[++i];
        if (!v) log.die("--allowed-users requires a non-empty argument");
        opts.allowedUsers = v;
        break;
      }
      case "--link":
        opts.link = true;
        break;
      case "--skip-build":
        opts.skipBuild = true;
        break;
      case "--status":
        opts.action = "status";
        break;
      case "--uninstall":
        opts.action = "uninstall";
        break;
      case "-h":
      case "--help":
        printUsage();
        process.exit(0);
      default:
        log.err(`Unknown option: ${a}`);
        printUsage();
        process.exit(1);
    }
  }

  if (!opts.serverUrl && process.env.ALGA_SERVER_URL) {
    opts.serverUrl = process.env.ALGA_SERVER_URL.replace(/\/+$/, "");
  }
  if (!opts.token && process.env.ALGA_AGENT_TOKEN) {
    opts.token = process.env.ALGA_AGENT_TOKEN;
  }
  if (!opts.allowedUsers && process.env.ALGA_ALLOWED_USERS) {
    opts.allowedUsers = process.env.ALGA_ALLOWED_USERS;
  }

  return opts;
}

function printUsage() {
  console.log(`${BOLD}Usage:${NC} node install.js [options]`);
  console.log("");
  console.log(`${BOLD}Options:${NC}`);
  console.log(`  --profile <name>        Install to a named OpenClaw profile (default: default profile)`);
  console.log(`  --server-url <url>      Alga backend base URL (env: ALGA_SERVER_URL)`);
  console.log(`  --token <token>         Alga agent token (env: ALGA_AGENT_TOKEN)`);
  console.log(`  --allowed-users <list>  Comma-separated OpenClaw user IDs (env: ALGA_ALLOWED_USERS)`);
  console.log(`  --link                  Symlink source into extension dir instead of copying (dev)`);
  console.log(`  --skip-build            Skip \`npm install --omit=dev\` when node_modules is missing`);
  console.log(`  --status                Check installation status`);
  console.log(`  --uninstall             Remove plugin files and config entries`);
  console.log(`  -h, --help              Show this help`);
}

function resolveHome(opts) {
  const envHome = process.env.OPENCLAW_HOME;
  let base;
  if (envHome) {
    base = envHome;
  } else if (opts.profile === "default") {
    base = path.join(os.homedir(), ".openclaw");
  } else {
    base = path.join(os.homedir(), `.openclaw-${opts.profile}`);
  }
  return {
    base,
    configPath: path.join(base, "openclaw.json"),
    extensionDir: path.join(base, "extensions", PLUGIN_ID),
  };
}

function readJson(p, fallback) {
  if (!fs.existsSync(p)) {
    return fallback === undefined ? {} : fallback;
  }
  const text = fs.readFileSync(p, "utf8");
  try {
    return JSON.parse(text);
  } catch (e) {
    log.die(`Failed to parse ${p}: ${e.message}`);
  }
}

function writeJsonAtomic(p, value) {
  const dir = path.dirname(p);
  fs.mkdirSync(dir, { recursive: true });
  const tmp = `${p}.tmp-${process.pid}-${Date.now()}`;
  fs.writeFileSync(tmp, `${JSON.stringify(value, null, 2)}\n`);
  fs.renameSync(tmp, p);
}

function isFileEqual(a, b) {
  if (!fs.existsSync(a) || !fs.existsSync(b)) return false;
  const sa = fs.statSync(a);
  const sb = fs.statSync(b);
  if (sa.size !== sb.size) return false;
  return fs.readFileSync(a).equals(fs.readFileSync(b));
}

function copyIfChanged(src, dst) {
  if (isFileEqual(src, dst)) return false;
  fs.mkdirSync(path.dirname(dst), { recursive: true });
  fs.copyFileSync(src, dst);
  return true;
}

function copyDirIfChanged(src, dst) {
  if (!fs.existsSync(dst)) {
    fs.mkdirSync(dst, { recursive: true });
    copyDirRecursive(src, dst);
    return true;
  }
  const srcEntries = fs.readdirSync(src, { withFileTypes: true });
  const dstNames = new Set(fs.readdirSync(dst, { withFileTypes: true }).map((e) => e.name));
  let changed = false;
  for (const e of srcEntries) {
    const s = path.join(src, e.name);
    const d = path.join(dst, e.name);
    if (e.isDirectory()) {
      if (copyDirIfChanged(s, d)) changed = true;
    } else if (e.isFile()) {
      if (copyIfChanged(s, d)) changed = true;
    }
  }
  for (const name of dstNames) {
    if (!srcEntries.some((e) => e.name === name)) {
      fs.rmSync(path.join(dst, name), { recursive: true, force: true });
      changed = true;
    }
  }
  return changed;
}

function copyDirRecursive(src, dst) {
  fs.mkdirSync(dst, { recursive: true });
  for (const entry of fs.readdirSync(src, { withFileTypes: true })) {
    const s = path.join(src, entry.name);
    const d = path.join(dst, entry.name);
    if (entry.isDirectory()) copyDirRecursive(s, d);
    else if (entry.isFile()) fs.copyFileSync(s, d);
  }
}

function replaceWithSymlink(target, linkPath) {
  try {
    fs.lstatSync(linkPath);
    fs.rmSync(linkPath, { recursive: true, force: true });
  } catch (_) {
    // path does not exist
  }
  fs.mkdirSync(path.dirname(linkPath), { recursive: true });
  fs.symlinkSync(target, linkPath, "dir");
}

function runNpmInstall(cwd) {
  log.info(`Running \`npm install --omit=dev\` in ${cwd} ...`);
  const res = cp.spawnSync("npm", ["install", "--omit=dev", "--silent", "--no-audit", "--no-fund"], {
    cwd,
    stdio: "inherit",
    env: { ...process.env, NODE_ENV: "production" },
  });
  if (res.status !== 0) {
    log.die(`npm install failed (exit ${res.status})`);
  }
}

function checkNodeModules(extDir) {
  if (!fs.existsSync(path.join(extDir, "node_modules"))) return false;
  for (const pkg of RUNTIME_NPM_PACKAGES) {
    if (!fs.existsSync(path.join(extDir, "node_modules", pkg))) return false;
  }
  return true;
}

function findOpenclawCli() {
  const candidates = [
    process.env.OPENCLAW_CLI,
    path.join(os.homedir(), ".npm-global", "bin", "openclaw"),
    "/usr/local/bin/openclaw",
    "/usr/bin/openclaw",
  ].filter(Boolean);
  for (const c of candidates) {
    try {
      fs.accessSync(c, fs.constants.X_OK);
      return c;
    } catch (_) {}
  }
  return "openclaw";
}

function ensureAlgaConfig(config, values) {
  let changed = false;

  if (!config.plugins || typeof config.plugins !== "object") {
    config.plugins = {};
    changed = true;
  }
  if (!config.plugins.entries || typeof config.plugins.entries !== "object") {
    config.plugins.entries = {};
    changed = true;
  }
  const entries = config.plugins.entries;
  if (!entries[PLUGIN_ID] || entries[PLUGIN_ID].enabled !== true) {
    entries[PLUGIN_ID] = { ...(entries[PLUGIN_ID] || {}), enabled: true };
    changed = true;
  }

  if (!config.tools || typeof config.tools !== "object") {
    config.tools = {};
    changed = true;
  }
  const tools = config.tools;
  const alsoAllow = Array.isArray(tools.alsoAllow) ? [...tools.alsoAllow] : [];
  const have = new Set(alsoAllow);
  for (const t of ALGA_TOOL_NAMES) {
    if (!have.has(t)) {
      alsoAllow.push(t);
      have.add(t);
      changed = true;
    }
  }
  if (changed) {
    tools.alsoAllow = alsoAllow;
  }

  if (!config.channels || typeof config.channels !== "object") {
    config.channels = {};
    changed = true;
  }
  if (!config.channels[PLUGIN_ID] || typeof config.channels[PLUGIN_ID] !== "object") {
    config.channels[PLUGIN_ID] = {};
    changed = true;
  }
  const ch = config.channels[PLUGIN_ID];
  if (values.serverUrl && ch.serverUrl !== values.serverUrl) {
    ch.serverUrl = values.serverUrl;
    changed = true;
  }
  if (values.token && ch.token !== values.token) {
    ch.token = values.token;
    changed = true;
  }
  if (values.allowedUsers) {
    const list = values.allowedUsers
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    const existing = Array.isArray(ch.allowFrom) ? ch.allowFrom : [];
    const ex = new Set(existing);
    const merged = [...existing];
    let did = false;
    for (const u of list) {
      if (!ex.has(u)) {
        merged.push(u);
        ex.add(u);
        did = true;
      }
    }
    if (did || merged.length !== existing.length) {
      ch.allowFrom = merged;
      changed = true;
    }
  }
  if (ch.enabled !== true) {
    ch.enabled = true;
    changed = true;
  }

  return { changed };
}

function stripAlgaConfig(config) {
  let changed = false;
  if (config.plugins?.entries && config.plugins.entries[PLUGIN_ID]) {
    delete config.plugins.entries[PLUGIN_ID];
    changed = true;
  }
  if (Array.isArray(config.tools?.alsoAllow)) {
    const before = config.tools.alsoAllow.length;
    config.tools.alsoAllow = config.tools.alsoAllow.filter((t) => !t.startsWith("alga_"));
    if (config.tools.alsoAllow.length !== before) changed = true;
  }
  if (config.channels?.[PLUGIN_ID]) {
    delete config.channels[PLUGIN_ID];
    changed = true;
  }
  return changed;
}

function showStatus(home, sourceDir) {
  const cfg = fs.existsSync(home.configPath) ? readJson(home.configPath, null) : null;
  const haveExt = fs.existsSync(home.extensionDir);
  const haveNm = haveExt && checkNodeModules(home.extensionDir);

  console.log(`${BOLD}${PLUGIN_NAME} plugin status${NC}`);
  console.log("");

  let allOk = true;

  if (haveExt) {
    let outdated = 0;
    for (const f of REQUIRED_PLUGIN_FILES) {
      const s = path.join(sourceDir, f);
      const d = path.join(home.extensionDir, f);
      if (!fs.existsSync(s)) continue;
      const sStat = fs.statSync(s);
      if (sStat.isDirectory()) {
        if (!fs.existsSync(d) || !fs.statSync(d).isDirectory()) outdated += 1;
      } else if (!isFileEqual(s, d)) {
        outdated += 1;
      }
    }
    if (outdated === 0) {
      console.log(`  plugin files    ${GREEN}installed, up to date${NC}  ${DIM}${home.extensionDir}${NC}`);
    } else {
      console.log(`  plugin files    ${YELLOW}installed, ${outdated} file(s) outdated${NC}  (run install.js to sync)`);
      allOk = false;
    }
  } else {
    console.log(`  plugin files    ${RED}not installed${NC}  ${DIM}${home.extensionDir}${NC}`);
    allOk = false;
  }

  if (haveNm) {
    console.log(`  node_modules    ${GREEN}present${NC}`);
  } else if (haveExt) {
    console.log(`  node_modules    ${YELLOW}missing${NC}  ${DIM}run install.js to populate${NC}`);
    allOk = false;
  } else {
    console.log(`  node_modules    ${DIM}unknown${NC}`);
    allOk = false;
  }

  if (cfg) {
    const ch = cfg.channels?.[PLUGIN_ID];
    if (ch?.serverUrl && ch?.token) {
      console.log(`  channel config  ${GREEN}configured${NC}  ${DIM}serverUrl=${ch.serverUrl}${NC}`);
    } else {
      console.log(`  channel config  ${YELLOW}incomplete${NC}  ${DIM}set ALGA_SERVER_URL and ALGA_AGENT_TOKEN${NC}`);
      allOk = false;
    }

    const enabled = cfg.plugins?.entries?.[PLUGIN_ID]?.enabled === true;
    if (enabled) {
      console.log(`  plugin enabled  ${GREEN}yes${NC}`);
    } else {
      console.log(`  plugin enabled  ${YELLOW}no${NC}  ${DIM}run install.js to enable${NC}`);
      allOk = false;
    }

    const allow = new Set(cfg.tools?.alsoAllow ?? []);
    const missing = ALGA_TOOL_NAMES.filter((t) => !allow.has(t));
    if (missing.length === 0) {
      console.log(`  toolset         ${GREEN}enabled${NC}  ${DIM}(${ALGA_TOOL_NAMES.length} tools)${NC}`);
    } else {
      console.log(`  toolset         ${YELLOW}missing ${missing.length} tool(s)${NC}  ${DIM}run install.js to register${NC}`);
      allOk = false;
    }
  } else {
    console.log(`  channel config  ${YELLOW}no openclaw.json${NC}  ${DIM}${home.configPath}${NC}`);
    allOk = false;
    console.log(`  plugin enabled  ${DIM}unknown${NC}`);
    console.log(`  toolset         ${DIM}unknown${NC}`);
  }

  console.log("");
  if (allOk) {
    log.info("Everything is configured.");
  } else {
    log.warn("Some items need attention (see above).");
  }
  return allOk ? 0 : 1;
}

function uninstall(home) {
  if (fs.existsSync(home.extensionDir)) {
    fs.rmSync(home.extensionDir, { recursive: true, force: true });
    log.info(`Removed ${home.extensionDir}`);
  } else {
    log.warn(`Plugin not installed at ${home.extensionDir}`);
  }

  if (fs.existsSync(home.configPath)) {
    const cfg = readJson(home.configPath, {});
    if (stripAlgaConfig(cfg)) {
      writeJsonAtomic(home.configPath, cfg);
      log.info(`Removed ${PLUGIN_ID} entries from ${home.configPath}`);
    } else {
      log.info(`No ${PLUGIN_ID} entries in ${home.configPath}`);
    }
  } else {
    log.info(`No config file at ${home.configPath}`);
  }

  console.log("");
  const cli = findOpenclawCli();
  log.info(`Restart with: ${cli} gateway restart`);
}

function install(home, sourceDir, opts) {
  if (!fs.existsSync(home.base)) {
    fs.mkdirSync(home.base, { recursive: true });
    log.info(`Created ${home.base}`);
  }

  if (opts.link) {
    replaceWithSymlink(sourceDir, home.extensionDir);
    log.info(`Linked ${sourceDir} -> ${home.extensionDir}`);
  } else {
    fs.mkdirSync(home.extensionDir, { recursive: true });
    let created = 0;
    let updated = 0;
    for (const f of REQUIRED_PLUGIN_FILES) {
      const s = path.join(sourceDir, f);
      const d = path.join(home.extensionDir, f);
      if (!fs.existsSync(s)) log.die(`Required source missing: ${s}`);
      const existed = fs.existsSync(d);
      let changed = false;
      if (fs.statSync(s).isDirectory()) {
        changed = copyDirIfChanged(s, d);
      } else {
        changed = copyIfChanged(s, d);
      }
      if (changed) {
        if (existed) updated += 1;
        else created += 1;
      }
    }
    if (created > 0) log.info(`Installed plugin (${created} new path(s)) to ${home.extensionDir}`);
    else if (updated > 0) log.info(`Synced plugin (${updated} path(s) updated)`);
    else log.info(`Plugin files up to date`);
  }

  if (!opts.link && !checkNodeModules(home.extensionDir)) {
    if (opts.skipBuild) {
      log.warn("node_modules missing; --skip-build passed, gateway will fail to start");
    } else {
      runNpmInstall(home.extensionDir);
    }
  } else if (!opts.link) {
    log.info(`node_modules present in ${home.extensionDir}`);
  }

  const cfg = readJson(home.configPath, {});
  const { changed: cfgChanged } = ensureAlgaConfig(cfg, {
    serverUrl: opts.serverUrl,
    token: opts.token,
    allowedUsers: opts.allowedUsers,
  });
  if (cfgChanged) {
    writeJsonAtomic(home.configPath, cfg);
    log.info(`Updated ${home.configPath}`);
  } else {
    log.info(`${home.configPath} up to date`);
  }

  console.log("");
  log.info("Plugin installed and configured.");
  if (!opts.token || !opts.serverUrl) {
    log.warn("Set ALGA_SERVER_URL and ALGA_AGENT_TOKEN in env, or re-run with --server-url <url> --token <token>");
  }
  const cli = findOpenclawCli();
  if (opts.profile === "default") {
    log.info(`Restart gateway: ${cli} gateway restart`);
  } else {
    log.info(`Restart gateway: ${cli} --profile ${opts.profile} gateway restart`);
  }
}

function main() {
  const opts = parseArgs(process.argv.slice(2));
  const home = resolveHome(opts);
  const sourceDir = __dirname;

  if (opts.action === "status") {
    process.exit(showStatus(home, sourceDir));
  }
  if (opts.action === "uninstall") {
    uninstall(home);
    process.exit(0);
  }

  install(home, sourceDir, opts);
  console.log("");
  showStatus(home, sourceDir);
}

main();
