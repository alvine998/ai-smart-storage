package filecommands

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"ai-smart-storage/internal/database"
)

type Action string

const (
	ActionSend     Action = "send"
	ActionDownload Action = "download"
)

type Target string

const (
	TargetFiles  Target = "files"
	TargetImages Target = "images"
)

type Command struct {
	Action    Action
	Target    Target
	All       bool
	Positions []int
	Query     string
}

func Parse(text string) (Command, bool, error) {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return Command{}, false, nil
	}

	var action Action
	var rest string
	for _, prefix := range []struct {
		text   string
		action Action
	}{
		{"kirimkan", ActionSend},
		{"kirim", ActionSend},
		{"send", ActionSend},
		{"download", ActionDownload},
		{"unduh", ActionDownload},
	} {
		if text == prefix.text {
			action = prefix.action
			rest = ""
			break
		}
		if strings.HasPrefix(text, prefix.text+" ") {
			action = prefix.action
			rest = strings.TrimSpace(strings.TrimPrefix(text, prefix.text))
			break
		}
	}
	if action == "" {
		return Command{}, false, nil
	}

	command := Command{Action: action, Target: TargetFiles}
	if strings.HasPrefix(rest, "semua ") {
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "semua "))
		command.All = true
	}
	for _, prefix := range []string{"file", "foto", "gambar"} {
		if rest == prefix {
			if prefix != "file" {
				command.Target = TargetImages
			}
			rest = ""
			break
		}
		if strings.HasPrefix(rest, prefix+" ") {
			if prefix != "file" {
				command.Target = TargetImages
			}
			rest = strings.TrimSpace(strings.TrimPrefix(rest, prefix))
			break
		}
	}
	if command.All {
		if rest != "" {
			return Command{}, true, fmt.Errorf("invalid all-files command")
		}
		return command, true, nil
	}
	if rest == "" {
		return command, true, nil
	}
	if positions, ok, err := parsePositions(rest); ok {
		if err != nil {
			return Command{}, true, err
		}
		command.Positions = positions
		return command, true, nil
	}
	command.Query = rest
	return command, true, nil
}

func parsePositions(value string) ([]int, bool, error) {
	if !strings.Contains(value, ",") {
		if _, err := strconv.Atoi(strings.TrimSpace(value)); err != nil {
			return nil, false, nil
		}
	}
	parts := strings.Split(value, ",")
	positions := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		position, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || position <= 0 {
			return nil, true, fmt.Errorf("file positions must be positive numbers separated by commas")
		}
		if _, exists := seen[position]; exists {
			return nil, true, fmt.Errorf("file positions must not contain duplicates")
		}
		seen[position] = struct{}{}
		positions = append(positions, position)
	}
	return positions, true, nil
}

type DocumentSource interface {
	Documents(context.Context, uint64, int, int) ([]database.Document, error)
	SearchDocuments(context.Context, uint64, string, int) ([]database.Document, error)
}

type Selection struct {
	Documents []database.Document
	Missing   []int
}

func Select(ctx context.Context, source DocumentSource, userID uint64, command Command) ([]database.Document, error) {
	selection, err := SelectWithMissing(ctx, source, userID, command)
	if err != nil {
		return nil, err
	}
	return selection.Documents, nil
}

func SelectWithMissing(ctx context.Context, source DocumentSource, userID uint64, command Command) (Selection, error) {
	if command.Query != "" {
		documents, err := source.SearchDocuments(ctx, userID, command.Query, 20)
		if err != nil {
			return Selection{}, err
		}
		return Selection{Documents: filterTarget(documents, command.Target)}, nil
	}
	if !command.All && len(command.Positions) == 0 {
		return Selection{}, nil
	}

	var documents []database.Document
	if command.All {
		for offset := 0; ; offset += 100 {
			page, err := source.Documents(ctx, userID, 100, offset)
			if err != nil {
				return Selection{}, err
			}
			documents = append(documents, page...)
			if len(page) < 100 {
				break
			}
		}
	} else {
		maxPosition := 0
		for _, position := range command.Positions {
			if position > maxPosition {
				maxPosition = position
			}
		}
		for offset := 0; len(documents) < maxPosition; offset += 100 {
			page, err := source.Documents(ctx, userID, 100, offset)
			if err != nil {
				return Selection{}, err
			}
			documents = append(documents, page...)
			if len(page) < 100 {
				break
			}
		}
	}

	if command.All {
		return Selection{Documents: filterTarget(documents, command.Target)}, nil
	}
	selected := make([]database.Document, 0, len(command.Positions))
	missing := make([]int, 0)
	for _, position := range command.Positions {
		if position <= len(documents) {
			selected = append(selected, documents[position-1])
		} else {
			missing = append(missing, position)
		}
	}
	return Selection{Documents: filterTarget(selected, command.Target), Missing: missing}, nil
}

func filterTarget(documents []database.Document, target Target) []database.Document {
	if target != TargetImages {
		return documents
	}
	filtered := make([]database.Document, 0, len(documents))
	for _, document := range documents {
		if strings.HasPrefix(strings.ToLower(document.MimeType), "image/") {
			filtered = append(filtered, document)
		}
	}
	return filtered
}
