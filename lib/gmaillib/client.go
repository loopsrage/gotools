package gmaillib

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	gmail "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// MessageHeader represents a single RFC 2822 header on a Gmail Message.
type MessageHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// MessagePartBody represents the body payload of a MIME part.
type MessagePartBody struct {
	Size int64  `json:"size,omitempty"`
	Data string `json:"data,omitempty"`
}

// MessagePart represents a MIME part in a Gmail Message payload.
type MessagePart struct {
	MimeType string           `json:"mimeType,omitempty"`
	Headers  []MessageHeader  `json:"headers,omitempty"`
	Body     *MessagePartBody `json:"body,omitempty"`
	Parts    []MessagePart    `json:"parts,omitempty"`
}

// Message represents a Gmail v1 API message resource.
type Message struct {
	ID       string       `json:"id"`
	ThreadID string       `json:"threadId,omitempty"`
	LabelIDs []string     `json:"labelIds,omitempty"`
	Snippet  string       `json:"snippet,omitempty"`
	Payload  *MessagePart `json:"payload,omitempty"`
	Raw      string       `json:"raw,omitempty"`
}

// Label represents a Gmail v1 API label resource.
type Label struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	LabelListVisibility   string `json:"labelListVisibility,omitempty"`
	MessageListVisibility string `json:"messageListVisibility,omitempty"`
	Type                  string `json:"type,omitempty"`
}

// GmailService defines the Gmail API operations consumed by GmailSorter and callers.
type GmailService interface {
	ListMessages(ctx context.Context, query string, maxResults *int) ([]Message, error)
	ListAllMessages(ctx context.Context, query string) ([]Message, error)
	MessageIDs(ctx context.Context, query string, maxResults *int) ([]string, error)
	AllMessageIDs(ctx context.Context, query string) ([]string, error)
	GetMessage(ctx context.Context, messageID, format string) (*Message, error)
	ModifyLabels(ctx context.Context, messageID string, addLabelIDs, removeLabelIDs []string) (*Message, error)
	BatchModify(ctx context.Context, messageIDs, addLabelIDs, removeLabelIDs []string, chunkSize int) error
	Archive(ctx context.Context, messageID string) (*Message, error)
	Trash(ctx context.Context, messageID string) (*Message, error)
	MarkRead(ctx context.Context, messageID string) (*Message, error)
	MarkUnread(ctx context.Context, messageID string) (*Message, error)
	ListLabels(ctx context.Context) ([]Label, error)
	LabelID(ctx context.Context, name string) (string, bool, error)
	CreateLabel(ctx context.Context, name, labelListVisibility, messageListVisibility string) (string, error)
}

// IntPtr returns a pointer to n for optional maxResults arguments.
func IntPtr(n int) *int {
	return &n
}

// GmailClient wraps the official google.golang.org/api/gmail/v1.Service with OAuth2 token persistence and rate limiting.
type GmailClient struct {
	Config     GmailConfig
	httpClient *http.Client
	tokens     *tokenManager

	mu  sync.Mutex
	svc *gmail.Service
}

// NewGmailClient constructs a GmailClient with normalized defaults when cfg is nil.
func NewGmailClient(cfg *GmailConfig) *GmailClient {
	norm := normalizeGmailConfig(cfg)
	hc := norm.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &GmailClient{
		Config:     norm,
		httpClient: hc,
		tokens:     newTokenManager(norm, hc),
	}
}

// Service initializes (once) and returns the underlying official *gmail.Service.
func (c *GmailClient) Service(ctx context.Context) (*gmail.Service, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.svc != nil {
		return c.svc, nil
	}

	if _, err := c.tokens.AccessToken(ctx, c.httpClient); err != nil {
		return nil, err
	}

	authedTransport := &oauth2.Transport{
		Source: c.tokens,
		Base:   c.httpClient.Transport,
	}
	authedClient := &http.Client{
		Transport: authedTransport,
		Timeout:   c.httpClient.Timeout,
	}

	opts := []option.ClientOption{
		option.WithHTTPClient(authedClient),
	}
	if c.Config.BaseURL != "" && c.Config.BaseURL != defaultGmailBaseURL {
		baseRoot := strings.TrimSuffix(strings.TrimRight(c.Config.BaseURL, "/"), "/gmail/v1")
		opts = append(opts, option.WithEndpoint(baseRoot+"/"))
	}

	svc, err := gmail.NewService(ctx, opts...)
	if err != nil {
		return nil, err
	}
	c.svc = svc
	return c.svc, nil
}

func (c *GmailClient) rateLimit(ctx context.Context) error {
	if c.Config.RateLimitDelay <= 0 {
		return nil
	}
	timer := time.NewTimer(c.Config.RateLimitDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func convertSDKPart(p *gmail.MessagePart) *MessagePart {
	if p == nil {
		return nil
	}
	headers := make([]MessageHeader, 0, len(p.Headers))
	for _, h := range p.Headers {
		if h != nil {
			headers = append(headers, MessageHeader{Name: h.Name, Value: h.Value})
		}
	}
	var body *MessagePartBody
	if p.Body != nil {
		body = &MessagePartBody{Size: p.Body.Size, Data: p.Body.Data}
	}
	parts := make([]MessagePart, 0, len(p.Parts))
	for _, child := range p.Parts {
		if cp := convertSDKPart(child); cp != nil {
			parts = append(parts, *cp)
		}
	}
	return &MessagePart{
		MimeType: p.MimeType,
		Headers:  headers,
		Body:     body,
		Parts:    parts,
	}
}

func convertSDKMessage(m *gmail.Message) Message {
	if m == nil {
		return Message{}
	}
	return Message{
		ID:       m.Id,
		ThreadID: m.ThreadId,
		LabelIDs: append([]string(nil), m.LabelIds...),
		Snippet:  m.Snippet,
		Payload:  convertSDKPart(m.Payload),
		Raw:      m.Raw,
	}
}

func convertSDKLabel(l *gmail.Label) Label {
	if l == nil {
		return Label{}
	}
	return Label{
		ID:                    l.Id,
		Name:                  l.Name,
		LabelListVisibility:   l.LabelListVisibility,
		MessageListVisibility: l.MessageListVisibility,
		Type:                  l.Type,
	}
}

// ListMessages queries messages matching query via gmail.Service, paginating until maxResults (or all pages when nil).
func (c *GmailClient) ListMessages(ctx context.Context, query string, maxResults *int) ([]Message, error) {
	if maxResults != nil && *maxResults <= 0 {
		return []Message{}, nil
	}
	svc, err := c.Service(ctx)
	if err != nil {
		return nil, err
	}

	var messages []Message
	pageToken := ""

	for {
		pageSize := int64(500)
		if maxResults != nil {
			rem := int64(*maxResults - len(messages))
			if rem > 500 {
				pageSize = rem
			}
		}

		call := svc.Users.Messages.List(c.Config.UserID).MaxResults(pageSize).Context(ctx)
		if query != "" {
			call = call.Q(query)
		}
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, err
		}
		if err := c.rateLimit(ctx); err != nil {
			return nil, err
		}

		for _, m := range resp.Messages {
			messages = append(messages, convertSDKMessage(m))
		}
		if resp.NextPageToken == "" || (maxResults != nil && len(messages) >= *maxResults) {
			break
		}
		pageToken = resp.NextPageToken
	}

	if maxResults != nil && len(messages) > *maxResults {
		messages = messages[:*maxResults]
	}
	if messages == nil {
		messages = []Message{}
	}
	return messages, nil
}

// ListAllMessages paginates through every message matching query across inbox history.
func (c *GmailClient) ListAllMessages(ctx context.Context, query string) ([]Message, error) {
	return c.ListMessages(ctx, query, nil)
}

// MessageIDs returns message IDs matching query up to maxResults.
func (c *GmailClient) MessageIDs(ctx context.Context, query string, maxResults *int) ([]string, error) {
	msgs, err := c.ListMessages(ctx, query, maxResults)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	return ids, nil
}

// AllMessageIDs returns every message ID matching query across all pages.
func (c *GmailClient) AllMessageIDs(ctx context.Context, query string) ([]string, error) {
	return c.MessageIDs(ctx, query, nil)
}

// GetMessage fetches a single message by ID using format (defaults to "metadata" when empty).
func (c *GmailClient) GetMessage(ctx context.Context, messageID, format string) (*Message, error) {
	svc, err := c.Service(ctx)
	if err != nil {
		return nil, err
	}
	if format == "" {
		format = "metadata"
	}
	resp, err := svc.Users.Messages.Get(c.Config.UserID, messageID).Format(format).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	if err := c.rateLimit(ctx); err != nil {
		return nil, err
	}
	converted := convertSDKMessage(resp)
	return &converted, nil
}

// ModifyLabels adds and/or removes label IDs on a single message via Users.Messages.Modify.
func (c *GmailClient) ModifyLabels(ctx context.Context, messageID string, addLabelIDs, removeLabelIDs []string) (*Message, error) {
	svc, err := c.Service(ctx)
	if err != nil {
		return nil, err
	}
	req := &gmail.ModifyMessageRequest{
		AddLabelIds:    addLabelIDs,
		RemoveLabelIds: removeLabelIDs,
	}
	resp, err := svc.Users.Messages.Modify(c.Config.UserID, messageID, req).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	if err := c.rateLimit(ctx); err != nil {
		return nil, err
	}
	converted := convertSDKMessage(resp)
	return &converted, nil
}

// BatchModify modifies labels across messageIDs in chunks (defaulting chunkSize to 500).
func (c *GmailClient) BatchModify(ctx context.Context, messageIDs, addLabelIDs, removeLabelIDs []string, chunkSize int) error {
	if chunkSize <= 0 {
		chunkSize = 500
	}
	svc, err := c.Service(ctx)
	if err != nil {
		return err
	}
	for start := 0; start < len(messageIDs); start += chunkSize {
		end := start + chunkSize
		if end > len(messageIDs) {
			end = len(messageIDs)
		}
		req := &gmail.BatchModifyMessagesRequest{
			Ids:            messageIDs[start:end],
			AddLabelIds:    addLabelIDs,
			RemoveLabelIds: removeLabelIDs,
		}
		if err := svc.Users.Messages.BatchModify(c.Config.UserID, req).Context(ctx).Do(); err != nil {
			return err
		}
		if err := c.rateLimit(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Archive removes the INBOX label from messageID.
func (c *GmailClient) Archive(ctx context.Context, messageID string) (*Message, error) {
	return c.ModifyLabels(ctx, messageID, nil, []string{"INBOX"})
}

// Trash moves messageID to the trash via Users.Messages.Trash.
func (c *GmailClient) Trash(ctx context.Context, messageID string) (*Message, error) {
	svc, err := c.Service(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := svc.Users.Messages.Trash(c.Config.UserID, messageID).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	if err := c.rateLimit(ctx); err != nil {
		return nil, err
	}
	converted := convertSDKMessage(resp)
	return &converted, nil
}

// MarkRead removes the UNREAD label from messageID.
func (c *GmailClient) MarkRead(ctx context.Context, messageID string) (*Message, error) {
	return c.ModifyLabels(ctx, messageID, nil, []string{"UNREAD"})
}

// MarkUnread adds the UNREAD label to messageID.
func (c *GmailClient) MarkUnread(ctx context.Context, messageID string) (*Message, error) {
	return c.ModifyLabels(ctx, messageID, []string{"UNREAD"}, nil)
}

// ListLabels returns all labels configured on the Gmail account via Users.Labels.List.
func (c *GmailClient) ListLabels(ctx context.Context) ([]Label, error) {
	svc, err := c.Service(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := svc.Users.Labels.List(c.Config.UserID).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	if err := c.rateLimit(ctx); err != nil {
		return nil, err
	}
	out := make([]Label, 0, len(resp.Labels))
	for _, l := range resp.Labels {
		out = append(out, convertSDKLabel(l))
	}
	return out, nil
}

// LabelID resolves a label's human-readable name to its Gmail label ID.
func (c *GmailClient) LabelID(ctx context.Context, name string) (string, bool, error) {
	labels, err := c.ListLabels(ctx)
	if err != nil {
		return "", false, err
	}
	for _, l := range labels {
		if l.Name == name {
			return l.ID, true, nil
		}
	}
	return "", false, nil
}

// CreateLabel creates a Gmail label if it does not already exist and returns its label ID.
func (c *GmailClient) CreateLabel(ctx context.Context, name, labelListVisibility, messageListVisibility string) (string, error) {
	if existingID, found, err := c.LabelID(ctx, name); err != nil {
		return "", err
	} else if found && existingID != "" {
		return existingID, nil
	}

	svc, err := c.Service(ctx)
	if err != nil {
		return "", err
	}
	if labelListVisibility == "" {
		labelListVisibility = "labelShow"
	}
	if messageListVisibility == "" {
		messageListVisibility = "show"
	}

	created, err := svc.Users.Labels.Create(c.Config.UserID, &gmail.Label{
		Name:                  name,
		LabelListVisibility:   labelListVisibility,
		MessageListVisibility: messageListVisibility,
	}).Context(ctx).Do()
	if err != nil {
		return "", err
	}
	if err := c.rateLimit(ctx); err != nil {
		return "", err
	}
	if created != nil && created.Id != "" {
		return created.Id, nil
	}
	return name, nil
}
