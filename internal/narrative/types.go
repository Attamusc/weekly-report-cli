// Package narrative defines the data types shared between the pipeline and AI
// layers for the highlights narrative feature. Placing these types here avoids
// an import cycle between internal/pipeline (which imports internal/ai) and
// internal/ai (which needs to accept these types in its interface).
package narrative

import "time"

// Truncation caps applied at collection time for Item fields.
// These bound token budget for the narrative AI call.
const (
	MaxBodyChars    = 500 // max chars for issue/PR body
	MaxCommentChars = 300 // max chars per comment body
	MaxComments     = 5   // max recent comments per item
	MaxEvents       = 10  // max timeline events per item
)

// Item is the rich per-survivor data shape consumed by the narrative AI call.
// It is populated by the Tier-2 hydration phase and passed directly to
// WriteNarrative.
type Item struct {
	URL            string
	Title          string
	Body           string // truncated to MaxBodyChars at collection time
	State          string // "open" | "closed"
	IsPR           bool
	Author         string // issue/PR opener
	Assignees      []string
	Labels         []string
	OpenedAt       time.Time
	ClosedAt       *time.Time
	MergedAt       *time.Time // PRs only, nil if not merged
	ClosedThisWeek bool
	MergedThisWeek bool
	RecentComments []Comment // chronological, since lookback
	Events         []Event   // chronological, since lookback, filtered
}

// Comment is an attributed comment included in an Item.
type Comment struct {
	Author    string
	CreatedAt time.Time
	Body      string // truncated to MaxCommentChars at collection time
}

// Event is a filtered timeline event included in an Item.
type Event struct {
	Type   string // e.g. "closed", "merged", "reviewed", "review_requested"
	Actor  string // username who performed the event
	At     time.Time
	Detail string // event-specific text (close reason, review state, etc.)
}
