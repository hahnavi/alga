---
title: Maintenance Windows
description: Keep Alga quiet during planned work.
---

# Maintenance Windows

A maintenance window tells Alga "we're working on this on purpose — stay quiet." Matching alerts are still saved in the list, but nobody gets paged and no investigation starts.

## Creating a window

1. Go to **Maintenance** in the sidebar.
2. Click New and give it a name like "Database migration."
3. Set a start and end time.
4. Say which alerts to ignore — for example, "only alerts for namespace database." Leave it blank to cover everything.

When the end time passes, Alga goes back to normal by itself. You can also turn a window off early or delete it.

## When to use what

- **Maintenance window** — for temporary, planned work with a clear start and end.
- **Silenced routing rule** — for noise you want to mute forever. See [Routing](/core-features/routing).

## See also

- [Routing](/core-features/routing) — where alerts go when you're not in maintenance
- [Alerts](/core-features/alerts) — what happens to alerts that are suppressed
