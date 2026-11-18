package gmaillib

import (
	"context"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	angleAddrRe = regexp.MustCompile(`<([^>]+)>`)
	nonWordRe   = regexp.MustCompile(`[^\w\s]`)
)

// MatchedMessage represents an inbox message matched against a rule or recurring sender category.
type MatchedMessage struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	From    string `json:"from"`
	Score   int    `json:"score"`
}

// ProgressFunc reports scan progress (1-based current index, total count, subject/sender label).
type ProgressFunc func(current, total int, item string)

// MessageHeaders extracts payload headers into a name->value map.
func MessageHeaders(msg *Message) map[string]string {
	out := make(map[string]string)
	if msg == nil || msg.Payload == nil {
		return out
	}
	for _, h := range msg.Payload.Headers {
		out[h.Name] = h.Value
	}
	return out
}

func headerLookup(headers map[string]string, name string) string {
	if v, ok := headers[name]; ok {
		return v
	}
	for k, v := range headers {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

// SubjectWords normalizes subject to lowercase alphanumeric tokens longer than 2 characters.
func SubjectWords(subject string) []string {
	clean := nonWordRe.ReplaceAllString(strings.ToLower(subject), " ")
	var out []string
	for _, w := range strings.Fields(clean) {
		if len(w) > 2 {
			out = append(out, w)
		}
	}
	return out
}

// SenderDomain extracts the lowercase email domain from a From header string.
func SenderDomain(sender string) string {
	addr := strings.TrimSpace(sender)
	if m := angleAddrRe.FindStringSubmatch(sender); len(m) > 1 {
		addr = m[1]
	}
	parts := strings.Split(addr, "@")
	if len(parts) > 1 {
		return strings.ToLower(parts[len(parts)-1])
	}
	return strings.ToLower(addr)
}

// SenderWords splits the sender's email domain into lowercase tokens longer than 2 characters.
func SenderWords(sender string) []string {
	domain := SenderDomain(sender)
	clean := nonWordRe.ReplaceAllString(domain, " ")
	var out []string
	for _, w := range strings.Fields(clean) {
		if len(w) > 2 {
			out = append(out, w)
		}
	}
	return out
}

// pyTitle replicates Python str.title(): uppercase the first letter following any non-letter rune.
func pyTitle(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevLetter := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			if !prevLetter {
				b.WriteRune(unicode.ToTitle(r))
			} else {
				b.WriteRune(unicode.ToLower(r))
			}
			prevLetter = true
		} else {
			b.WriteRune(r)
			prevLetter = false
		}
	}
	return b.String()
}

// GmailSorter organizes Gmail inbox messages by query rules, domains, fuzzy previews, and auto-categories.
type GmailSorter struct {
	Client GmailService
}

// NewGmailSorter creates a GmailSorter wrapping client.
func NewGmailSorter(client GmailService) *GmailSorter {
	return &GmailSorter{Client: client}
}

// SortInbox applies label->query rules across the inbox and optionally archives matched messages.
func (s *GmailSorter) SortInbox(ctx context.Context, rules map[string]string, archive bool, maxResults *int) (map[string]int, error) {
	counts := make(map[string]int, len(rules))
	labelNames := make([]string, 0, len(rules))
	for k := range rules {
		labelNames = append(labelNames, k)
	}
	sort.Strings(labelNames)

	for _, labelName := range labelNames {
		query := rules[labelName]
		labelID, found, err := s.Client.LabelID(ctx, labelName)
		if err != nil {
			return nil, err
		}
		if !found || labelID == "" {
			labelID, err = s.Client.CreateLabel(ctx, labelName, "labelShow", "show")
			if err != nil {
				return nil, err
			}
		}

		msgIDs, err := s.Client.MessageIDs(ctx, query, maxResults)
		if err != nil {
			return nil, err
		}
		if len(msgIDs) > 0 {
			var removeIDs []string
			if archive {
				removeIDs = []string{"INBOX"}
			}
			if err := s.Client.BatchModify(ctx, msgIDs, []string{labelID}, removeIDs, 500); err != nil {
				return nil, err
			}
		}
		counts[labelName] = len(msgIDs)
	}
	return counts, nil
}

// SortByDomain moves messages from domain into labelName and returns the matched message count.
func (s *GmailSorter) SortByDomain(ctx context.Context, domain, labelName string, archive bool, maxResults *int) (int, error) {
	counts, err := s.SortInbox(ctx, map[string]string{labelName: "from:" + domain}, archive, maxResults)
	if err != nil {
		return 0, err
	}
	return counts[labelName], nil
}

type scannedMessage struct {
	id      string
	subject string
	from    string
	words   map[string]struct{}
}

// PreviewSort performs a read-only dry-run matching inbox Subject + From headers against keyword rules.
func (s *GmailSorter) PreviewSort(ctx context.Context, rules map[string]string, threshold int, maxResults *int, onProgress ProgressFunc) (map[string][]MatchedMessage, error) {
	candidateIDs, err := s.Client.MessageIDs(ctx, "in:inbox", maxResults)
	if err != nil {
		return nil, err
	}
	total := len(candidateIDs)
	messages := make([]scannedMessage, 0, total)

	for i, msgID := range candidateIDs {
		msg, err := s.Client.GetMessage(ctx, msgID, "metadata")
		if err != nil {
			return nil, err
		}
		headers := MessageHeaders(msg)
		subj := headerLookup(headers, "Subject")
		sender := headerLookup(headers, "From")
		messages = append(messages, scannedMessage{
			id:      msgID,
			subject: subj,
			from:    sender,
		})
		if onProgress != nil {
			onProgress(i+1, total, subj)
		}
	}

	results := make(map[string][]MatchedMessage, len(rules))
	for labelName, keywords := range rules {
		keywordTerms := strings.Fields(strings.ToLower(keywords))
		matches := make([]MatchedMessage, 0)
		if len(keywordTerms) > 0 {
			for _, m := range messages {
				haystack := strings.ToLower(m.subject + " " + m.from)
				haystackWords := make(map[string]struct{})
				for _, w := range strings.Fields(haystack) {
					haystackWords[w] = struct{}{}
				}
				matched := 0
				for _, t := range keywordTerms {
					if _, ok := haystackWords[t]; ok {
						matched++
					}
				}
				score := int(math.Round(float64(matched) / float64(len(keywordTerms)) * 100.0))
				if score >= threshold {
					matches = append(matches, MatchedMessage{
						ID:      m.id,
						Subject: m.subject,
						From:    m.from,
						Score:   score,
					})
				}
			}
		}
		sort.SliceStable(matches, func(i, j int) bool {
			return matches[i].Score > matches[j].Score
		})
		results[labelName] = matches
	}
	return results, nil
}

// AutoCategorizeInbox groups recurring sender domains across inbox messages, creates category labels,
// archives matched groups of at least minGroupSize, and returns the applied matches per category.
func (s *GmailSorter) AutoCategorizeInbox(ctx context.Context, threshold, minGroupSize int, maxResults *int, pageSize int, onProgress ProgressFunc) (map[string][]MatchedMessage, error) {
	if threshold <= 0 {
		threshold = 50
	}
	if minGroupSize <= 0 {
		minGroupSize = 2
	}

	var (
		candidates []string
		err        error
	)
	if maxResults == nil {
		candidates, err = s.Client.AllMessageIDs(ctx, "in:inbox")
	} else {
		candidates, err = s.Client.MessageIDs(ctx, "in:inbox", maxResults)
	}
	if err != nil {
		return nil, err
	}
	total := len(candidates)

	if pageSize > 0 {
		return s.autoCategorizePaged(ctx, candidates, total, threshold, minGroupSize, pageSize, onProgress)
	}

	messages := make([]scannedMessage, 0, total)
	for i, msgID := range candidates {
		msg, err := s.Client.GetMessage(ctx, msgID, "metadata")
		if err != nil {
			return nil, err
		}
		headers := MessageHeaders(msg)
		subj := headerLookup(headers, "Subject")
		sender := headerLookup(headers, "From")
		wordSet := make(map[string]struct{})
		for _, w := range SenderWords(sender) {
			wordSet[w] = struct{}{}
		}
		messages = append(messages, scannedMessage{
			id:      msgID,
			subject: subj,
			from:    sender,
			words:   wordSet,
		})
		if onProgress != nil {
			onProgress(i+1, total, sender)
		}
	}

	groups := computeSenderGroups(messages, threshold, minGroupSize, false)
	if err := s.applyCategories(ctx, groups); err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *GmailSorter) autoCategorizePaged(ctx context.Context, candidateIDs []string, total, threshold, minGroupSize, pageSize int, onProgress ProgressFunc) (map[string][]MatchedMessage, error) {
	categoryLabelIDs := make(map[string]string)
	allMatches := make(map[string][]MatchedMessage)
	scanned := 0

	for pageStart := 0; pageStart < total; pageStart += pageSize {
		end := pageStart + pageSize
		if end > total {
			end = total
		}
		pageIDs := candidateIDs[pageStart:end]
		pageMessages := make([]scannedMessage, 0, len(pageIDs))

		for _, msgID := range pageIDs {
			msg, err := s.Client.GetMessage(ctx, msgID, "metadata")
			if err != nil {
				return nil, err
			}
			headers := MessageHeaders(msg)
			subj := headerLookup(headers, "Subject")
			sender := headerLookup(headers, "From")
			wordSet := make(map[string]struct{})
			for _, w := range SenderWords(sender) {
				wordSet[w] = struct{}{}
			}
			pageMessages = append(pageMessages, scannedMessage{
				id:      msgID,
				subject: subj,
				from:    sender,
				words:   wordSet,
			})
			scanned++
			if onProgress != nil {
				onProgress(scanned, total, sender)
			}
		}

		pageGroups := computeSenderGroups(pageMessages, threshold, minGroupSize, true)
		catNames := make([]string, 0, len(pageGroups))
		for k := range pageGroups {
			catNames = append(catNames, k)
		}
		sort.Strings(catNames)

		for _, catName := range catNames {
			matches := pageGroups[catName]
			labelID, exists := categoryLabelIDs[catName]
			if !exists {
				createdID, err := s.Client.CreateLabel(ctx, catName, "labelShow", "show")
				if err != nil {
					return nil, err
				}
				labelID = createdID
				categoryLabelIDs[catName] = labelID
			}
			ids := make([]string, len(matches))
			for i, m := range matches {
				ids[i] = m.ID
			}
			if err := s.Client.BatchModify(ctx, ids, []string{labelID}, []string{"INBOX"}, 500); err != nil {
				return nil, err
			}
			allMatches[catName] = append(allMatches[catName], matches...)
		}
	}

	return allMatches, nil
}

func (s *GmailSorter) applyCategories(ctx context.Context, categories map[string][]MatchedMessage) error {
	names := make([]string, 0, len(categories))
	for k := range categories {
		names = append(names, k)
	}
	sort.Strings(names)

	for _, name := range names {
		matches := categories[name]
		labelID, err := s.Client.CreateLabel(ctx, name, "labelShow", "show")
		if err != nil {
			return err
		}
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		if err := s.Client.BatchModify(ctx, ids, []string{labelID}, []string{"INBOX"}, 500); err != nil {
			return err
		}
	}
	return nil
}

func computeSenderGroups(messages []scannedMessage, threshold, minGroupSize int, dotSeparated bool) map[string][]MatchedMessage {
	termCounter := make(map[string]int)
	for _, m := range messages {
		for w := range m.words {
			termCounter[w]++
		}
	}

	candidateTerms := make(map[string]struct{})
	for t, c := range termCounter {
		if c >= minGroupSize {
			candidateTerms[t] = struct{}{}
		}
	}

	groups := make(map[string][]scannedMessage)
	var groupOrder []string
	for _, m := range messages {
		var shared []string
		for _, t := range SenderWords(m.from) {
			if _, ok := candidateTerms[t]; ok {
				shared = append(shared, t)
			}
		}
		if len(shared) == 0 {
			continue
		}
		key := strings.Join(shared, " ")
		if _, exists := groups[key]; !exists {
			groupOrder = append(groupOrder, key)
		}
		groups[key] = append(groups[key], m)
	}

	results := make(map[string][]MatchedMessage)
	for _, key := range groupOrder {
		group := groups[key]
		if len(group) < minGroupSize {
			continue
		}
		keyTerms := strings.Fields(key)
		var matches []MatchedMessage
		for _, m := range group {
			matched := 0
			for _, t := range keyTerms {
				if _, ok := m.words[t]; ok {
					matched++
				}
			}
			score := int(math.Round(float64(matched) / float64(len(keyTerms)) * 100.0))
			if score >= threshold {
				matches = append(matches, MatchedMessage{
					ID:      m.id,
					Subject: m.subject,
					From:    m.from,
					Score:   score,
				})
			}
		}
		if len(matches) >= minGroupSize {
			sort.SliceStable(matches, func(i, j int) bool {
				return matches[i].Score > matches[j].Score
			})
			rawCat := key
			if dotSeparated {
				rawCat = strings.ReplaceAll(key, " ", ".")
			}
			results[pyTitle(rawCat)] = matches
		}
	}
	return results
}
