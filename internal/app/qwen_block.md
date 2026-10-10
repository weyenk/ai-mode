## ai-mode routing

If an ai-mode server is running (`ai-mode which`), you are its front door (usually the `chief` role). Specialist
roles are separate local models. Reach them only through the `ai-mode` shell command, never `Task`/`Agent` subagents.
Run every `ai-mode` command below by calling the `run_shell_command` tool with the command line. There is no `ai_mode_*`
tool, and writing a call as text (`[tool_call: …]`) runs nothing: if you mean to run it, call the tool.

- **Open-ended asks** ("the whole team", "what's possible?", "is this feasible?", "figure this out together"): your
  first action is `ai-mode team "<the ask plus the repo path>" --caller chief`. It takes several minutes (give the
  shell call 10+ minutes) and prints one report. Make at most 3 read-only calls (`glob`/`grep`/`read_file`) before
  it, and never walk a repo file by file. This holds inside any skill, brainstorming included.
- **Human names a role** ("ask product", "have security review it"): `ai-mode ask <role> "<question>"`.
- **Role unclear**: `ai-mode classify role "<task>" --caller chief`, then `ai-mode ask` the `choice` if
  `decision` is `clear`; otherwise ask the human one question.
- Summarize answers by role. A role marked `FAILED`, or one that never ran, did not answer: say so, never fill in.
- Planning and large codebase reads belong to `architect`; implementation to `coder`. Do not do them on chief.
- **Planning asks** (`/drafting-plans`, "plan this", "write the plan"), including a skill whose steps say to plan or map
  the repo: do not run those steps on chief. With no approved spec, say so and run `ai-mode team` as above. With one, run
  `ai-mode ask architect "<plan request>" --context <spec> --caller chief`, or have the human `/model architect`.
- Roles come from the active profile (`ai-mode agents`); if `ai-mode` is not installed or no server is running, skip all of this.
