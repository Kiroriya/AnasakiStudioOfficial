package sitecode

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
)

// Phase 4a: background jobs (site.job, jobs.enqueue) and schedules
// (site.cron). Jobs live in script_jobs (CMS database) and are claimed by
// worker goroutines with SELECT … FOR UPDATE SKIP LOCKED, so several
// server instances share the queue safely.

// JobInfo is a registered site.job.
type JobInfo struct {
	Name        string `json:"name"`
	Timeout     int    `json:"timeout"` // seconds
	Retries     int    `json:"retries"`
	Concurrency int    `json:"concurrency"`
}

// CronInfo is a registered site.cron: Name identifies the schedule (the job
// name, "<job>@2" for a second schedule of the same job, "@cron-N" for a
// function).
type CronInfo struct {
	Name string `json:"name"`
	Expr string `json:"expr"`
	Job  string `json:"job"`
}

type regJob struct {
	info JobInfo
	fn   goja.Callable
	at   Problem
}

type regCron struct {
	info CronInfo
	fn   goja.Callable // a function schedule (runs as job @cron-N)
	at   Problem
}

// jobRun is the job a VM is running (ctx.progress).
type jobRun struct {
	e  *Engine
	id int64
}

const (
	maxJobs        = 100
	maxCrons       = 100
	maxJobTimeout  = 300
	maxJobRetries  = 10
	maxJobData     = 64 << 10
	jobsPerPage    = 50
	cronJobTimeout = 60
)

var delayRe = regexp.MustCompile(`^(\d+)(s|m|h|d)$`)

// ---------------------------------------------------------------- registration

func (v *vm) siteJob(c goja.FunctionCall) goja.Value {
	if v.mode == modeHandler {
		v.throw("site.job ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler")
	}
	v.need("jobs", "site.job")
	name, _ := c.Argument(0).Export().(string)
	what := fmt.Sprintf("site.job(%q)", name)
	if !bindingNameRe.MatchString(name) {
		v.throw("site.job: ชื่อ %q ไม่ถูกต้อง (a-z ตัวแรก ตามด้วย a-z A-Z 0-9 _ - ไม่เกิน 63 ตัว)", c.Argument(0).String())
	}
	if len(v.jobDefs) >= maxJobs {
		v.throw("ลงทะเบียน site.job ได้ไม่เกิน %d งาน", maxJobs)
	}
	optsV, fnV := c.Argument(1), c.Argument(2)
	if _, ok := goja.AssertFunction(optsV); ok {
		optsV, fnV = nil, optsV
	}
	fn, ok := goja.AssertFunction(fnV)
	if !ok {
		v.throw("%s: ต้องมีฟังก์ชัน ({ data, attempt, progress }) => any", what)
	}
	info := JobInfo{Name: name, Timeout: 60, Retries: 3, Concurrency: 1}
	if o := optObject(v.rt, optsV); o != nil {
		intOpt := func(k string, lo, hi int, dst *int) {
			x := o.Get(k)
			if x == nil || goja.IsUndefined(x) || goja.IsNull(x) {
				return
			}
			f := x.ToFloat()
			if f != float64(int(f)) || int(f) < lo || int(f) > hi {
				v.throw("%s: %s ต้องเป็นจำนวนเต็ม %d ถึง %d", what, k, lo, hi)
			}
			*dst = int(f)
		}
		intOpt("timeout", 1, maxJobTimeout, &info.Timeout)
		intOpt("retries", 0, maxJobRetries, &info.Retries)
		intOpt("concurrency", 1, 20, &info.Concurrency)
	}
	v.jobDefs = append(v.jobDefs, &regJob{info: info, fn: fn, at: v.caller()})
	return goja.Undefined()
}

func (v *vm) siteCron(c goja.FunctionCall) goja.Value {
	if v.mode == modeHandler {
		v.throw("site.cron ต้องเรียกตอนเริ่มต้นของโค้ด ไม่ใช่ภายใน handler")
	}
	v.need("jobs", "site.cron")
	expr, _ := c.Argument(0).Export().(string)
	if _, err := ParseCron(expr); err != nil {
		v.throw("site.cron: %s", err.Error())
	}
	if len(v.crons) >= maxCrons {
		v.throw("ลงทะเบียน site.cron ได้ไม่เกิน %d รายการ", maxCrons)
	}
	rc := &regCron{info: CronInfo{Expr: strings.TrimSpace(expr)}, at: v.caller()}
	switch t := c.Argument(1).Export().(type) {
	case string:
		if !bindingNameRe.MatchString(t) {
			v.throw("site.cron(%q): ชื่องาน %q ไม่ถูกต้อง", expr, t)
		}
		rc.info.Job = t
	default:
		fn, ok := goja.AssertFunction(c.Argument(1))
		if !ok {
			v.throw("site.cron(%q): ต้องระบุชื่องาน (site.job) หรือฟังก์ชัน", expr)
		}
		_ = t
		rc.fn = fn
	}
	v.crons = append(v.crons, rc)
	return goja.Undefined()
}

// cronNames names the schedules of a registration run.
func cronNames(crons []*regCron) {
	perJob, fns := map[string]int{}, 0
	for _, c := range crons {
		if c.fn != nil {
			fns++
			c.info.Name = "@cron-" + strconv.Itoa(fns)
			c.info.Job = c.info.Name
			continue
		}
		perJob[c.info.Job]++
		c.info.Name = c.info.Job
		if n := perJob[c.info.Job]; n > 1 {
			c.info.Name = c.info.Job + "@" + strconv.Itoa(n)
		}
	}
}

// registerJobs validates jobs and schedules of a registration run.
func (p *Program) registerJobs(v *vm) []Problem {
	var probs []Problem
	p.jobs = map[string]*JobInfo{}
	for _, j := range v.jobDefs {
		if _, dup := p.jobs[j.info.Name]; dup {
			probs = append(probs, at(j.at, fmt.Sprintf("site.job %q ซ้ำกับที่ลงทะเบียนไว้แล้ว", j.info.Name)))
			continue
		}
		info := j.info
		p.jobs[info.Name] = &info
		p.Manifest.Jobs = append(p.Manifest.Jobs, info)
	}
	cronNames(v.crons)
	for _, c := range v.crons {
		if c.fn == nil && p.jobs[c.info.Job] == nil {
			probs = append(probs, at(c.at, fmt.Sprintf("site.cron(%q): ไม่มี site.job %q", c.info.Expr, c.info.Job)))
			continue
		}
		if c.fn != nil {
			p.jobs[c.info.Name] = &JobInfo{Name: c.info.Name, Timeout: cronJobTimeout, Retries: 0, Concurrency: 1}
		}
		p.Manifest.Crons = append(p.Manifest.Crons, c.info)
	}
	return probs
}

// jobFn finds a job's handler in v ("@cron-N" = the Nth function schedule).
func (v *vm) jobFn(name string) goja.Callable {
	if strings.HasPrefix(name, "@cron-") {
		cronNames(v.crons)
		for _, c := range v.crons {
			if c.fn != nil && c.info.Name == name {
				return c.fn
			}
		}
		return nil
	}
	for _, j := range v.jobDefs {
		if j.info.Name == name {
			return j.fn
		}
	}
	return nil
}

// ---------------------------------------------------------------- jobs.enqueue

// parseDelay reads a delay: seconds, or "30s", "10m", "24h", "7d".
func parseDelay(x goja.Value) (time.Duration, error) {
	if x == nil || goja.IsUndefined(x) || goja.IsNull(x) {
		return 0, nil
	}
	var d time.Duration
	if s, ok := x.Export().(string); ok {
		m := delayRe.FindStringSubmatch(strings.TrimSpace(s))
		if m == nil {
			return 0, errors.New("delay ต้องเป็นวินาที หรือข้อความเช่น '30s', '10m', '24h', '7d'")
		}
		n, _ := strconv.Atoi(m[1])
		unit := map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour}[m[2]]
		d = time.Duration(n) * unit
	} else {
		f := x.ToFloat()
		if f < 0 {
			return 0, errors.New("delay ต้องไม่ติดลบ")
		}
		d = time.Duration(f * float64(time.Second))
	}
	if d < 0 || d > 366*24*time.Hour {
		return 0, errors.New("delay ต้องไม่เกิน 366 วัน")
	}
	return d, nil
}

func (v *vm) jobsEnqueue(c goja.FunctionCall) goja.Value {
	const what = "jobs.enqueue"
	v.handlerOnly(what)
	v.need("jobs", what)
	name, _ := c.Argument(0).Export().(string)
	var info *JobInfo
	for _, j := range v.jobDefs {
		if j.info.Name == name {
			ji := j.info
			info = &ji
		}
	}
	if info == nil {
		v.fail("%s: ไม่มี site.job %q", what, name)
	}
	data := "null"
	if x := c.Argument(1); !goja.IsUndefined(x) && !goja.IsNull(x) {
		s, ok, err := v.toJSON(x)
		if err != nil || !ok {
			v.fail("%s: data ต้องแปลงเป็น JSON ได้", what)
		}
		data = s
	}
	if len(data) > maxJobData {
		v.fail("%s: data ใหญ่เกิน 64 KB", what)
	}
	var delay time.Duration
	key := ""
	if o := optObject(v.rt, c.Argument(2)); o != nil {
		var err error
		if delay, err = parseDelay(o.Get("delay")); err != nil {
			v.fail("%s: %s", what, err.Error())
		}
		key = optString(o, "key")
		if len(key) > 150 {
			v.fail("%s: key ยาวเกิน 150 ตัวอักษร", what)
		}
	}
	if v.h.e == nil || v.h.e.st == nil {
		v.fail("%s: ไม่พร้อมใช้งาน", what)
	}
	id, err := v.h.e.Enqueue(v.u.qualify(name), data, delay, key, info.Retries+1)
	if err != nil {
		v.fail("%s: %s", what, err.Error())
	}
	return v.rt.ToValue(id)
}

// Enqueue adds a job (data is JSON); with key, an already pending job of the
// same name and key is returned instead.
func (e *Engine) Enqueue(name, data string, delay time.Duration, key string, maxAttempts int) (int64, error) {
	now := e.now()
	var dedupe any
	if key != "" {
		dedupe = name + ":" + key
		var id int64
		err := e.st.DB.QueryRow(`SELECT id FROM script_jobs WHERE name = ? AND dedupe_key = ? AND status = 'pending' ORDER BY id LIMIT 1`,
			name, dedupe).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	}
	return e.insertJob(name, data, now.Add(delay), dedupe, maxAttempts)
}

func (e *Engine) insertJob(name, data string, runAt time.Time, dedupe any, maxAttempts int) (int64, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	res, err := e.st.DB.Exec(`INSERT INTO script_jobs (name, data, status, attempts, max_attempts, run_at, created_at, dedupe_key)
		VALUES (?, ?, 'pending', 0, ?, ?, ?, ?)`, name, data, maxAttempts, runAt.UTC(), e.now(), dedupe)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC().Truncate(time.Millisecond)
	}
	return time.Now().UTC().Truncate(time.Millisecond)
}

// ---------------------------------------------------------------- worker

type worker struct {
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running sync.Map // job id → *runningJob (cancel / shutdown interrupt it)
}

// runningJob is a job handler running on this instance.
type runningJob struct {
	v      *vm
	reason atomic.Value // error: errJobCancelled / errShutdown
}

func (r *runningJob) stop(reason error) {
	r.reason.Store(reason)
	r.v.rt.Interrupt(reason)
}

var errJobCancelled = errors.New("cancelled")

// StartWorker starts the job workers and the cron clock (cmd/server; stop
// with StopWorker). Safe mode, SITE_CODE_DISABLED and demo mode pause them.
func (e *Engine) StartWorker() {
	if e.worker != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &worker{cancel: cancel}
	e.worker = w
	n := max(e.JobWorkers, 1)
	for i := 0; i < n; i++ {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			t := time.NewTicker(e.JobPoll)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					e.ProcessJobs(ctx, 50)
				}
			}
		}()
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		for {
			wait := e.CronEvery
			if wait >= time.Minute { // production: on the minute boundary
				now := time.Now()
				wait = now.Truncate(time.Minute).Add(time.Minute).Sub(now) + 50*time.Millisecond
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				e.CronTick(e.now())
			}
		}
	}()
}

// StopWorker stops the workers: running job handlers are interrupted and
// their jobs go back to the queue (the attempt is not counted).
func (e *Engine) StopWorker() {
	w := e.worker
	if w == nil {
		return
	}
	w.cancel()
	w.running.Range(func(_, x any) bool {
		x.(*runningJob).stop(errShutdown)
		return true
	})
	w.wg.Wait()
	e.worker = nil
}

var errShutdown = errors.New("shutdown")

// claimed is a job taken by this worker.
type claimed struct {
	id                    int64
	name, data            string
	attempts, maxAttempts int
	dedupe                string
}

// claim takes the next due job (SKIP LOCKED: other workers/instances pick
// other rows). Jobs at their concurrency limit are skipped.
func (e *Engine) claim(ctx context.Context, jobs *jobUnits) (*claimed, error) {
	full := []string{}
	rows, err := e.st.DB.QueryContext(ctx, `SELECT name, COUNT(*) FROM script_jobs WHERE status = 'running' GROUP BY name`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var n string
		var c int
		if rows.Scan(&n, &c) == nil {
			if _, info := jobs.find(n); info != nil && c >= info.Concurrency {
				full = append(full, n)
			}
		}
	}
	rows.Close()
	tx, err := e.st.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	q := `SELECT id, name, COALESCE(CAST(data AS CHAR), 'null'), attempts, max_attempts, COALESCE(dedupe_key, '') FROM script_jobs
		WHERE status = 'pending' AND run_at <= ?`
	args := []any{e.now()}
	if len(full) > 0 {
		q += ` AND name NOT IN (?` + strings.Repeat(",?", len(full)-1) + `)`
		for _, n := range full {
			args = append(args, n)
		}
	}
	if e.cfg.Demo {
		// demo mode: only the Store plugins' jobs run; Site Code's and uploaded
		// plugins' jobs stay pending (not failed as "job not found")
		loaded := jobs.set.loaded()
		if len(loaded) == 0 {
			return nil, nil
		}
		q += ` AND (name LIKE ?` + strings.Repeat(" OR name LIKE ?", len(loaded)-1) + `)`
		for _, lp := range loaded {
			args = append(args, likeEscape(lp.unit.ID+":")+"%")
		}
	}
	q += ` ORDER BY run_at, id LIMIT 1 FOR UPDATE SKIP LOCKED`
	j := &claimed{}
	err = tx.QueryRowContext(ctx, q, args...).Scan(&j.id, &j.name, &j.data, &j.attempts, &j.maxAttempts, &j.dedupe)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE script_jobs SET status = 'running', attempts = attempts + 1, started_at = ?,
		finished_at = NULL, progress = NULL WHERE id = ?`, e.now(), j.id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	j.attempts++
	return j, nil
}

// ProcessJobs claims and runs due jobs until none is left (or max ran).
// The worker calls it on every poll; tests call it directly.
func (e *Engine) ProcessJobs(ctx context.Context, maxJobs int) int {
	n := 0
	for n < maxJobs && ctx.Err() == nil {
		jobs := e.jobUnits()
		if jobs == nil {
			return n // nothing published, safe mode, disabled or demo: paused
		}
		j, err := e.claim(ctx, jobs)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("site code: claim job: %v", err)
			}
			return n
		}
		if j == nil {
			return n
		}
		e.runJob(ctx, jobs, j)
		n++
	}
	return n
}

// jobUnits are the units whose jobs run (Site Code and active plugins).
type jobUnits struct {
	site *liveProg
	set  *unitSet
}

// jobUnits is nil while nothing runs (safe mode, disabled, nothing published
// and no JS plugin; in demo mode only Store plugins run).
func (e *Engine) jobUnits() *jobUnits {
	opts, err := e.st.Options()
	if err != nil || e.pluginsOff(opts) {
		return nil
	}
	j := &jobUnits{set: e.units()}
	if opts[OptVersion] != "" && !e.siteOff(opts) {
		if lp := e.current(opts); lp != nil && lp.prog != nil {
			j.site = lp
		}
	}
	if j.site == nil && j.set.empty() {
		return nil
	}
	return j
}

// find returns the unit and info of a job ("<id>:<job>" for a plugin).
func (j *jobUnits) find(name string) (*liveProg, *JobInfo) {
	unit, local := SplitUnit(name)
	var lp *liveProg
	if unit == SiteUnit {
		lp = j.site
	} else {
		lp = j.set.get(unit)
	}
	if lp == nil || lp.prog == nil {
		return nil, nil
	}
	return lp, lp.prog.jobs[local]
}

func (e *Engine) runJob(ctx context.Context, jobs *jobUnits, j *claimed) {
	lp, info := jobs.find(j.name)
	unit, local := SplitUnit(j.name)
	source := "job " + j.name
	if lp != nil {
		source = lp.unit.src("job " + local)
	}
	if info == nil {
		e.finishJob(j, "failed", "", "job not found: ไม่มี site.job "+j.name+" ในโค้ดที่เผยแพร่", false)
		e.persist([]LogEntry{{Level: "error", Source: source, Message: "job not found (#" + strconv.FormatInt(j.id, 10) + ")", Unit: unit}}, 0)
		return
	}
	timeout := time.Duration(info.Timeout) * time.Second
	release, ok := e.acquire(e.QueueWait)
	if !ok {
		e.requeue(j, 5*time.Second)
		return
	}
	defer release()
	v, err := e.liveVM(lp)
	var fn goja.Callable
	if err == nil {
		if fn = v.jobFn(local); fn == nil {
			err = &RunError{Message: "job not found: ไม่มี site.job " + j.name + " ในรอบนี้"}
		}
	}
	var result string
	rj := &runningJob{v: v}
	if err == nil {
		if w := e.worker; w != nil {
			w.running.Store(j.id, rj)
			defer w.running.Delete(j.id)
		}
		v.user, v.logs, v.source = nil, nil, source
		v.vis = &visitorState{done: true}
		v.job = &jobRun{e: e, id: j.id}
		sess := e.attachSession(v, nil, nil)
		defer sess.Close()
		var data goja.Value = goja.Null()
		if parsed, perr := v.parse(goja.Undefined(), v.rt.ToValue(j.data)); perr == nil {
			data = parsed
		}
		jctx := v.rt.NewObject()
		_ = jctx.Set("data", data)
		_ = jctx.Set("attempt", j.attempts)
		_ = jctx.Set("jobId", j.id)
		_ = jctx.Set("progress", v.jobProgress)
		_ = jctx.Set("user", goja.Null())
		_ = jctx.Set("visitor", goja.Null())
		var val goja.Value
		val, err = v.call(fn, timeout, jctx)
		if err == nil {
			if s, ok, jerr := v.toJSON(val); jerr == nil && ok {
				result = clip(s, 64000)
			}
		}
	}
	var entries []LogEntry
	if v != nil {
		entries = unitEntries(logEntries(v.logs, source), lp.unit)
	}
	if err != nil {
		re := toRunError(err)
		reason, _ := rj.reason.Load().(error)
		if reason == errShutdown || ctx.Err() != nil && re.Timeout {
			e.persist(entries, lp.id)
			e.requeueShutdown(j)
			return
		}
		if reason == errJobCancelled {
			entries = append(entries, LogEntry{Level: "warn", Source: source, Message: fmt.Sprintf("งาน #%d ถูกยกเลิก", j.id), Unit: lp.unit.id()})
			e.persist(entries, lp.id)
			return
		}
		entries = append(entries, unitEntry(errorEntry(source, re), lp.unit))
		e.persist(entries, lp.id)
		e.finishJob(j, "failed", "", re.Error(), true)
		return
	}
	e.persist(entries, lp.id)
	e.finishJob(j, "done", result, "", false)
}

// jobProgress is ctx.progress(n | text).
func (v *vm) jobProgress(c goja.FunctionCall) goja.Value {
	if v.job == nil {
		return goja.Undefined()
	}
	p := c.Argument(0).String()
	if f, ok := c.Argument(0).Export().(float64); ok {
		p = strconv.FormatFloat(f, 'f', -1, 64)
	}
	if _, err := v.job.e.st.DB.Exec(`UPDATE script_jobs SET progress = ? WHERE id = ? AND status = 'running'`, clip(p, 200), v.job.id); err != nil {
		v.addLogLine("warn", "progress: บันทึกไม่สำเร็จ")
	}
	return goja.Undefined()
}

// finishJob records the outcome; a failure with attempts left goes back to
// pending with exponential backoff (JobBackoff × 2^(attempt-1)).
func (e *Engine) finishJob(j *claimed, status, result, msg string, retry bool) {
	now := e.now()
	cronStatus := status
	var err error
	if status == "failed" && retry && j.attempts < j.maxAttempts {
		backoff := e.JobBackoff << (j.attempts - 1)
		if backoff <= 0 || backoff > 24*time.Hour {
			backoff = 24 * time.Hour
		}
		_, err = e.st.DB.Exec(`UPDATE script_jobs SET status = 'pending', run_at = ?, error = ?, finished_at = ? WHERE id = ? AND status = 'running'`,
			now.Add(backoff), clip(msg, 8000), now, j.id)
		cronStatus = "retrying"
	} else {
		var res any
		if result != "" {
			res = result
		}
		var em any
		if msg != "" {
			em = clip(msg, 8000)
		}
		_, err = e.st.DB.Exec(`UPDATE script_jobs SET status = ?, result = ?, error = ?, finished_at = ? WHERE id = ? AND status = 'running'`,
			status, res, em, now, j.id)
	}
	if err != nil {
		log.Printf("site code: finish job %d: %v", j.id, err)
	}
	if name, ok := cronOf(j.dedupe); ok {
		_, _ = e.st.DB.Exec(`UPDATE script_cron SET last_status = ? WHERE name = ?`, cronStatus, name)
	}
}

// cronOf returns the schedule of a cron-started job ("cron:<name>:<minute>").
func cronOf(dedupe string) (string, bool) {
	rest, ok := strings.CutPrefix(dedupe, "cron:")
	if !ok {
		return "", false
	}
	i := strings.LastIndex(rest, ":")
	if i <= 0 {
		return "", false
	}
	return rest[:i], true
}

func (e *Engine) requeue(j *claimed, after time.Duration) {
	_, _ = e.st.DB.Exec(`UPDATE script_jobs SET status = 'pending', attempts = GREATEST(attempts - 1, 0), run_at = ? WHERE id = ? AND status = 'running'`,
		e.now().Add(after), j.id)
}

func (e *Engine) requeueShutdown(j *claimed) { e.requeue(j, 0) }

// ---------------------------------------------------------------- admin

// JobRow is one job of GET /code/jobs.
type JobRow struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Data        any        `json:"data"`
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	MaxAttempts int        `json:"maxAttempts"`
	RunAt       time.Time  `json:"runAt"`
	StartedAt   *time.Time `json:"startedAt"`
	FinishedAt  *time.Time `json:"finishedAt"`
	Progress    *string    `json:"progress"`
	Result      any        `json:"result"`
	Error       *string    `json:"error"`
	CreatedAt   time.Time  `json:"createdAt"`
	Key         *string    `json:"key"`
	Unit        string     `json:"unit"` // "site" or the JS plugin
}

var jobStatuses = map[string]bool{"pending": true, "running": true, "done": true, "failed": true, "cancelled": true}

// IsJobStatus reports a valid job status filter.
func IsJobStatus(s string) bool { return jobStatuses[s] }

// Jobs lists jobs (newest first), filtered by status / name.
func (e *Engine) Jobs(status, name string, page, perPage int) ([]JobRow, int, error) {
	return e.JobsOf("", status, name, page, perPage)
}

// JobsOf is Jobs of one unit ("" = all; plugin jobs are "<id>:<job>").
func (e *Engine) JobsOf(unit, status, name string, page, perPage int) ([]JobRow, int, error) {
	if perPage < 1 || perPage > 200 {
		perPage = jobsPerPage
	}
	var where []string
	var args []any
	if status != "" {
		where = append(where, "status = ?")
		args = append(args, status)
	}
	if name != "" {
		where = append(where, "name = ?")
		args = append(args, name)
	}
	switch {
	case unit == SiteUnit:
		where = append(where, "name NOT LIKE '%:%'")
	case unit != "":
		where = append(where, "name LIKE ?")
		args = append(args, likeEscape(unit+":")+"%")
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := e.st.DB.QueryRow(`SELECT COUNT(*) FROM script_jobs`+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	page = max(page, 1)
	rows, err := e.st.DB.Query(`SELECT id, name, COALESCE(CAST(data AS CHAR), 'null'), status, attempts, max_attempts, run_at, started_at,
		finished_at, progress, result, error, created_at, dedupe_key FROM script_jobs`+cond+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, perPage, (page-1)*perPage)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []JobRow{}
	for rows.Next() {
		var j JobRow
		var data string
		var started, finished sql.NullTime
		var progress, result, errMsg, key sql.NullString
		if err := rows.Scan(&j.ID, &j.Name, &data, &j.Status, &j.Attempts, &j.MaxAttempts, &j.RunAt, &started, &finished,
			&progress, &result, &errMsg, &j.CreatedAt, &key); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal([]byte(data), &j.Data)
		j.Unit, _ = SplitUnit(j.Name)
		if started.Valid {
			j.StartedAt = &started.Time
		}
		if finished.Valid {
			j.FinishedAt = &finished.Time
		}
		if progress.Valid {
			j.Progress = &progress.String
		}
		if result.Valid {
			if json.Unmarshal([]byte(result.String), &j.Result) != nil {
				j.Result = result.String
			}
		}
		if errMsg.Valid {
			j.Error = &errMsg.String
		}
		if key.Valid {
			k := strings.TrimPrefix(key.String, j.Name+":")
			j.Key = &k
		}
		out = append(out, j)
	}
	return out, total, rows.Err()
}

// ErrJobState: the job cannot change from its current status (409).
var ErrJobState = errors.New("job state")

// CancelJob cancels a pending or running job (a running handler on this
// instance is interrupted).
func (e *Engine) CancelJob(id int64) error {
	res, err := e.st.DB.Exec(`UPDATE script_jobs SET status = 'cancelled', finished_at = ? WHERE id = ? AND status IN ('pending', 'running')`, e.now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return e.jobMissing(id)
	}
	if w := e.worker; w != nil {
		if x, ok := w.running.Load(id); ok {
			x.(*runningJob).stop(errJobCancelled)
		}
	}
	return nil
}

// RetryJob puts a failed or cancelled job back in the queue (attempts reset).
func (e *Engine) RetryJob(id int64) error {
	res, err := e.st.DB.Exec(`UPDATE script_jobs SET status = 'pending', attempts = 0, run_at = ?, started_at = NULL, finished_at = NULL,
		progress = NULL, error = NULL, result = NULL WHERE id = ? AND status IN ('failed', 'cancelled')`, e.now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return e.jobMissing(id)
	}
	return nil
}

func (e *Engine) jobMissing(id int64) error {
	var n int
	if err := e.st.DB.QueryRow(`SELECT COUNT(*) FROM script_jobs WHERE id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return ErrJobState
}

// ---------------------------------------------------------------- cron

// CronRow is one schedule of GET /code/cron.
type CronRow struct {
	Name       string     `json:"name"`
	Expr       string     `json:"expr"`
	Job        string     `json:"job"`
	Unit       string     `json:"unit"`
	NextRun    *time.Time `json:"nextRun"`
	LastRun    *time.Time `json:"lastRun"`
	LastStatus *string    `json:"lastStatus"`
}

// CronTick runs the schedules due at now's minute: each instance tries to
// move a schedule's next_run with a conditional UPDATE, and only the one
// that succeeds enqueues the job (one run per schedule per minute).
func (e *Engine) CronTick(now time.Time) int {
	jobs := e.jobUnits()
	if jobs == nil {
		return 0
	}
	opts, _ := e.st.Options()
	loc := siteLocation(opts)
	minute := now.UTC().Truncate(time.Minute)
	started := 0
	for _, c := range jobs.crons() {
		sched, err := ParseCron(c.Expr)
		if err != nil {
			continue
		}
		var expr string
		var next sql.NullTime
		err = e.st.DB.QueryRow(`SELECT expr, next_run FROM script_cron WHERE name = ?`, c.Name).Scan(&expr, &next)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.Printf("site code: cron %s: %v", c.Name, err)
			continue
		}
		if errors.Is(err, sql.ErrNoRows) || expr != c.Expr || !next.Valid {
			first := sched.Next(minute.Add(-time.Second), loc) // the first run at or after this minute
			if first.IsZero() {
				continue
			}
			if _, err := e.st.DB.Exec(`INSERT INTO script_cron (name, expr, next_run) VALUES (?, ?, ?)
				ON DUPLICATE KEY UPDATE expr = VALUES(expr), next_run = VALUES(next_run)`, c.Name, c.Expr, first); err != nil {
				continue
			}
			next = sql.NullTime{Time: first, Valid: true}
		}
		if next.Time.After(minute) {
			continue
		}
		upcoming := sched.Next(minute, loc)
		res, err := e.st.DB.Exec(`UPDATE script_cron SET next_run = ?, last_run = ?, last_status = 'queued' WHERE name = ? AND next_run = ?`,
			nullTime(upcoming), minute, c.Name, next.Time)
		if err != nil {
			continue
		}
		if n, _ := res.RowsAffected(); n != 1 {
			continue // another instance took this run
		}
		_, info := jobs.find(c.Job)
		attempts := 1
		if info != nil {
			attempts = info.Retries + 1
		}
		data, _ := json.Marshal(map[string]any{"cron": c.Name, "scheduledAt": minute.Format(time.RFC3339)})
		if _, err := e.insertJob(c.Job, string(data), minute, "cron:"+c.Name+":"+strconv.FormatInt(minute.Unix(), 10), attempts); err != nil {
			log.Printf("site code: cron %s: enqueue: %v", c.Name, err)
			continue
		}
		started++
	}
	return started
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// crons are the schedules of every unit, names qualified ("<id>:<name>").
func (j *jobUnits) crons() []CronInfo {
	var out []CronInfo
	for _, lp := range j.set.loaded() {
		for _, c := range lp.prog.Manifest.Crons {
			out = append(out, CronInfo{Name: lp.unit.qualify(c.Name), Expr: c.Expr, Job: lp.unit.qualify(c.Job)})
		}
	}
	if j.site != nil {
		out = append(out, j.site.prog.Manifest.Crons...)
	}
	return out
}

// Crons lists the live schedules with their next/last run.
func (e *Engine) Crons() ([]CronRow, error) { return e.CronsOf("") }

// CronsOf is Crons of one unit ("" = all).
func (e *Engine) CronsOf(unit string) ([]CronRow, error) {
	out := []CronRow{}
	opts, err := e.st.Options()
	if err != nil {
		return nil, err
	}
	list := e.ManifestFor(opts).Crons
	if !e.pluginsOff(opts) {
		for _, lp := range e.units().loaded() {
			for _, c := range lp.prog.Manifest.Crons {
				list = append(list, CronInfo{Name: lp.unit.qualify(c.Name), Expr: c.Expr, Job: lp.unit.qualify(c.Job)})
			}
		}
	}
	loc := siteLocation(opts)
	for _, c := range list {
		cu, _ := SplitUnit(c.Name)
		if unit != "" && cu != unit {
			continue
		}
		row := CronRow{Name: c.Name, Expr: c.Expr, Job: c.Job, Unit: cu}
		var expr string
		var next, last sql.NullTime
		var status sql.NullString
		err := e.st.DB.QueryRow(`SELECT expr, next_run, last_run, last_status FROM script_cron WHERE name = ?`, c.Name).Scan(&expr, &next, &last, &status)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if next.Valid && expr == c.Expr {
			t := next.Time
			row.NextRun = &t
		} else if s, err := ParseCron(c.Expr); err == nil {
			if t := s.Next(e.now(), loc); !t.IsZero() {
				row.NextRun = &t
			}
		}
		if last.Valid {
			t := last.Time
			row.LastRun = &t
		}
		if status.Valid {
			row.LastStatus = &status.String
		}
		out = append(out, row)
	}
	return out, nil
}

// RunCronNow enqueues a schedule's job immediately (POST /code/cron/{name}/run).
func (e *Engine) RunCronNow(name string) (int64, error) {
	jobs := e.jobUnits()
	if jobs == nil {
		opts, _ := e.st.Options()
		m := e.ManifestFor(opts)
		for _, c := range m.Crons {
			if c.Name == name {
				return 0, &ErrBadSettings{Msg: "Site Code ปิดอยู่ (safe mode หรือปิดด้วย env) — รันไม่ได้"}
			}
		}
		return 0, ErrNotFound
	}
	for _, c := range jobs.crons() {
		if c.Name != name {
			continue
		}
		_, info := jobs.find(c.Job)
		attempts := 1
		if info != nil {
			attempts = info.Retries + 1
		}
		now := e.now()
		data, _ := json.Marshal(map[string]any{"cron": c.Name, "manual": true})
		id, err := e.insertJob(c.Job, string(data), now, "cron:"+c.Name+":manual-"+strconv.FormatInt(now.UnixMilli(), 10), attempts)
		if err != nil {
			return 0, err
		}
		_, _ = e.st.DB.Exec(`INSERT INTO script_cron (name, expr, last_run, last_status) VALUES (?, ?, ?, 'queued')
			ON DUPLICATE KEY UPDATE last_run = VALUES(last_run), last_status = VALUES(last_status)`, c.Name, c.Expr, now.Truncate(time.Second))
		return id, nil
	}
	return 0, ErrNotFound
}
