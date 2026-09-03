---
title: Notification Preferences
description: Choose how you get paged — pick channels for each kind of notification.
---

# Notification Preferences

Choose **how you hear about things**: which channels ping you for pages, handoffs, mentions, and everything else.

## How It Works

You set up rules like "send pages to chat and phone, send everything else to in-app." When something happens, Alga finds the first rule matching that kind of event. Anything without a matching rule goes to your default channel.

## Channels

| Channel    | What it is                                           | What you need                                                |
| ---------- | ---------------------------------------------------- | ------------------------------------------------------------ |
| **In-app** | Bell icon in the web app                             | Nothing — always works                                       |
| **Email**  | Email to your address                                | Email set up by your admin                                   |
| **Slack**  | DM from Alga                                         | Link your Slack account first (Profile → Connected Accounts) |
| **Voice**  | Phone call where you can acknowledge with the keypad | A phone number on file                                       |

You can opt out of voice calls entirely — when opted out, Alga skips phone calls even if a policy includes them. You only need a phone number if you want voice calls; rotas and other channels work without one.

## Changing Your Preferences

1. Click your **profile avatar** in the top-right corner.
2. Select **Notification Preferences**.
3. Set your **default channel** (used when no rule matches).
4. Add or edit rules — pick the kind of event, tick the channels you want, turn each rule on or off.
5. Click **Save**, then use **Test** to check in-app delivery.

The test button only sends an in-app ping. To check email, Slack, or voice, trigger a real notification (for example, a test incident assigned to yourself).

## Tips

- **Reserve phone calls for pages** — keep voice on for escalations only, not everything.
- **Mirror to Slack** — keep pages and mentions going to Slack so you see them where you work.
- **Keep a default** — so you never miss a kind of event you forgot to add a rule for.
- **Turn off instead of deleting** — you can re-enable a rule later without rebuilding it.

## See Also

- [Teams](/on-call/) — teams and rotas
- [Escalation Policies](/on-call/escalation-policies) — who gets paged and when
- [On-Call Schedules](/on-call/schedules) — who's on call
