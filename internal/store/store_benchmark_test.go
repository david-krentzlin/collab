package store

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/david-krentzlin/collab/internal/message"
)

func BenchmarkListForRecipient(b *testing.B) {
	s := &Store{Root: filepath.Join(b.TempDir(), CollabDir, DefaultTask), Task: DefaultTask}
	if err := s.Init([]string{"agent-a", "agent-b", "agent-c"}); err != nil {
		b.Fatalf("init store: %v", err)
	}

	const totalMessages = 5000
	for i := 0; i < totalMessages; i++ {
		to := "agent-b"
		if i%3 == 1 {
			to = "agent-c"
		} else if i%3 == 2 {
			to = BroadcastTo
		}
		from := "agent-a"
		if i%2 == 1 {
			from = "agent-c"
		}

		msg := &message.Message{
			From:    from,
			To:      to,
			Type:    message.Info,
			TS:      message.Now(),
			Summary: fmt.Sprintf("summary-%d", i),
			Status:  message.Open,
			Body:    "body",
		}
		if _, err := s.CreateMessage(msg); err != nil {
			b.Fatalf("create message %d: %v", i, err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		entries, err := s.ListForRecipient(totalMessages-200, "agent-b")
		if err != nil {
			b.Fatalf("list for recipient: %v", err)
		}
		if len(entries) == 0 {
			b.Fatalf("expected benchmark entries")
		}
	}
}
