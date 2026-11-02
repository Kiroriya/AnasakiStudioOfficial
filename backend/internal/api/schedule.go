package api

import (
	"context"
	"log"
	"time"

	"github.com/anasaki/studio/backend/internal/store"
)

// Scheduled posts (2.1.0): a post saved as "scheduled" (or "publish" with a
// future publishedAt) waits; the scheduler publishes it when the time comes
// and fires post.saved + post.published like a publish from the editor.

// SchedulerEvery is how often the scheduler looks for due posts.
var SchedulerEvery = 30 * time.Second

// PublishScheduled publishes the posts due at now and returns how many.
func (a *API) PublishScheduled(now time.Time) int {
	// the store fires post.saved / post.published (actor source "scheduler")
	posts, err := a.st.As(store.Actor{Source: "scheduler"}).PublishDue(now)
	if err != nil {
		log.Printf("scheduler: %v", err)
	}
	return len(posts)
}

// StartScheduler runs PublishScheduled every SchedulerEvery until ctx ends.
func (a *API) StartScheduler(ctx context.Context) {
	go func() {
		t := time.NewTicker(SchedulerEvery)
		defer t.Stop()
		a.PublishScheduled(time.Now())
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				a.PublishScheduled(now)
			}
		}
	}()
}
