package sitecode

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cron expressions (site.cron): 5 fields "minute hour day-of-month month
// day-of-week" with * , - / and names (JAN-DEC, SUN-SAT), or @hourly,
// @daily, @weekly, @monthly, @yearly. Evaluated in the site timezone.

// Schedule is a parsed cron expression.
type Schedule struct {
	min, hour, dom, month, dow [64]bool
	domStar, dowStar           bool
}

var cronMacros = map[string]string{"@yearly": "0 0 1 1 *", "@annually": "0 0 1 1 *", "@monthly": "0 0 1 * *",
	"@weekly": "0 0 * * 0", "@daily": "0 0 * * *", "@midnight": "0 0 * * *", "@hourly": "0 * * * *"}

var monthNames = map[string]int{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}
var dowNames = map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}

// ParseCron parses a 5-field cron expression.
func ParseCron(expr string) (*Schedule, error) {
	e := strings.TrimSpace(strings.ToLower(expr))
	if m, ok := cronMacros[e]; ok {
		e = m
	}
	f := strings.Fields(e)
	if len(f) != 5 {
		return nil, fmt.Errorf("cron %q ต้องมี 5 ช่อง: นาที ชั่วโมง วันที่ เดือน วันในสัปดาห์ (เช่น \"0 8 * * *\")", expr)
	}
	s := &Schedule{}
	specs := []struct {
		field    string
		dst      *[64]bool
		lo, hi   int
		names    map[string]int
		star     *bool
		fieldNam string
	}{
		{f[0], &s.min, 0, 59, nil, nil, "นาที"},
		{f[1], &s.hour, 0, 23, nil, nil, "ชั่วโมง"},
		{f[2], &s.dom, 1, 31, nil, &s.domStar, "วันที่"},
		{f[3], &s.month, 1, 12, monthNames, nil, "เดือน"},
		{f[4], &s.dow, 0, 7, dowNames, &s.dowStar, "วันในสัปดาห์"},
	}
	for _, sp := range specs {
		if sp.star != nil {
			*sp.star = sp.field == "*" || sp.field == "?"
		}
		if err := parseCronField(sp.field, sp.dst, sp.lo, sp.hi, sp.names); err != nil {
			return nil, fmt.Errorf("cron %q: ช่อง%s: %v", expr, sp.fieldNam, err)
		}
	}
	if s.dow[7] {
		s.dow[0] = true // 7 = Sunday too
	}
	return s, nil
}

func parseCronField(field string, dst *[64]bool, lo, hi int, names map[string]int) error {
	num := func(x string) (int, error) {
		if n, ok := names[x]; ok {
			return n, nil
		}
		n, err := strconv.Atoi(x)
		if err != nil || n < lo || n > hi {
			return 0, fmt.Errorf("%q ต้องอยู่ระหว่าง %d-%d", x, lo, hi)
		}
		return n, nil
	}
	for _, part := range strings.Split(field, ",") {
		if part == "" {
			return fmt.Errorf("ค่าว่าง")
		}
		rng, stepStr, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			n, err := strconv.Atoi(stepStr)
			if err != nil || n < 1 || n > hi+1 {
				return fmt.Errorf("step %q ไม่ถูกต้อง", stepStr)
			}
			step = n
		}
		a, b := lo, hi
		switch {
		case rng == "*" || rng == "?":
		case strings.Contains(rng, "-"):
			x, y, _ := strings.Cut(rng, "-")
			var err error
			if a, err = num(x); err != nil {
				return err
			}
			if b, err = num(y); err != nil {
				return err
			}
			if a > b {
				return fmt.Errorf("ช่วง %q ไม่ถูกต้อง", rng)
			}
		default:
			n, err := num(rng)
			if err != nil {
				return err
			}
			a = n
			if hasStep {
				b = hi
			} else {
				b = n
			}
		}
		for i := a; i <= b; i += step {
			dst[i] = true
		}
	}
	return nil
}

func (s *Schedule) dayMatch(t time.Time) bool {
	dom, dow := s.dom[t.Day()], s.dow[int(t.Weekday())]
	switch {
	case s.domStar && s.dowStar:
		return true
	case s.domStar:
		return dow
	case s.dowStar:
		return dom
	}
	return dom || dow // both restricted: either matches (Vixie cron)
}

// Next returns the first run time strictly after t, in loc (returned in UTC);
// zero when none within 5 years.
func (s *Schedule) Next(t time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	l := t.In(loc)
	c := time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), l.Minute(), 0, 0, loc).Add(time.Minute)
	limit := l.AddDate(5, 0, 0)
	for c.Before(limit) {
		if !s.month[int(c.Month())] {
			c = time.Date(c.Year(), c.Month()+1, 1, 0, 0, 0, 0, loc)
			continue
		}
		if !s.dayMatch(c) {
			c = time.Date(c.Year(), c.Month(), c.Day()+1, 0, 0, 0, 0, loc)
			continue
		}
		if !s.hour[c.Hour()] {
			c = time.Date(c.Year(), c.Month(), c.Day(), c.Hour()+1, 0, 0, 0, loc)
			continue
		}
		if !s.min[c.Minute()] {
			c = c.Add(time.Minute)
			continue
		}
		return c.UTC()
	}
	return time.Time{}
}

// siteLocation is the site timezone (default Asia/Bangkok).
func siteLocation(opts map[string]string) *time.Location {
	if tz := opts["timezone"]; tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			return l
		}
	}
	if l, err := time.LoadLocation("Asia/Bangkok"); err == nil {
		return l
	}
	return time.FixedZone("ICT", 7*3600)
}
