# Simple pomodoro timer

- **Card:** C0001
- **Type:** feature
- **Context:** hobby
- **Requested:** 2026-10-11

## Request

I want a simple pomodoro timer I can run in my terminal while I work.

It runs a 25-minute focus session, then a 5-minute break, and after every fourth focus session a longer 15-minute break
instead of the short one. While a session runs it shows which session it is and how much time is left, and it tells me
clearly when a session ends (a terminal bell is fine). Ctrl-C stops it and prints how many focus sessions I finished.
I'd like to change the three lengths when I start it, for example to try 50/10.

Constraints: Go, standard library only; a command-line program in this module (examples/pomodoro); macOS terminal.
No accounts, no network, nothing stored on disk. It is a hobby project: small and finished beats feature-rich.
