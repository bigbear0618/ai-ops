// Package report is the biz layer for scheduled operational reports
// (HLD-014). This PR (PR-1) lands the Usecase skeleton: schedule CRUD
// passthrough, the period calculation, and the dedup-protected report
// row creation that the cron evaluator (PR-3) and the manual
// "generate now" API (PR-4) both call. The actual content generation
// (ReportFacts collection + reporter worker) is PR-2 and is left as a
// Generator seam here.
package report

import (
	"context"
	"errors"
	"time"

	model "github.com/ongridio/ongrid/internal/manager/model/report"
	"github.com/ongridio/ongrid/internal/pkg/errs"
)

// Repo is the storage surface the Usecase needs. Implemented by
// data/report/store.Repo. Kept minimal for PR-1 — read paths the API
// layer needs (List/Get) arrive with PR-4.
type Repo interface {
	// CreateReport inserts a report row. Implementations MUST surface a
	// unique-constraint violation on (schedule_id, period_start) as
	// errs.ErrConflict so the Usecase can treat a duplicate fire as a
	// no-op rather than a hard error.
	CreateReport(ctx context.Context, r *model.Report) error
	GetReport(ctx context.Context, id string) (*model.Report, error)
	UpdateReport(ctx context.Context, r *model.Report) error

	CreateSchedule(ctx context.Context, s *model.ReportSchedule) error
	GetSchedule(ctx context.Context, id uint64) (*model.ReportSchedule, error)
	UpdateSchedule(ctx context.Context, s *model.ReportSchedule) error
	// DueSchedules returns enabled schedules whose next_fire_at <= now.
	// The evaluator (PR-3) drives generation off this.
	DueSchedules(ctx context.Context, now time.Time) ([]*model.ReportSchedule, error)
}

// IDGen mints report UUIDs. Injected so tests get deterministic ids and
// so we don't hard-depend on a uuid lib at this layer.
type IDGen func() string

// Generator turns a created (pending) report into a finished one:
// collects ReportFacts, runs the reporter worker, writes ContentJSON,
// flips status to ready/failed. PR-1 ships a no-op default; PR-2
// replaces it. The Usecase calls it asynchronously after the row is
// committed so a slow LLM never blocks the evaluator tick.
type Generator interface {
	Generate(ctx context.Context, reportID string)
}

// nopGenerator leaves the report in pending — used until PR-2 wires the
// real worker. Surfaced explicitly (not nil) so callers don't have to
// nil-check on every fire.
type nopGenerator struct{}

func (nopGenerator) Generate(context.Context, string) {}

// Usecase is the report business logic.
type Usecase struct {
	repo Repo
	gen  Generator
	idGen IDGen
}

// NewUsecase builds the Usecase. A nil Generator falls back to the
// no-op (report stays pending — useful in PR-1 tests and before PR-2
// wires the worker). A nil IDGen is a programming error and panics —
// the caller in main.go always supplies uuid.NewString.
func NewUsecase(repo Repo, gen Generator, idGen IDGen) *Usecase {
	if repo == nil {
		panic("report: nil Repo")
	}
	if idGen == nil {
		panic("report: nil IDGen")
	}
	if gen == nil {
		gen = nopGenerator{}
	}
	return &Usecase{repo: repo, gen: gen, idGen: idGen}
}

// CreateSchedule validates a new schedule and arms its initial
// next_fire_at, then persists it. Used by the API (PR-4) and the chat
// tool. For preset kinds with an empty CronSpec, the default cron for
// that kind is filled in; custom kind requires an explicit spec.
//
// `now` is the reference time the first next_fire_at is computed after
// (injected so tests are deterministic; callers pass time.Now().UTC()).
func (u *Usecase) CreateSchedule(ctx context.Context, s *model.ReportSchedule, now time.Time) error {
	if s.CronSpec == "" && s.Kind != model.KindCustom {
		spec, err := CronSpecForKind(s.Kind)
		if err != nil {
			return err
		}
		s.CronSpec = spec
	}
	loc, err := loadLocation(s.Timezone)
	if err != nil {
		return err
	}
	next, err := CronNext(s.CronSpec, loc, now)
	if err != nil {
		return err
	}
	if s.ScopeJSON == "" {
		s.ScopeJSON = "{}"
	}
	if s.ChannelIDsJSON == "" {
		s.ChannelIDsJSON = "[]"
	}
	if s.AgentPersona == "" {
		s.AgentPersona = model.DefaultReporterPersona
	}
	s.NextFireAt = &next
	return u.repo.CreateSchedule(ctx, s)
}

// FireSchedule generates one report for a due schedule and re-arms its
// next_fire_at. Called by the evaluator (PR-3). Idempotent on the
// (schedule_id, period_start) unique key: if two evaluator ticks race
// the same schedule, the second CreateReport hits ErrConflict and we
// skip generation (but still re-arm so the schedule keeps advancing).
//
// nextFireAt is computed by the caller (it owns the cron parser); we
// take it as an argument so this layer stays free of the cron lib.
func (u *Usecase) FireSchedule(ctx context.Context, s *model.ReportSchedule, fireAt, nextFireAt time.Time) (*model.Report, error) {
	loc, err := loadLocation(s.Timezone)
	if err != nil {
		return nil, err
	}
	var prevFire time.Time
	if s.LastFireAt != nil {
		prevFire = *s.LastFireAt
	}
	period, err := PeriodFor(s.Kind, fireAt, loc, prevFire)
	if err != nil {
		return nil, err
	}

	rpt := u.buildPendingReport(s, period)
	createErr := u.repo.CreateReport(ctx, rpt)

	// Re-arm the schedule regardless of whether this fire produced a new
	// report — a duplicate (ErrConflict) still means "this window is
	// handled", and we must advance next_fire_at or the evaluator will
	// re-select this row forever.
	now := fireAt
	s.LastFireAt = &now
	s.NextFireAt = &nextFireAt
	if rpt.Status == model.StatusPending {
		s.LastReportID = &rpt.ID
	}
	if uerr := u.repo.UpdateSchedule(ctx, s); uerr != nil {
		// Surface the re-arm failure — leaving next_fire_at stale would
		// cause repeated re-fires. The report (if created) is fine.
		return nil, uerr
	}

	if createErr != nil {
		if errors.Is(createErr, errs.ErrConflict) {
			// Duplicate window — already generated. Not an error.
			return nil, nil
		}
		return nil, createErr
	}

	go u.gen.Generate(context.WithoutCancel(ctx), rpt.ID)
	return rpt, nil
}

// GenerateNow creates an ad-hoc report not bound to a schedule (manual
// "generate now", API PR-4). scheduleID is nil so the dedup unique key
// doesn't apply — every manual trigger produces a fresh row.
func (u *Usecase) GenerateNow(ctx context.Context, createdBy uint64, kind, tz, scopeJSON string, period Period) (*model.Report, error) {
	if _, err := loadLocation(tz); err != nil {
		return nil, err
	}
	rpt := &model.Report{
		ID:          u.idGen(),
		ScheduleID:  nil,
		CreatedBy:   createdBy,
		Title:       TitleFor(kind, period),
		Kind:        kind,
		PeriodStart: period.Start,
		PeriodEnd:   period.End,
		Timezone:    tz,
		Status:      model.StatusPending,
		ErrorMsg:    "",
		ContentJSON: "",
		ContentMD:   "",
		ScopeJSON: scopeJSON,
	}
	if err := u.repo.CreateReport(ctx, rpt); err != nil {
		return nil, err
	}
	go u.gen.Generate(context.WithoutCancel(ctx), rpt.ID)
	return rpt, nil
}

// buildPendingReport assembles the pending row for a scheduled fire.
func (u *Usecase) buildPendingReport(s *model.ReportSchedule, p Period) *model.Report {
	id := s.ID
	return &model.Report{
		ID:            u.idGen(),
		ScheduleID:    &id,
		CreatedBy:     s.CreatedBy,
		Title:         TitleFor(s.Kind, p),
		Kind:          s.Kind,
		PeriodStart:   p.Start,
		PeriodEnd:     p.End,
		Timezone:      s.Timezone,
		Status:        model.StatusPending,
		ErrorMsg:      "",
		ContentJSON:   "",
		ContentMD:     "",
		ScopeJSON: s.ScopeJSON,
	}
}

// loadLocation resolves a schedule timezone, defaulting to UTC on empty.
// An unparseable tz is a config error surfaced as ErrInvalid.
func loadLocation(tz string) (*time.Location, error) {
	if tz == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, errors.Join(errs.ErrInvalid, err)
	}
	return loc, nil
}
