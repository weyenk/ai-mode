## research review
**Blocking**  
- none  

**Non-blocking**  
- Terminal bell (`\a`) may produce no audible sound in macOS terminal configurations where bell is disabled (e.g., default iTerm2 settings), though this aligns with standard CLI practice and spec requirements.  
- Spec assumes all macOS terminals support `\r` for in-place updates; some legacy terminals may require `^M` rendering, but Go's `os.Stdout` handles this correctly per [Go 1.18+ TTY behavior](https://golang.org/doc/go1.18#terminal).  

**Proposed edits**  
- none
