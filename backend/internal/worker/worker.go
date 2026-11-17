package worker

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"smartcampaign/internal/clock"
	"smartcampaign/internal/content"
	"smartcampaign/internal/queue"
)

// Channels the worker serves.
var channels = []string{"sms", "email"}

// resultBudget bounds the database write that records a result. It does not
// depend on the caller's context: a result that was obtained must be saved even
// while the process is shutting down.
const resultBudget = 5 * time.Second

// Config tunes the worker.
type Config struct {
	Concurrency   int           // senders per channel (20)
	BatchSize     int           // rows claimed at once, at most (20)
	Lease         time.Duration // how long a claimed row is ours (30s)
	RatePerSecond int           // per channel, strictly paced (90)
	Poll          time.Duration // idle wait between claims (200ms)
	// RequestTimeout is how long one provider call may take (5s). It stays below
	// the provider's 10s "hang" mode so a hanging call becomes a retry, not a stall.
	RequestTimeout time.Duration
	Policy         RetryPolicy
}

// DefaultConfig is the production setting.
func DefaultConfig() Config {
	return Config{
		Concurrency: 20, BatchSize: 20, Lease: 30 * time.Second, RatePerSecond: 90, Poll: 200 * time.Millisecond, RequestTimeout: 5 * time.Second,
		Policy: RetryPolicy{MaxAttempts: 5, MaxAge: 10 * time.Minute, BaseDelay: 2 * time.Second, MaxDelay: 60 * time.Second, DefaultRetryAfter: time.Second},
	}
}

// Worker sends queued deliveries.
type Worker struct {
	q      queue.Queue
	send   Sender
	policy content.Policy
	clk    clock.Clock
	log    *slog.Logger
	cfg    Config
	pacers map[string]*Pacer

	mu       sync.Mutex
	contents map[contentKey]content.Content // frozen after approval, so cacheable
}

func New(q queue.Queue, s Sender, policy content.Policy, clk clock.Clock, log *slog.Logger, cfg Config) *Worker {
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	if cfg.BatchSize < 1 {
		cfg.BatchSize = 1
	}
	w := &Worker{q: q, send: s, policy: policy, clk: clk, log: log, cfg: cfg,
		pacers: map[string]*Pacer{}, contents: map[contentKey]content.Content{}}
	for _, ch := range channels {
		w.pacers[ch] = NewPacer(clk, cfg.RatePerSecond)
	}
	return w
}

// RunOnce claims one batch per channel, processes it and returns how many
// deliveries it handled. It also completes campaigns with nothing left.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	total := 0
	for _, ch := range channels {
		n := w.cfg.BatchSize
		if n > w.cfg.Concurrency {
			n = w.cfg.Concurrency
		}
		ds, err := w.q.Claim(ctx, ch, n, w.cfg.Lease, w.clk.Now())
		if err != nil {
			return total, err
		}
		var wg sync.WaitGroup
		for _, d := range ds {
			wg.Add(1)
			go func() {
				defer wg.Done()
				w.process(ctx, d)
			}()
		}
		wg.Wait()
		total += len(ds)
	}
	if _, err := w.q.Finish(ctx, w.clk.Now()); err != nil {
		return total, err
	}
	return total, nil
}

// Run processes the queue until ctx ends. Each channel has its own claimer that
// keeps up to Concurrency senders busy; all senders of a channel share one
// pacer, which is what enforces the rate. In-flight sends finish before it returns.
func (w *Worker) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	for _, ch := range channels {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.runChannel(ctx, ch)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ctx.Err() == nil {
			if n, err := w.q.Finish(ctx, w.clk.Now()); err != nil && ctx.Err() == nil {
				w.log.Error("finish campaigns", "error", err)
			} else if n > 0 {
				w.log.Info("campaigns completed", "count", n)
			}
			_ = w.clk.Sleep(ctx, time.Second)
		}
	}()
	wg.Wait()
	return ctx.Err()
}

func (w *Worker) runChannel(ctx context.Context, ch string) {
	sem := make(chan struct{}, w.cfg.Concurrency)
	var wg sync.WaitGroup
	defer wg.Wait()
	for ctx.Err() == nil {
		free := w.cfg.Concurrency - len(sem)
		if free > w.cfg.BatchSize {
			free = w.cfg.BatchSize
		}
		claimed := 0
		if free > 0 {
			ds, err := w.q.Claim(ctx, ch, free, w.cfg.Lease, w.clk.Now())
			if err != nil && ctx.Err() == nil {
				w.log.Error("claim", "channel", ch, "error", err)
			}
			claimed = len(ds)
			for _, d := range ds {
				sem <- struct{}{}
				wg.Add(1)
				go func() {
					defer func() { <-sem; wg.Done() }()
					w.process(ctx, d)
				}()
			}
		}
		if claimed == 0 {
			_ = w.clk.Sleep(ctx, w.cfg.Poll)
		}
	}
}

// process sends one delivery and records what happened.
func (w *Worker) process(ctx context.Context, d queue.Delivery) {
	log := w.log.With("delivery", d.ID, "campaign", d.CampaignID, "channel", d.Channel)

	// Consent and contact data are checked again now: the person may have
	// opted out after the campaign was approved.
	to, ok := w.recipient(d)
	if !ok {
		w.record(func(c context.Context, now time.Time) error {
			return w.q.Skip(c, d.ID, "consent withdrawn or contact missing since approval", now)
		})
		log.Info("skipped: consent withdrawn or contact missing")
		return
	}

	lang := content.NormalizeLanguage(d.Language)
	tmpl, err := w.template(ctx, d.CampaignID, lang)
	if err != nil {
		if ctx.Err() == nil {
			log.Error("cannot load content", "error", err)
			w.record(func(c context.Context, now time.Time) error {
				return w.q.Retry(c, d.ID, now.Add(5*time.Second), queue.RetryTransient, "content unavailable", now)
			})
		}
		return
	}
	text := w.policy.RenderContent(tmpl, content.Recipient{FirstName: d.FirstName, City: d.City, Language: lang})
	// Measured once more as it will be sent: a message over 160 / 80 never
	// leaves, whatever the configuration. Retrying cannot shorten it: final.
	if reason := content.RenderedProblem(d.Channel, text); reason != "" {
		w.record(func(c context.Context, now time.Time) error { return w.q.Fail(c, d.ID, reason, now) })
		log.Error("not sent: message breaks a limit after personalization", "reason", reason)
		return
	}
	msg := Message{To: to, Body: text.SMSBody}
	if d.Channel == "email" {
		msg = Message{To: to, Subject: text.EmailSubject, Body: text.EmailBody}
	}

	pacer := w.pacers[d.Channel]
	if err := pacer.Wait(ctx); err != nil {
		return // shutting down: the lease expires and the row is picked up again
	}
	began := w.clk.Now()
	res := w.send.Send(ctx, d.Channel, msg, strconv.FormatInt(d.ID, 10))
	if ctx.Err() != nil {
		return // interrupted mid-flight: not the provider's fault, do not count it
	}

	now := w.clk.Now()
	w.record(func(c context.Context, _ time.Time) error { return w.q.Observe(c, d.ID, now.Sub(began)) })
	dec := w.cfg.Policy.Decide(res.Outcome, d.Attempts, now.Sub(d.CreatedAt))
	switch dec.Action {
	case ActionSent:
		w.record(func(c context.Context, at time.Time) error { return w.q.MarkSent(c, d.ID, res.MessageID, at) })
		log.Debug("sent", "provider_message_id", res.MessageID)
	case ActionRetry:
		at := now.Add(dec.Delay)
		if dec.Kind == queue.RetryRateLimited {
			pacer.PauseUntil(at) // the whole channel backs off, not just this message
		}
		w.record(func(c context.Context, n time.Time) error { return w.q.Retry(c, d.ID, at, dec.Kind, dec.Reason, n) })
		log.Info("will retry", "after", dec.Delay, "reason", dec.Reason, "rate_limited", dec.Kind == queue.RetryRateLimited)
	default:
		w.record(func(c context.Context, n time.Time) error { return w.q.Fail(c, d.ID, dec.Reason, n) })
		log.Warn("failed", "reason", dec.Reason)
	}
}

// recipient returns the address to use, or false when the person may not (or
// cannot) be contacted on this channel any more.
func (w *Worker) recipient(d queue.Delivery) (string, bool) {
	switch d.Channel {
	case "sms":
		return d.Phone, d.SMSConsent && d.Phone != ""
	case "email":
		return d.Email, d.EmailConsent && d.Email != ""
	}
	return "", false
}

// contentKey identifies one language's text of one campaign.
type contentKey struct {
	campaign int64
	language string
}

func (w *Worker) template(ctx context.Context, campaignID int64, lang string) (content.Content, error) {
	key := contentKey{campaignID, lang}
	w.mu.Lock()
	c, ok := w.contents[key]
	w.mu.Unlock()
	if ok {
		return c, nil
	}
	c, err := w.q.Content(ctx, campaignID, lang)
	if err != nil {
		return content.Content{}, err
	}
	w.mu.Lock()
	w.contents[key] = c
	w.mu.Unlock()
	return c, nil
}

// record writes a result with its own deadline, whatever happens to the caller.
func (w *Worker) record(f func(ctx context.Context, now time.Time) error) {
	ctx, cancel := context.WithTimeout(context.Background(), resultBudget)
	defer cancel()
	if err := f(ctx, w.clk.Now()); err != nil {
		w.log.Error("record result", "error", err)
	}
}
