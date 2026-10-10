## product review
### Blocking
- The **work board** and **companion services** are out of scope for the current design spec. They introduce new components (Vikunja integration, per-profile service concept) that are not aligned with the core goal of the `ai-mode amigos` tool, which is to facilitate structured conversations around a single story before implementation. These additions should be split into separate specs and reviewed independently.

### Non-blocking
- The **acceptance criteria** are clear and testable, though they could benefit from more explicit references to the work board and companion services if they were included in scope.
- The **user stories** align with the core functionality of the `ai-mode amigos` tool. They do not directly reference the work board or companion services, which is consistent with them being out of scope.

### Proposed edits
- **Split the work board and companion services into a separate spec** to avoid scope creep and ensure that the current design spec remains focused on the core functionality of `ai-mode amigos`.
- Clarify in the **out of scope** section that the work board and companion services are not part of this spec.
- Remove any references to the work board or companion services from the **acceptance criteria**, **user stories**, and **problem/success metrics** sections unless they are explicitly included in scope.
