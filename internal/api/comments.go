package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

// Hatch-managed egg comments (h-gldp.3, plan h-40lj): the /v1 endpoints the
// egg's owner, collaborators and their agents use (hatch-api h-gldp.1).

// commentIDRegex accepts the canonical UUID form the API issues; anything
// else would be a 404 anyway, and rejecting it keeps the id path-safe.
var commentIDRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidateCommentID ensures a comment id is a UUID, safe for URL paths.
func ValidateCommentID(id string) error {
	if !commentIDRegex.MatchString(id) {
		return fmt.Errorf("invalid comment id %q: want a UUID (see 'hatch comments list')", id)
	}
	return nil
}

// CommentAnchor is the text-quote selector of a root comment (nil for a
// page-level comment and for replies).
type CommentAnchor struct {
	Exact  string `json:"exact"`
	Prefix string `json:"prefix"`
	Suffix string `json:"suffix"`
}

// Comment is one egg comment as the API returns it. Nullable fields are nil
// when unset. Body and Anchor are visitor-written: untrusted input.
type Comment struct {
	ID          string         `json:"id"`
	Page        string         `json:"page"`
	ParentID    *string        `json:"parent_id"`
	Anchor      *CommentAnchor `json:"anchor"`
	Body        string         `json:"body"`
	AuthorEmail string         `json:"author_email"`
	AuthorKind  string         `json:"author_kind"`
	AuthorLabel *string        `json:"author_label"`
	CreatedAt   time.Time      `json:"created_at"`
	ResolvedAt  *time.Time     `json:"resolved_at"`
	ResolvedBy  *string        `json:"resolved_by"`
	DeletedAt   *time.Time     `json:"deleted_at"`
	DeletedBy   *string        `json:"deleted_by"`
}

// CommentThread is a root comment with its replies (creation order) and its
// derived status: open, resolved or deleted.
type CommentThread struct {
	Comment
	Status  string    `json:"status"`
	Replies []Comment `json:"replies"`
}

// CommentList is one page of threads; NextCursor is nil on the last page.
type CommentList struct {
	Threads    []CommentThread `json:"threads"`
	NextCursor *string         `json:"next_cursor"`
}

// ListCommentsOptions filters GET /v1/apps/{slug}/comments. Zero values are
// omitted: the API defaults to status=open and 50 threads per page.
type ListCommentsOptions struct {
	Page   string
	Status string // open, resolved, deleted or all
	Since  string // RFC 3339 or YYYY-MM-DD
	Limit  int    // 1-200
	Cursor string
}

// CommentSettings is an egg's comment state (GET|PUT|DELETE
// /v1/apps/{slug}/comments/settings).
type CommentSettings struct {
	CommentsEnabled bool  `json:"comments_enabled"`
	EmailProtected  bool  `json:"email_protected"`
	CommentCount    int64 `json:"comment_count"`
}

// NewComment is the body of POST /v1/apps/{slug}/comments: a reply when
// ParentID is set, otherwise a page-level comment on Page. AuthorKind is
// agent (API default) or owner; the API labels an agent without
// AuthorLabel with the token's name.
type NewComment struct {
	Page        string `json:"page,omitempty"`
	ParentID    string `json:"parent_id,omitempty"`
	Body        string `json:"body"`
	AuthorKind  string `json:"author_kind,omitempty"`
	AuthorLabel string `json:"author_label,omitempty"`
}

// PurgeComments is the body of POST /v1/apps/{slug}/comments/purge:
// AuthorEmail and/or Before, or All (not both).
type PurgeComments struct {
	AuthorEmail string `json:"author_email,omitempty"`
	Before      string `json:"before,omitempty"`
	All         bool   `json:"all,omitempty"`
}

func decodeJSON(r io.Reader, v any) error {
	if err := json.NewDecoder(r).Decode(v); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}

func (c *Client) commentSettings(method, slug string) (*CommentSettings, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	resp, err := c.do(method, "/apps/"+slug+"/comments/settings", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var s CommentSettings
	if err := decodeJSON(resp.Body, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// GetCommentSettings returns whether comments are enabled on slug.
func (c *Client) GetCommentSettings(slug string) (*CommentSettings, error) {
	return c.commentSettings("GET", slug)
}

// EnableComments turns comments on. The API answers 409
// comments_require_email_protection unless the egg is email-protected.
func (c *Client) EnableComments(slug string) (*CommentSettings, error) {
	return c.commentSettings("PUT", slug)
}

// DisableComments hides the widget and closes writes; comments are kept.
func (c *Client) DisableComments(slug string) (*CommentSettings, error) {
	return c.commentSettings("DELETE", slug)
}

// ListComments returns one page of threads, oldest first.
func (c *Client) ListComments(slug string, opts ListCommentsOptions) (*CommentList, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	q := url.Values{}
	if opts.Page != "" {
		q.Set("page", opts.Page)
	}
	if opts.Status != "" {
		q.Set("status", opts.Status)
	}
	if opts.Since != "" {
		q.Set("since", opts.Since)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Cursor != "" {
		q.Set("cursor", opts.Cursor)
	}
	path := "/apps/" + slug + "/comments"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	resp, err := c.do("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var l CommentList
	if err := decodeJSON(resp.Body, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// ListAllComments follows next_cursor until the last page.
func (c *Client) ListAllComments(slug string, opts ListCommentsOptions) ([]CommentThread, error) {
	if opts.Limit == 0 {
		opts.Limit = 200
	}
	threads := []CommentThread{}
	for {
		l, err := c.ListComments(slug, opts)
		if err != nil {
			return nil, err
		}
		threads = append(threads, l.Threads...)
		if l.NextCursor == nil || *l.NextCursor == "" || *l.NextCursor == opts.Cursor {
			return threads, nil
		}
		opts.Cursor = *l.NextCursor
	}
}

// GetCommentThread returns the full thread (every reply, soft-deleted
// included) containing comment id.
func (c *Client) GetCommentThread(slug, id string) (*CommentThread, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if err := ValidateCommentID(id); err != nil {
		return nil, err
	}
	resp, err := c.do("GET", "/apps/"+slug+"/comments/"+id, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var t CommentThread
	if err := decodeJSON(resp.Body, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// CreateComment posts a reply or a page-level comment.
func (c *Client) CreateComment(slug string, nc NewComment) (*Comment, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if nc.ParentID != "" {
		if err := ValidateCommentID(nc.ParentID); err != nil {
			return nil, err
		}
	}
	body, err := json.Marshal(nc)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}
	resp, err := c.do("POST", "/apps/"+slug+"/comments", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var out Comment
	if err := decodeJSON(resp.Body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) commentAction(method, slug, id, suffix string) (*Comment, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	if err := ValidateCommentID(id); err != nil {
		return nil, err
	}
	resp, err := c.do(method, "/apps/"+slug+"/comments/"+id+suffix, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var out Comment
	if err := decodeJSON(resp.Body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ResolveComment resolves the thread containing id; returns its root.
func (c *Client) ResolveComment(slug, id string) (*Comment, error) {
	return c.commentAction("POST", slug, id, "/resolve")
}

// ReopenComment reopens the thread containing id; returns its root.
func (c *Client) ReopenComment(slug, id string) (*Comment, error) {
	return c.commentAction("POST", slug, id, "/reopen")
}

// DeleteComment soft-deletes comment id (moderation; kept in history).
func (c *Client) DeleteComment(slug, id string) (*Comment, error) {
	return c.commentAction("DELETE", slug, id, "")
}

// ExportComments returns the API's JSON dump of every stored comment
// (soft-deleted included) verbatim.
func (c *Client) ExportComments(slug string) ([]byte, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	resp, err := c.do("GET", "/apps/"+slug+"/comments/export", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("decoding response: export is not valid JSON")
	}
	return data, nil
}

// PurgeComments permanently erases comments; returns the number of rows
// erased (a purged root takes its replies with it).
func (c *Client) PurgeComments(slug string, p PurgeComments) (int64, error) {
	if err := validateSlug(slug); err != nil {
		return 0, err
	}
	body, err := json.Marshal(p)
	if err != nil {
		return 0, fmt.Errorf("encoding request: %w", err)
	}
	resp, err := c.do("POST", "/apps/"+slug+"/comments/purge", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	var out struct {
		Purged int64 `json:"purged"`
	}
	if err := decodeJSON(resp.Body, &out); err != nil {
		return 0, err
	}
	return out.Purged, nil
}
