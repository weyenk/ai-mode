Blocked: this skill runs on architect, and chief never writes plans. Write no plan text yourself and never write files into a skill's directory. If the spec is approved, your next output is one real run_shell_command call, with the real spec path and a short topic:

ai-mode ask architect "Write the implementation plan for this spec with the drafting-plans skill." --context <spec path> --caller chief --max-tokens 32768 > docs/superpowers/plans/<YYYY-MM-DD>-<topic>.md

Then run `ai-mode plan lint` on that file and tell the human where the plan is and what lint reported. If there is no approved spec yet, call the `skill` tool with skill "ai-mode-routing" instead.
