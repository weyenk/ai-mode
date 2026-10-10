package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var errAuditQuota = errors.New("audit size cap exceeded")

var auditSecretRe = regexp.MustCompile(`\b(tk_|sk_|sk-)[A-Za-z0-9]{20,}`)

func redactSecrets(s string) string { return auditSecretRe.ReplaceAllString(s, "[REDACTED]") }

type fileAudit struct {
	level    string
	dir      string
	maxBytes int64
	written  int64
}

func newFileAudit(level, dir string, maxBytes int64) (auditSink, error) {
	switch level {
	case "off", "summary", "full":
	default:
		return nil, fmt.Errorf("audit level must be off, summary or full, got %q", level)
	}
	if level != "off" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return nil, err
		}
	}
	return &fileAudit{level: level, dir: dir, maxBytes: maxBytes}, nil
}

func (a *fileAudit) Level() string { return a.level }

func (a *fileAudit) Write(kind string, v any) error {
	var file string
	appendMode := true
	switch kind {
	case "contract":
		file, appendMode = "contract.json", false
	case "decision":
		file = "decisions.jsonl"
	case "parked":
		file = "parked.jsonl"
	case "transcript":
		file = "transcript.jsonl"
	default:
		return fmt.Errorf("unknown audit kind %q", kind)
	}
	if a.level == "off" || (kind == "transcript" && a.level != "full") {
		return nil
	}
	var data []byte
	var err error
	if appendMode {
		data, err = json.Marshal(v)
		data = append(data, '\n')
	} else {
		data, err = json.MarshalIndent(v, "", "  ")
		data = append(data, '\n')
	}
	if err != nil {
		return err
	}
	if a.level == "full" {
		data = []byte(redactSecrets(string(data)))
	}
	if a.maxBytes > 0 && a.written+int64(len(data)) > a.maxBytes {
		return errAuditQuota
	}
	flags := os.O_WRONLY | os.O_CREATE
	if appendMode {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(filepath.Join(a.dir, file), flags, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	a.written += int64(len(data))
	return nil
}

func loadSnapshot(baseDir, meetingID string) (Snapshot, error) {
	if meetingID == "" || strings.ContainsAny(meetingID, `/\`) || strings.Contains(meetingID, "..") {
		return Snapshot{}, fmt.Errorf("invalid meeting id %q", meetingID)
	}
	data, err := os.ReadFile(filepath.Join(baseDir, meetingID, "contract.json"))
	if err != nil {
		return Snapshot{}, fmt.Errorf("meeting %s: %w", meetingID, err)
	}
	var s Snapshot
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&s); err != nil {
		return Snapshot{}, fmt.Errorf("meeting %s: %w", meetingID, err)
	}
	if s.State.MeetingID != meetingID {
		return Snapshot{}, fmt.Errorf("meeting %s: contract.json belongs to %q", meetingID, s.State.MeetingID)
	}
	return s, nil
}
