package imbridge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ongridio/ongrid/internal/pkg/errs"
	model "github.com/ongridio/ongrid/internal/manager/model/imbridge"
)

// AdminRepo is the surface UC needs from the data layer. The webhook
// + stream paths use the more specific Repo interface; this is a
// superset that includes ImApp CRUD.
type AdminRepo interface {
	Repo
	ListApps(ctx context.Context, provider string) ([]*model.ImApp, error)
	GetApp(ctx context.Context, id uint64) (*model.ImApp, error)
	CreateApp(ctx context.Context, app *model.ImApp) error
	UpdateApp(ctx context.Context, app *model.ImApp) error
	DeleteApp(ctx context.Context, id uint64) error
}

// UC bundles the admin operations consumed by the HTTP handler.
type UC struct {
	repo AdminRepo
}

func NewUC(repo AdminRepo) *UC { return &UC{repo: repo} }

// AppInput is the mutation payload.
type AppInput struct {
	Provider    string
	Mode        string
	Name        string
	AppID       string
	AppSecret   string
	VerifyToken string
	EncryptKey  string
	AllowFrom   string // Telegram sender allowlist (numeric user IDs); see ParseAllowFrom
	Enabled     bool
}

// ParseAllowFrom splits a raw allowlist (comma / space / newline / semicolon
// separated) into normalized numeric Telegram user IDs. `telegram:` / `tg:`
// prefixes are stripped (OpenClaw allowFrom compatibility). Non-numeric and
// negative tokens are dropped — only positive user IDs are valid (group /
// supergroup chat IDs are negative and don't belong in a sender allowlist).
// Order-preserving + de-duplicated. Shared by validate() and the Telegram
// provider's poll loop so the parse rule has exactly one definition.
func ParseAllowFrom(raw string) []string {
	seen := make(map[string]struct{})
	var out []string
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t' || r == '\r' || r == ';'
	})
	for _, tok := range fields {
		tok = strings.TrimSpace(tok)
		tok = strings.TrimPrefix(tok, "telegram:")
		tok = strings.TrimPrefix(tok, "tg:")
		if tok == "" || !isNumericID(tok) {
			continue
		}
		if _, dup := seen[tok]; dup {
			continue
		}
		seen[tok] = struct{}{}
		out = append(out, tok)
	}
	return out
}

func isNumericID(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func (in *AppInput) validate() error {
	provider := strings.ToLower(strings.TrimSpace(in.Provider))
	switch provider {
	case model.ProviderFeishu, model.ProviderDingTalk, model.ProviderTelegram:
	default:
		return fmt.Errorf("%w: provider must be feishu, dingtalk, or telegram", errs.ErrInvalid)
	}
	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	if mode == "" {
		mode = model.ModeStream
	}
	if mode != model.ModeStream && mode != model.ModeWebhook {
		return fmt.Errorf("%w: mode must be stream or webhook", errs.ErrInvalid)
	}
	in.Mode = mode
	if strings.TrimSpace(in.AppID) == "" {
		return fmt.Errorf("%w: app_id required", errs.ErrInvalid)
	}
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("%w: name required", errs.ErrInvalid)
	}
	// Webhook mode requires encrypt_key for signed/encrypted events;
	// stream mode doesn't. Verify token is optional in both modes.
	if mode == model.ModeWebhook && strings.TrimSpace(in.EncryptKey) == "" {
		return fmt.Errorf("%w: encrypt_key required in webhook mode", errs.ErrInvalid)
	}
	// Telegram: poll/stream-only, and the bot is publicly discoverable by
	// username, so it MUST carry a non-empty sender allowlist. An empty
	// allowlist would let anyone on Telegram command a tool-equipped agent
	// (ADR-031; OpenClaw issue #73756). Feishu/DingTalk skip this — they're
	// gated by enterprise-tenant membership.
	if provider == model.ProviderTelegram {
		if mode != model.ModeStream {
			return fmt.Errorf("%w: telegram only supports stream mode", errs.ErrInvalid)
		}
		ids := ParseAllowFrom(in.AllowFrom)
		if len(ids) == 0 {
			return fmt.Errorf("%w: telegram requires allow_from — at least one numeric Telegram user ID (the bot is publicly reachable; an empty allowlist would let anyone command the agent)", errs.ErrInvalid)
		}
		in.AllowFrom = strings.Join(ids, ",") // canonicalize stored form
	}
	return nil
}

func (uc *UC) ListApps(ctx context.Context, provider string) ([]*model.ImApp, error) {
	return uc.repo.ListApps(ctx, provider)
}

func (uc *UC) GetApp(ctx context.Context, id uint64) (*model.ImApp, error) {
	return uc.repo.GetApp(ctx, id)
}

func (uc *UC) CreateApp(ctx context.Context, in AppInput) (*model.ImApp, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.AppSecret) == "" {
		return nil, fmt.Errorf("%w: app_secret required", errs.ErrInvalid)
	}
	now := time.Now().UTC()
	app := &model.ImApp{
		Provider:    in.Provider,
		Mode:        in.Mode,
		Name:        in.Name,
		AppID:       in.AppID,
		AppSecret:   in.AppSecret,
		VerifyToken: in.VerifyToken,
		EncryptKey:  in.EncryptKey,
		AllowFrom:   in.AllowFrom,
		Enabled:     in.Enabled,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := uc.repo.CreateApp(ctx, app); err != nil {
		return nil, fmt.Errorf("create im_app: %w", err)
	}
	return app, nil
}

// UpdateApp updates the row. Empty AppSecret = keep current (so the
// edit form doesn't have to re-display + re-submit the secret).
func (uc *UC) UpdateApp(ctx context.Context, id uint64, in AppInput) (*model.ImApp, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	cur, err := uc.repo.GetApp(ctx, id)
	if err != nil {
		return nil, err
	}
	cur.Provider = in.Provider
	cur.Mode = in.Mode
	cur.Name = in.Name
	cur.AppID = in.AppID
	if strings.TrimSpace(in.AppSecret) != "" {
		cur.AppSecret = in.AppSecret
	}
	cur.VerifyToken = in.VerifyToken
	cur.EncryptKey = in.EncryptKey
	cur.AllowFrom = in.AllowFrom
	cur.Enabled = in.Enabled
	cur.UpdatedAt = time.Now().UTC()
	if err := uc.repo.UpdateApp(ctx, cur); err != nil {
		return nil, fmt.Errorf("update im_app: %w", err)
	}
	return cur, nil
}

func (uc *UC) DeleteApp(ctx context.Context, id uint64) error {
	return uc.repo.DeleteApp(ctx, id)
}
