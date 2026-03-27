package store

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/david-krentzlin/collab/internal/message"
)

type indexRecord struct {
	Seq     int            `json:"seq"`
	From    string         `json:"from"`
	To      string         `json:"to"`
	Type    message.Type   `json:"type"`
	Re      int            `json:"re,omitempty"`
	TS      string         `json:"ts"`
	Summary string         `json:"summary"`
	Status  message.Status `json:"status"`
	Path    string         `json:"path"`
}

func indexRecordFromMessage(path string, msg *message.Message) indexRecord {
	return indexRecord{
		Seq:     msg.Seq,
		From:    msg.From,
		To:      msg.To,
		Type:    msg.Type,
		Re:      msg.Re,
		TS:      msg.TS,
		Summary: msg.Summary,
		Status:  msg.Status,
		Path:    path,
	}
}

func (r indexRecord) toEntry() MessageEntry {
	return MessageEntry{
		Seq:     r.Seq,
		From:    r.From,
		To:      r.To,
		Type:    r.Type,
		Re:      r.Re,
		Summary: r.Summary,
		Status:  r.Status,
		Path:    r.Path,
	}
}

func (s *Store) ensureIndexFile() error {
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return fmt.Errorf("create collab task dir: %w", err)
	}

	indexPath := filepath.Join(s.Root, IndexFile)
	f, err := os.OpenFile(indexPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return fmt.Errorf("create index file: %w", err)
	}
	return f.Close()
}

func (s *Store) withIndexLock(exclusive bool, fn func() error) error {
	lockPath := filepath.Join(s.Root, IndexLock)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open index lock file: %w", err)
	}

	lockType := syscall.LOCK_SH
	if exclusive {
		lockType = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(f.Fd()), lockType); err != nil {
		_ = f.Close()
		return fmt.Errorf("lock index file: %w", err)
	}

	err = fn()

	if unlockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); unlockErr != nil {
		if err == nil {
			err = fmt.Errorf("unlock index file: %w", unlockErr)
		}
	}
	if closeErr := f.Close(); closeErr != nil {
		if err == nil {
			err = fmt.Errorf("close index lock file: %w", closeErr)
		}
	}

	return err
}

func (s *Store) appendIndexRecord(msg *message.Message, path string) error {
	if err := s.ensureIndexFile(); err != nil {
		return err
	}

	record := indexRecordFromMessage(path, msg)
	return s.withIndexLock(true, func() error {
		indexPath := filepath.Join(s.Root, IndexFile)
		f, err := os.OpenFile(indexPath, os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return fmt.Errorf("open index file for append: %w", err)
		}
		defer f.Close()

		if err := json.NewEncoder(f).Encode(record); err != nil {
			return fmt.Errorf("append index record: %w", err)
		}

		return nil
	})
}

func (s *Store) readIndexRecords() ([]indexRecord, error) {
	if err := s.ensureIndexFile(); err != nil {
		return nil, err
	}

	records, err := s.readIndexRecordsLocked()
	if err == nil {
		return records, nil
	}

	if rebuildErr := s.rebuildIndexFromMessages(); rebuildErr != nil {
		return nil, fmt.Errorf("read index records: %v (rebuild failed: %w)", err, rebuildErr)
	}

	records, retryErr := s.readIndexRecordsLocked()
	if retryErr != nil {
		return nil, retryErr
	}
	return records, nil
}

func (s *Store) readIndexRecordsLocked() ([]indexRecord, error) {
	var records []indexRecord
	err := s.withIndexLock(false, func() error {
		var err error
		records, err = s.readIndexRecordsUnlocked()
		return err
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Store) readIndexRecordsUnlocked() ([]indexRecord, error) {
	indexPath := filepath.Join(s.Root, IndexFile)
	f, err := os.Open(indexPath)
	if err != nil {
		return nil, fmt.Errorf("open index file for read: %w", err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	bySeq := make(map[int]indexRecord)
	for {
		var record indexRecord
		if err := dec.Decode(&record); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode index record: %w", err)
		}
		if err := validateIndexRecord(record); err != nil {
			return nil, err
		}
		bySeq[record.Seq] = record
	}

	seqs := make([]int, 0, len(bySeq))
	for seq := range bySeq {
		seqs = append(seqs, seq)
	}
	sort.Ints(seqs)

	records := make([]indexRecord, 0, len(seqs))
	for _, seq := range seqs {
		records = append(records, bySeq[seq])
	}

	return records, nil
}

func validateIndexRecord(record indexRecord) error {
	if record.Seq <= 0 {
		return fmt.Errorf("invalid index record: missing or invalid seq")
	}
	if strings.TrimSpace(record.Path) == "" {
		return fmt.Errorf("invalid index record #%d: missing path", record.Seq)
	}
	if strings.TrimSpace(record.From) == "" {
		return fmt.Errorf("invalid index record #%d: missing from", record.Seq)
	}
	if strings.TrimSpace(record.To) == "" {
		return fmt.Errorf("invalid index record #%d: missing to", record.Seq)
	}
	if _, err := message.ValidType(string(record.Type)); err != nil {
		return fmt.Errorf("invalid index record #%d: %w", record.Seq, err)
	}
	switch record.Status {
	case message.Open, message.Resolved:
		// valid
	default:
		return fmt.Errorf("invalid index record #%d: status %q", record.Seq, record.Status)
	}
	return nil
}

func (s *Store) writeIndexRecordsUnlocked(records []indexRecord) error {
	indexPath := filepath.Join(s.Root, IndexFile)
	tmp, err := os.CreateTemp(s.Root, ".index-*.tmp")
	if err != nil {
		return fmt.Errorf("create index temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	enc := json.NewEncoder(tmp)
	for _, record := range records {
		if err := enc.Encode(record); err != nil {
			tmp.Close()
			return fmt.Errorf("encode index record: %w", err)
		}
	}

	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod index temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close index temp file: %w", err)
	}

	if err := os.Rename(tmpPath, indexPath); err != nil {
		return fmt.Errorf("rename index temp file: %w", err)
	}

	return nil
}

func (s *Store) SetStatus(seq int, status message.Status) error {
	switch status {
	case message.Open, message.Resolved:
		// valid
	default:
		return fmt.Errorf("invalid status %q", status)
	}

	if err := s.ensureIndexFile(); err != nil {
		return err
	}

	return s.withIndexLock(true, func() error {
		records, err := s.readIndexRecordsUnlocked()
		if err != nil {
			return err
		}

		updated := false
		for i := range records {
			if records[i].Seq == seq {
				records[i].Status = status
				updated = true
				break
			}
		}
		if !updated {
			return fmt.Errorf("message #%d not found", seq)
		}

		return s.writeIndexRecordsUnlocked(records)
	})
}

func (s *Store) entry(seq int) (MessageEntry, error) {
	records, err := s.readIndexRecords()
	if err != nil {
		return MessageEntry{}, err
	}

	for _, record := range records {
		if record.Seq == seq {
			return record.toEntry(), nil
		}
	}

	if err := s.rebuildIndexFromMessages(); err != nil {
		return MessageEntry{}, fmt.Errorf("message #%d not found in index and rebuild failed: %w", seq, err)
	}

	records, err = s.readIndexRecordsLocked()
	if err != nil {
		return MessageEntry{}, err
	}
	for _, record := range records {
		if record.Seq == seq {
			return record.toEntry(), nil
		}
	}

	return MessageEntry{}, fmt.Errorf("message #%d not found", seq)
}

func (s *Store) rebuildIndexFromMessages() error {
	if err := s.ensureIndexFile(); err != nil {
		return err
	}

	return s.withIndexLock(true, func() error {
		records, err := s.scanMessageFilesForIndex()
		if err != nil {
			return err
		}
		return s.writeIndexRecordsUnlocked(records)
	})
}

func (s *Store) scanMessageFilesForIndex() ([]indexRecord, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return []indexRecord{}, nil
		}
		return nil, fmt.Errorf("scan index source directories: %w", err)
	}

	records := make([]indexRecord, 0)
	seen := make(map[int]struct{})

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		agentDir := filepath.Join(s.Root, entry.Name())
		files, err := os.ReadDir(agentDir)
		if err != nil {
			return nil, fmt.Errorf("scan agent directory %q: %w", agentDir, err)
		}

		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".md") {
				continue
			}

			path := filepath.Join(agentDir, file.Name())
			msg, err := s.ReadMessageAtPath(path)
			if err != nil {
				return nil, fmt.Errorf("rebuild index: %w", err)
			}

			if _, exists := seen[msg.Seq]; exists {
				return nil, fmt.Errorf("rebuild index: duplicate seq #%d in message files", msg.Seq)
			}
			seen[msg.Seq] = struct{}{}

			record := indexRecordFromMessage(path, msg)
			if err := validateIndexRecord(record); err != nil {
				return nil, fmt.Errorf("rebuild index: %w", err)
			}
			records = append(records, record)
		}
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].Seq < records[j].Seq
	})

	return records, nil
}
