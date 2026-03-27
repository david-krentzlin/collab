package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/david-krentzlin/collab/internal/message"
)

func TestFindUsesDefaultTaskSubdirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	s := Find(root)

	want := filepath.Join(root, CollabDir, DefaultTask)
	if s.Root != want {
		t.Fatalf("store root = %q, want %q", s.Root, want)
	}
}

func TestFindUsesTaskFromEnv(t *testing.T) {
	t.Setenv(TaskEnv, "feature/cache-race")

	root := t.TempDir()
	s := Find(root)

	want := filepath.Join(root, CollabDir, "feature/cache-race")
	if s.Root != want {
		t.Fatalf("store root = %q, want %q", s.Root, want)
	}
}

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

func TestInitCreatesAgentsUnderTaskRoot(t *testing.T) {
	t.Setenv(TaskEnv, "feature/new-layout")

	base := t.TempDir()
	s := Find(base)
	if err := s.Init([]string{"agent-a", "agent-b"}); err != nil {
		t.Fatalf("init store: %v", err)
	}

	if _, err := os.Stat(filepath.Join(s.Root, "agent-a")); err != nil {
		t.Fatalf("stat task-scoped agent dir: %v", err)
	}

	legacyAgentDir := filepath.Join(base, CollabDir, "agent-a")
	if _, err := os.Stat(legacyAgentDir); !os.IsNotExist(err) {
		t.Fatalf("legacy agent dir should not exist at %q", legacyAgentDir)
	}
}

func TestEnsureSeqFileDoesNotBootstrapFromMessages(t *testing.T) {
	t.Parallel()

	s := &Store{Root: filepath.Join(t.TempDir(), CollabDir, DefaultTask), Task: DefaultTask}
	if err := os.MkdirAll(filepath.Join(s.Root, "agent-a"), 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}

	legacyLike := &message.Message{
		Seq:     42,
		From:    "agent-a",
		To:      "agent-b",
		Type:    message.Info,
		TS:      message.Now(),
		Summary: "existing",
		Status:  message.Open,
		Body:    "body",
	}
	if err := os.WriteFile(filepath.Join(s.Root, "agent-a", "042-info.md"), legacyLike.Marshal(), 0o644); err != nil {
		t.Fatalf("write message file: %v", err)
	}

	if err := s.ensureSeqFile(); err != nil {
		t.Fatalf("ensure seq file: %v", err)
	}

	seq, err := readSeq(filepath.Join(s.Root, SeqFile))
	if err != nil {
		t.Fatalf("read seq: %v", err)
	}
	if seq != 0 {
		t.Fatalf("seq bootstrap = %d, want 0", seq)
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
