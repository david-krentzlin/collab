package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/david-krentzlin/collab/internal/message"
)

const (
	CollabDir   = ".collab"
	DefaultTask = "default"
	TaskEnv     = "COLLAB_TASK"
	SeqFile     = ".seq"
	SeqLock     = ".seq.lock"
	IndexFile   = ".index.jsonl"
	IndexLock   = ".index.lock"
	BroadcastTo = "all"
	seqRetries  = 64
)

type Store struct {
	Root string // absolute path to .collab/<task>/
	Task string // task identifier (can include subpaths like feature/foo)
}

// Find walks up from dir to find an existing .collab directory.
// If none found, returns the default task location (dir/.collab/<task>).
func Find(dir string) *Store {
	cur := dir
	task := resolveTask(os.Getenv(TaskEnv))
	for {
		candidate := filepath.Join(cur, CollabDir)
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return &Store{Root: filepath.Join(candidate, task), Task: task}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return &Store{Root: filepath.Join(dir, CollabDir, task), Task: task}
}

func resolveTask(raw string) string {
	task := strings.TrimSpace(raw)
	if task == "" {
		return DefaultTask
	}

	task = filepath.Clean(task)
	if task == "." || task == string(filepath.Separator) {
		return DefaultTask
	}
	if filepath.IsAbs(task) {
		return DefaultTask
	}
	if task == ".." || strings.HasPrefix(task, ".."+string(filepath.Separator)) {
		return DefaultTask
	}

	return task
}

// Init creates the .collab directory and agent subdirectories.
func (s *Store) Init(agents []string) error {
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return fmt.Errorf("create collab dir: %w", err)
	}
	for _, agent := range agents {
		agentDir := filepath.Join(s.Root, agent)
		if err := os.MkdirAll(agentDir, 0o755); err != nil {
			return fmt.Errorf("create agent dir %s: %w", agent, err)
		}
	}
	if err := s.ensureSeqFile(); err != nil {
		return err
	}
	if err := s.ensureIndexFile(); err != nil {
		return err
	}
	return nil
}

// NextSeq allocates the next global sequence number with a process-safe file lock.
func (s *Store) NextSeq() (int, error) {
	if err := s.ensureSeqFile(); err != nil {
		return 0, err
	}

	seqPath := filepath.Join(s.Root, SeqFile)
	lockPath := filepath.Join(s.Root, SeqLock)
	for i := 0; i < seqRetries; i++ {
		snapshot, err := readSeq(seqPath)
		if err != nil {
			return 0, err
		}

		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			return 0, fmt.Errorf("open seq lock file: %w", err)
		}

		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
			_ = f.Close()
			return 0, fmt.Errorf("lock seq file: %w", err)
		}

		current, err := readSeq(seqPath)
		if err != nil {
			_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			_ = f.Close()
			return 0, err
		}

		if current != snapshot {
			_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			_ = f.Close()
			time.Sleep(2 * time.Millisecond)
			continue
		}

		next := current + 1
		if err := writeSeq(seqPath, next); err != nil {
			_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			_ = f.Close()
			return 0, err
		}

		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
			_ = f.Close()
			return 0, fmt.Errorf("unlock seq file: %w", err)
		}
		if err := f.Close(); err != nil {
			return 0, fmt.Errorf("close seq lock file: %w", err)
		}

		return next, nil
	}

	return 0, fmt.Errorf("allocate next seq: optimistic retries exceeded")
}

// CreateMessage allocates a sequence number and writes the message file.
func (s *Store) CreateMessage(msg *message.Message) (string, error) {
	if msg == nil {
		return "", fmt.Errorf("message is required")
	}

	seq, err := s.NextSeq()
	if err != nil {
		return "", err
	}
	msg.Seq = seq

	return s.WriteMessage(msg)
}

// WriteMessage writes a message file into the sender's directory.
func (s *Store) WriteMessage(msg *message.Message) (string, error) {
	path, err := s.writeMessageFile(msg)
	if err != nil {
		return "", err
	}
	if err := s.appendIndexRecord(msg, path); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func (s *Store) writeMessageFile(msg *message.Message) (string, error) {
	agentDir := filepath.Join(s.Root, msg.From)
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return "", fmt.Errorf("ensure agent dir: %w", err)
	}
	filename := fmt.Sprintf("%03d-%s.md", msg.Seq, msg.Type)
	path := filepath.Join(agentDir, filename)
	tmp, err := os.CreateTemp(agentDir, ".message-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create message temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(msg.Marshal()); err != nil {
		tmp.Close()
		return "", fmt.Errorf("write message temp file: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return "", fmt.Errorf("chmod message temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close message temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return "", fmt.Errorf("rename message temp file: %w", err)
	}
	return path, nil
}

// ReadMessageAtPath reads and parses a message from a known filesystem path.
func (s *Store) ReadMessageAtPath(path string) (*message.Message, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read message file: %w", err)
	}
	msg, err := message.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("parse message file: %w", err)
	}
	return msg, nil
}

func (s *Store) ensureSeqFile() error {
	seqPath := filepath.Join(s.Root, SeqFile)
	if _, err := os.Stat(seqPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat seq file: %w", err)
	}
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return fmt.Errorf("create collab task dir: %w", err)
	}

	if err := writeSeq(seqPath, 0); err != nil {
		return err
	}

	return nil
}

func readSeq(seqPath string) (int, error) {
	data, err := os.ReadFile(seqPath)
	if err != nil {
		return 0, fmt.Errorf("read seq file: %w", err)
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return 0, nil
	}

	seq, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("parse seq file: %w", err)
	}
	if seq < 0 {
		return 0, fmt.Errorf("invalid negative seq %d", seq)
	}

	return seq, nil
}

func writeSeq(seqPath string, seq int) error {
	tmp, err := os.CreateTemp(filepath.Dir(seqPath), ".seq-*.tmp")
	if err != nil {
		return fmt.Errorf("create seq temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := fmt.Fprintf(tmp, "%d\n", seq); err != nil {
		tmp.Close()
		return fmt.Errorf("write seq temp file: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod seq temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close seq temp file: %w", err)
	}

	if err := os.Rename(tmpPath, seqPath); err != nil {
		return fmt.Errorf("rename seq temp file: %w", err)
	}

	return nil
}

// MessageEntry is a lightweight index entry returned by List.
type MessageEntry struct {
	Seq     int
	From    string
	To      string
	Type    message.Type
	Re      int
	Summary string
	Status  message.Status
	Path    string
}

// List returns all messages across all agents, sorted by seq.
// If since > 0, only returns messages with seq > since.
// If excludeFrom is non-empty, excludes messages from that agent.
func (s *Store) List(since int, excludeFrom string) ([]MessageEntry, error) {
	records, err := s.readIndexRecords()
	if err != nil {
		return nil, err
	}
	var results []MessageEntry
	for _, record := range records {
		entry := record.toEntry()
		if excludeFrom != "" && entry.From == excludeFrom {
			continue
		}
		if since > 0 && entry.Seq <= since {
			continue
		}
		results = append(results, entry)
	}
	return results, nil
}

// ListForRecipient returns messages addressed to a specific recipient.
// It excludes messages sent by the recipient and includes broadcast messages (`to: all`).
func (s *Store) ListForRecipient(since int, recipient string) ([]MessageEntry, error) {
	if recipient == "" {
		return s.List(since, "")
	}

	records, err := s.readIndexRecords()
	if err != nil {
		return nil, err
	}

	var results []MessageEntry
	for _, record := range records {
		entry := record.toEntry()
		if entry.From == recipient {
			continue
		}
		if since > 0 && entry.Seq <= since {
			continue
		}
		if entry.To != recipient && !strings.EqualFold(entry.To, BroadcastTo) {
			continue
		}

		results = append(results, entry)
	}

	return results, nil
}

// Entry returns indexed metadata for a message sequence.
func (s *Store) Entry(seq int) (MessageEntry, error) {
	return s.entry(seq)
}

// ReadMessage reads and parses a message by seq number.
func (s *Store) ReadMessage(seq int) (*message.Message, error) {
	entry, err := s.entry(seq)
	if err != nil {
		return nil, err
	}

	return s.ReadMessageAtPath(entry.Path)
}

// Agents returns the list of agent directories.
func (s *Store) Agents() ([]string, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		return nil, err
	}
	var agents []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			agents = append(agents, e.Name())
		}
	}
	return agents, nil
}
