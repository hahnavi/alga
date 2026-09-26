---
title: Triage
description: How Alga filters noise before it reaches you.
---

# Triage

Triage is Alga's spam filter. It looks at incoming alerts and decides: investigate, escalate, quietly resolve, or ignore.

## How it works

1. **Simple rules first** — these are fast and free. Your team writes things like "ignore test environment noise" or "escalate anything critical in production." The first matching rule decides.
2. **AI for the rest** — if no rule matches, Alga asks its AI to classify the alert and suggest what to do.
3. **Alga learns** — if your team keeps agreeing with the same decision for the same kind of alert, Alga starts applying that decision directly and skips the AI step.

## What you see

There is no triage page in the UI — triage happens behind the scenes before an investigation starts.

What you do see is the result: alerts that matter get investigated and show up in your channels, noise gets suppressed or resolved quietly.

## Overriding a decision

If Alga gets it wrong, you can change the decision through the API or command line. Your reason is saved alongside the change so the team can see why it was overruled, and it helps Alga make better calls next time.

You can also check accuracy stats from the command line to see how often the team agrees with triage.

## Tips

- Start with a few broad rules for obvious noise, then add more specific ones as you go.
- Put specific rules before general ones.
- Prefer adding helpful context over deleting alerts you might want later.
