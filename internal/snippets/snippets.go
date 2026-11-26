// Package snippets stores reusable commands. Sending them to terminals is a
// browser action over each terminal's own writer lease; nothing here
// executes commands.
package snippets

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"sshworkbench/internal/fault"
	"sshworkbench/internal/ident"
)

const commandLimit = 16 * 1024

type Snippet struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Command     string    `json:"command"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Repository interface {
	ListSnippets(context.Context) ([]Snippet, error)
	GetSnippet(context.Context, string) (Snippet, error)
	PutSnippet(context.Context, Snippet) error
	DeleteSnippet(context.Context, string) error
}

type Service struct {
	repository Repository
}

func New(repository Repository) *Service {
	return &Service{repository: repository}
}

func (service *Service) List(ctx context.Context) ([]Snippet, error) {
	return service.repository.ListSnippets(ctx)
}

func Validate(snippet Snippet) error {
	if snippet.Name == "" || utf8.RuneCountInString(snippet.Name) > 80 || strings.ContainsAny(snippet.Name, "\x00\r\n") || !utf8.ValidString(snippet.Name) {
		return fault.New(400, "invalid_snippet_name", "请填写 1–80 个字符的片段名称。")
	}
	if strings.TrimSpace(snippet.Command) == "" || len(snippet.Command) > commandLimit || !utf8.ValidString(snippet.Command) || strings.ContainsRune(snippet.Command, '\x00') {
		return fault.New(400, "invalid_snippet_command", "命令不能为空，且不超过 16 KiB。")
	}
	if utf8.RuneCountInString(snippet.Description) > 200 || !utf8.ValidString(snippet.Description) {
		return fault.New(400, "invalid_snippet_description", "说明不超过 200 个字符。")
	}
	return nil
}

func (service *Service) Save(ctx context.Context, identifier string, snippet Snippet) (Snippet, error) {
	snippet.Name = strings.TrimSpace(snippet.Name)
	snippet.Description = strings.TrimSpace(snippet.Description)
	snippet.Command = strings.TrimRight(snippet.Command, "\r\n")
	if err := Validate(snippet); err != nil {
		return Snippet{}, err
	}
	now := time.Now().UTC()
	snippet.ID, snippet.CreatedAt, snippet.UpdatedAt = identifier, now, now
	if identifier == "" {
		existing, err := service.repository.ListSnippets(ctx)
		if err != nil {
			return Snippet{}, err
		}
		if len(existing) >= 500 {
			return Snippet{}, fault.New(409, "snippet_limit", "最多保存 500 个命令片段。")
		}
		snippet.ID = ident.New()
	} else {
		previous, err := service.repository.GetSnippet(ctx, identifier)
		if err != nil {
			return Snippet{}, err
		}
		snippet.CreatedAt = previous.CreatedAt
	}
	if err := service.repository.PutSnippet(ctx, snippet); err != nil {
		return Snippet{}, err
	}
	return snippet, nil
}

func (service *Service) Delete(ctx context.Context, identifier string) error {
	return service.repository.DeleteSnippet(ctx, identifier)
}
