package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// sendLockKey names the PostgreSQL advisory lock held by the one worker that
// sends. The rate limit lives in each process's pacer, so two sending workers
// (docker compose --scale worker=2) would send at twice the rate (~194/s per
// channel against a provider that allows 100).
const sendLockKey int64 = 0x5343_4d50_0001 // "SCMP" 1

// RunExclusive runs the worker only while this process holds the send lock.
// A worker that cannot get it does not send: it tries again every retry
// until it gets it (when the sending worker stops) or ctx ends. The lock is a
// session lock on a connection set aside for it, released when Run returns.
func (w *Worker) RunExclusive(ctx context.Context, pool *pgxpool.Pool, retry time.Duration) error {
	release, err := holdSendLock(ctx, pool, retry, w)
	if err != nil {
		return err
	}
	defer release()
	return w.Run(ctx)
}

func holdSendLock(ctx context.Context, pool *pgxpool.Pool, retry time.Duration, w *Worker) (func(), error) {
	waiting := false
	for {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			w.log.Error("send lock: no database connection", "error", err)
		} else {
			var got bool
			if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, sendLockKey).Scan(&got); err != nil {
				conn.Release()
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				w.log.Error("send lock", "error", err)
			} else if got {
				w.log.Info("send lock taken: this worker sends")
				return func() {
					c, cancel := context.WithTimeout(context.Background(), resultBudget)
					defer cancel()
					if _, err := conn.Exec(c, `SELECT pg_advisory_unlock($1)`, sendLockKey); err != nil {
						// Do not hand a connection that may still hold the lock back to the pool.
						_ = conn.Conn().Close(c)
						w.log.Error("send lock: release", "error", fmt.Errorf("unlock: %w", err))
					}
					conn.Release()
				}, nil
			} else {
				conn.Release()
				if !waiting {
					w.log.Info("another worker holds the send lock; waiting", "retry_every", retry)
					waiting = true
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retry):
		}
	}
}
