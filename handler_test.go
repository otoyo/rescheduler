package main

import (
	"testing"
	"time"

	"github.com/otoyo/garoon"
)

func TestBuildTimeRanges(t *testing.T) {
	var ev garoon.Event
	var periods *[]garoon.DateTimePeriod
	var start, end garoon.Time
	var dt time.Time

	h := interactionHandler{}

	// 2019-01-06(Sun) 10:00:00 JST
	dt = time.Date(2019, 1, 6, 10, 0, 0, 0, time.Local)
	start = garoon.Time{
		DateTime: dt,
		TimeZone: "Asia/Tokyo",
	}
	end = garoon.Time{
		DateTime: dt.Add(time.Duration(1) * time.Hour),
		TimeZone: "Asia/Tokyo",
	}
	ev = garoon.Event{
		Start: start,
		End:   end,
	}
	periods, _ = h.buildTimeRanges(&ev)
	for _, p := range *periods {
		if p.Start.After(p.End) {
			t.Errorf("start is after before: %s, %s", p.Start, p.End)
			return
		}
	}

	// 2019-01-07(Mon) 10:00:00 JST
	dt = time.Date(2019, 1, 7, 10, 0, 0, 0, time.Local)
	start = garoon.Time{
		DateTime: dt,
		TimeZone: "Asia/Tokyo",
	}
	end = garoon.Time{
		DateTime: dt.Add(time.Duration(1) * time.Hour),
		TimeZone: "Asia/Tokyo",
	}
	ev = garoon.Event{
		Start: start,
		End:   end,
	}
	periods, _ = h.buildTimeRanges(&ev)
	for _, p := range *periods {
		if p.Start.After(p.End) {
			t.Errorf("start is after before: %s, %s", p.Start, p.End)
			return
		}
	}
}
