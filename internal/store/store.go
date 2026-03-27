package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/david-krentzlin/collab/internal/message"
)

const (
	CollabDir = ".collab"
	SeqFile   = ".seq"
)

type Store struct {
	Root string // absolute path to .collab/
}

// Find walks up from dir to find an existing .collab directory.
// If none found, returns the default location (dir/.collab).
func Find(dir string) *Store {
	cur := dir
	for {
		candidate := filepath.Join(cur, CollabDir)
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return &Store{Root: candidate}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return &Store{Root: filepath.Join(dir, CollabDir)}
}

// Init creates the .collab directory and agent subdirectories.
func (s *Store) Init(agents []string) error {
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return fmt.Errorf("create collab dir: %w", err)
	}
	// Initialize sequence counter
	seqPath := filepath.Join(s.Root, SeqFile)
	if err := os.WriteFile(seqPath, []byte("0\n"), 0o644); err != nil {
		return fmt.Errorf("create seq file: %w", err)
	}
	for _, agent := range agents {
		agentDir := filepath.Join(s.Root, agent)
		if err := os.MkdirAll(agentDir, 0o755); err != nil {
			return fmt.Errorf("create agent dir %s: %w", agent, err)
		}
	}
	return nil
}

// NextSeq atomically-enough increments and returns the next sequence number.
// Uses a temp file + rename for pseudo-atomic write.
func (s *Store) NextSeq() (int, error) {
	seqPath := filepath.Join(s.Root, SeqFile)
	data, err := os.ReadFile(seqPath)
	if err != nil {
		return 0, fmt.Errorf("read seq: %w", err)
	}
	current, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse seq: %w", err)
	}
	next := current + 1
	// Write to temp then rename for pseudo-atomicity
	tmp := seqPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(fmt.Sprintf("%d\n", next)), 0o644); err != nil {
		return 0, fmt.Errorf("write seq tmp: %w", err)
	}
	if err := os.Rename(tmp, seqPath); err != nil {
		return 0, fmt.Errorf("rename seq: %w", err)
	}
	return next, nil
}

// WriteMessage writes a message file into the sender's directory.
func (s *Store) WriteMessage(msg *message.Message) (string, error) {
	agentDir := filepath.Join(s.Root, msg.From)
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return "", fmt.Errorf("ensure agent dir: %w", err)
	}
	filename := fmt.Sprintf("%03d-%s.md", msg.Seq, msg.Type)
	path := filepath.Join(agentDir, filename)
	if err := os.WriteFile(path, msg.Marshal(), 0o644); err != nil {
		return "", fmt.Errorf("write message: %w", err)
	}
	return path, nil
}

// MessageEntry is a lightweight index entry returned by List.
type MessageEntry struct {
	Seq     int
	From    string
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
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		return nil, fmt.Errorf("read collab dir: %w", err)
	}
	var results []MessageEntry
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		agentName := e.Name()
		if excludeFrom != "" && agentName == excludeFrom {
			continue
		}
		agentDir := filepath.Join(s.Root, agentName)
		files, err := os.ReadDir(agentDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
				continue
			}
			path := filepath.Join(agentDir, f.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			msg, err := message.Unmarshal(data)
			if err != nil {
				continue
			}
			if since > 0 && msg.Seq <= since {
				continue
			}
			results = append(results, MessageEntry{
				Seq:     msg.Seq,
				From:    msg.From,
				Type:    msg.Type,
				Re:      msg.Re,
				Summary: msg.Summary,
				Status:  msg.Status,
				Path:    path,
			})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Seq < results[j].Seq
	})
	return results, nil
}

// ReadMessage reads and parses a message by seq number.
func (s *Store) ReadMessage(seq int) (*message.Message, error) {
	// Search all agent directories
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		return nil, fmt.Errorf("read collab dir: %w", err)
	}
	pattern := fmt.Sprintf("%03d-", seq)
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		agentDir := filepath.Join(s.Root, e.Name())
		files, err := os.ReadDir(agentDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if strings.HasPrefix(f.Name(), pattern) {
				data, err := os.ReadFile(filepath.Join(agentDir, f.Name()))
				if err != nil {
					return nil, err
				}
				return message.Unmarshal(data)
			}
		}
	}
	return nil, fmt.Errorf("message #%d not found", seq)
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
