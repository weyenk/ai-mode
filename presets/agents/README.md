# Agent guides

Each `*.md` file is a **system prompt** for a named model role in the presets.

## Use with clients

```bash
# Print system prompt for a role
ai-mode prompt product

# Use with curl / OpenAI clients
SYS="$(ai-mode prompt coder)"
curl http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d "$(jq -n --arg s "$SYS" '{
    model:"coder",
    messages:[
      {role:"system", content:$s},
      {role:"user", content:"…"}
    ]
  }')"
```

Frontmatter fields:

| Key | Meaning |
| --- | --- |
| `role` | Must match the `[section]` name in a preset `.ini` |
| `profile` | Which studio preset this role belongs to |
| `model` | Same as `role` (OpenAI `model` field) |
| `trigger` | Optional exact user phrase the model expects |

Load order for `ai-mode prompt <role>`: `presets/agents/<role>.md`.

**jev (System One):** use two calls — Stage A `role`, then Stage B `test_layer` when the
task is qa / test strategy (`jev.md`). QA consumes the layer; it does not replace jev's
classification.
