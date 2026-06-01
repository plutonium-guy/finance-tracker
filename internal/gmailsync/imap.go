package gmailsync

import (
	"context"
	"io"
	"net/textproto"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
)

// IMAPConfig configures the Gmail IMAP connection.
type IMAPConfig struct {
	Host     string // "imap.gmail.com:993"
	Username string // full email address
	Password string // Gmail app password (not the account password)
	Mailbox  string // "INBOX"
}

// IMAPFetcher fetches matching messages over IMAP (implements Fetcher).
type IMAPFetcher struct {
	cfg IMAPConfig
}

// NewIMAPFetcher builds a fetcher with Gmail defaults.
func NewIMAPFetcher(cfg IMAPConfig) *IMAPFetcher {
	if cfg.Host == "" {
		cfg.Host = "imap.gmail.com:993"
	}
	if cfg.Mailbox == "" {
		cfg.Mailbox = "INBOX"
	}
	return &IMAPFetcher{cfg: cfg}
}

// Fetch connects, searches the mailbox for messages from any of senders since
// the cutoff, and returns them reduced to Message values.
func (f *IMAPFetcher) Fetch(ctx context.Context, senders []string, since time.Time) ([]Message, error) {
	c, err := client.DialTLS(f.cfg.Host, nil)
	if err != nil {
		return nil, err
	}
	defer c.Logout()
	if err := c.Login(f.cfg.Username, f.cfg.Password); err != nil {
		return nil, err
	}
	if _, err := c.Select(f.cfg.Mailbox, true); err != nil { // read-only
		return nil, err
	}

	// Union of sequence numbers matching any sender.
	seen := map[uint32]bool{}
	set := new(imap.SeqSet)
	any := false
	for _, sender := range senders {
		sender = strings.TrimSpace(sender)
		if sender == "" {
			continue
		}
		crit := imap.NewSearchCriteria()
		crit.Since = since
		crit.Header = textproto.MIMEHeader{}
		crit.Header.Add("From", sender)
		ids, err := c.Search(crit)
		if err != nil {
			continue
		}
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				set.AddNum(id)
				any = true
			}
		}
	}
	if !any {
		return nil, nil
	}

	section := &imap.BodySectionName{Peek: true} // don't mark mail as read
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchInternalDate, section.FetchItem()}
	ch := make(chan *imap.Message, 20)
	done := make(chan error, 1)
	go func() { done <- c.Fetch(set, items, ch) }()

	var out []Message
	for msg := range ch {
		out = append(out, toMessage(msg, section))
	}
	if err := <-done; err != nil {
		return out, err
	}
	return out, nil
}

func toMessage(msg *imap.Message, section *imap.BodySectionName) Message {
	m := Message{}
	if env := msg.Envelope; env != nil {
		m.Subject = env.Subject
		m.ID = env.MessageId
		m.Date = env.Date
		if len(env.From) > 0 {
			m.From = env.From[0].MailboxName + "@" + env.From[0].HostName
		}
	}
	if m.Date.IsZero() {
		m.Date = msg.InternalDate
	}
	if body := msg.GetBody(section); body != nil {
		m.Text = bodyText(body)
	}
	return m
}

// bodyText extracts readable text from a message body, preferring text/plain
// and falling back to HTML (tags stripped).
func bodyText(r io.Reader) string {
	mr, err := mail.CreateReader(r)
	if err != nil {
		b, _ := io.ReadAll(r)
		return stripHTML(string(b))
	}
	var plain, html string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		if ih, ok := p.Header.(*mail.InlineHeader); ok {
			ct, _, _ := ih.ContentType()
			b, _ := io.ReadAll(p.Body)
			switch {
			case strings.HasPrefix(ct, "text/plain"):
				plain += string(b)
			case strings.HasPrefix(ct, "text/html"):
				html += string(b)
			}
		}
	}
	if strings.TrimSpace(plain) != "" {
		return strings.Join(strings.Fields(plain), " ")
	}
	return stripHTML(html)
}

// DefaultCardSenders are common Indian bank credit-card alert From addresses.
var DefaultCardSenders = []string{
	"alerts@hdfcbank.net",
	"alerts@hdfcbank.com",
	"credit_cards@icicibank.com",
	"alert@icicibank.com",
	"cc.alerts@axisbank.com",
	"alerts@axisbank.com",
	"onlinesbicard@sbicard.com",
	"Email.Statements@sbicard.com",
	"noreply@kotak.com",
	"creditcards@aubank.in",
}
