package message

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Type string

const (
	Inquiry  Type = "inquiry"
	Reply    Type = "reply"
	Proposal Type = "proposal"
	Review   Type = "review"
	Info     Type = "info"
)

func ValidType(s string) (Type, error) {
	switch Type(s) {
	case Inquiry, Reply, Proposal, Review, Info:
		return Type(s), nil
	default:
		return "", fmt.Errorf("invalid message type %q (valid: inquiry, reply, proposal, review, info)", s)
	}
}

type Status string

const (
	Open     Status = "open"
	Resolved Status = "resolved"
)

type Message struct {
	Seq     int    `yaml:"seq"`
	From    string `yaml:"from"`
	To      string `yaml:"to"`
	Type    Type   `yaml:"type"`
	Re      int    `yaml:"re,omitempty"`
	TS      string `yaml:"ts"`
	Summary string `yaml:"summary"`
	Status  Status `yaml:"status"`
	Body    string `yaml:"-"`
}

type frontmatter struct {
	Seq     int    `yaml:"seq"`
	From    string `yaml:"from"`
	To      string `yaml:"to"`
	Type    Type   `yaml:"type"`
	Re      int    `yaml:"re,omitempty"`
	TS      string `yaml:"ts"`
	Summary string `yaml:"summary"`
	Status  Status `yaml:"status"`
}

// Marshal writes a message as markdown with YAML frontmatter.
func (m *Message) Marshal() []byte {
	fm := frontmatter{
		Seq:     m.Seq,
		From:    m.From,
		To:      m.To,
		Type:    m.Type,
		Re:      m.Re,
		TS:      m.TS,
		Summary: m.Summary,
		Status:  m.Status,
	}

	frontmatterBytes, err := yaml.Marshal(&fm)
	if err != nil {
		panic(fmt.Sprintf("marshal message frontmatter: %v", err))
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.Write(frontmatterBytes)
	b.WriteString("---\n\n")
	b.WriteString(m.Body)
	if !strings.HasSuffix(m.Body, "\n") {
		b.WriteString("\n")
	}
	return []byte(b.String())
}

// Unmarshal parses markdown with YAML frontmatter into a Message.
func Unmarshal(data []byte) (*Message, error) {
	s := string(data)
	if !strings.HasPrefix(s, "---\n") {
		return nil, fmt.Errorf("missing frontmatter delimiter")
	}
	rest := s[4:]
	end := strings.Index(rest, "\n---\n")
	if end == -1 {
		return nil, fmt.Errorf("missing closing frontmatter delimiter")
	}
	frontmatterText := rest[:end]
	body := strings.TrimPrefix(rest[end+5:], "\n")

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(frontmatterText), &fm); err != nil {
		return nil, fmt.Errorf("parse frontmatter yaml: %w", err)
	}

	if fm.Seq <= 0 {
		return nil, fmt.Errorf("invalid or missing seq")
	}
	if strings.TrimSpace(fm.From) == "" {
		return nil, fmt.Errorf("missing from")
	}
	if strings.TrimSpace(fm.To) == "" {
		return nil, fmt.Errorf("missing to")
	}
	if strings.TrimSpace(fm.TS) == "" {
		return nil, fmt.Errorf("missing ts")
	}
	if strings.TrimSpace(fm.Summary) == "" {
		return nil, fmt.Errorf("missing summary")
	}
	if fm.Re < 0 {
		return nil, fmt.Errorf("invalid re: %d", fm.Re)
	}

	msgType, err := ValidType(string(fm.Type))
	if err != nil {
		return nil, err
	}

	switch fm.Status {
	case Open, Resolved:
		// valid
	default:
		return nil, fmt.Errorf("invalid status %q (valid: open, resolved)", fm.Status)
	}

	return &Message{
		Seq:     fm.Seq,
		From:    fm.From,
		To:      fm.To,
		Type:    msgType,
		Re:      fm.Re,
		TS:      fm.TS,
		Summary: fm.Summary,
		Status:  fm.Status,
		Body:    body,
	}, nil
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
