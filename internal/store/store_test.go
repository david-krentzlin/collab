package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/david-krentzlin/collab/internal/message"
)

func TestInitCreatesSequenceFile(t *testing.T) {
	t.Parallel()

	s := &Store{Root: filepath.Join(t.TempDir(), CollabDir)}
	if err := s.Init([]string{"agent-a", "agent-b"}); err != nil {
		t.Fatalf("init store: %v", err)
	}

	seqPath := filepath.Join(s.Root, ".seq")
	if _, err := os.Stat(seqPath); err != nil {
		t.Fatalf("stat sequence file %q: %v", seqPath, err)
	}
}

func TestCreateMessageAllocatesUniqueSeqsConcurrently(t *testing.T) {
	t.Parallel()

	s := &Store{Root: filepath.Join(t.TempDir(), CollabDir)}
	if err := s.Init([]string{"agent-a", "agent-b"}); err != nil {
		t.Fatalf("init store: %v", err)
	}

	const sends = 32

	seqs := make(chan int, sends)
	errCh := make(chan error, sends)
	var wg sync.WaitGroup

	for i := 0; i < sends; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			msg := &message.Message{
				From:    "agent-a",
				To:      "agent-b",
				Type:    message.Info,
				Re:      0,
				TS:      message.Now(),
				Summary: fmt.Sprintf("message %d", i),
				Status:  message.Open,
				Body:    fmt.Sprintf("body %d", i),
			}

			path, err := s.CreateMessage(msg)
			if err != nil {
				errCh <- err
				return
			}
			if path == "" {
				errCh <- fmt.Errorf("empty path for seq %d", msg.Seq)
				return
			}

			seqs <- msg.Seq
		}(i)
	}

	wg.Wait()
	close(errCh)
	close(seqs)

	for err := range errCh {
		if err != nil {
			t.Fatalf("create message concurrently: %v", err)
		}
	}

	seen := map[int]bool{}
	for seq := range seqs {
		if seen[seq] {
			t.Fatalf("duplicate sequence allocated: %d", seq)
		}
		seen[seq] = true
	}

	if len(seen) != sends {
		t.Fatalf("got %d unique sequences, want %d", len(seen), sends)
	}

	for seq := 1; seq <= sends; seq++ {
		if !seen[seq] {
			t.Fatalf("missing sequence %d from concurrent allocation", seq)
		}
	}
}

func TestInitPreservesSequenceProgress(t *testing.T) {
	t.Parallel()

	s := &Store{Root: filepath.Join(t.TempDir(), CollabDir)}
	if err := s.Init([]string{"agent-a", "agent-b"}); err != nil {
		t.Fatalf("init store: %v", err)
	}

	first := &message.Message{
		From:    "agent-a",
		To:      "agent-b",
		Type:    message.Inquiry,
		TS:      message.Now(),
		Summary: "first",
		Status:  message.Open,
		Body:    "body",
	}
	if _, err := s.CreateMessage(first); err != nil {
		t.Fatalf("create first message: %v", err)
	}

	if err := s.Init([]string{"agent-a", "agent-b"}); err != nil {
		t.Fatalf("re-init store: %v", err)
	}

	second := &message.Message{
		From:    "agent-a",
		To:      "agent-b",
		Type:    message.Info,
		TS:      message.Now(),
		Summary: "second",
		Status:  message.Open,
		Body:    "body",
	}
	if _, err := s.CreateMessage(second); err != nil {
		t.Fatalf("create second message: %v", err)
	}

	if second.Seq != first.Seq+1 {
		t.Fatalf("second seq = %d, want %d", second.Seq, first.Seq+1)
	}
}
