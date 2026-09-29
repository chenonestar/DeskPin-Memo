package recur

import (
	"testing"
	"time"
)

var sh = time.FixedZone("CST", 8*3600)

func TestWorkdayFridayToMonday(t *testing.T) { // AC-07
	rule, _ := Build(Workday, 0)
	fri := time.Date(2026, 10, 2, 9, 0, 0, 0, sh) // 2026-10-02 是周五
	if fri.Weekday() != time.Friday {
		t.Fatal("test date")
	}
	n, ok, err := Next(rule, fri, fri, sh)
	if err != nil || !ok {
		t.Fatal(err, ok)
	}
	want := time.Date(2026, 10, 5, 9, 0, 0, 0, sh)
	if !n.Equal(want) {
		t.Fatalf("got %v want %v", n, want)
	}
}

func TestMonthly(t *testing.T) {
	rule, _ := Build(Monthly, 5)
	d := time.Date(2026, 1, 5, 9, 0, 0, 0, sh)
	n, _, _ := Next(rule, d, d, sh)
	if !n.Equal(time.Date(2026, 2, 5, 9, 0, 0, 0, sh)) {
		t.Fatal(n)
	}
}

func TestEveryNAndDescribe(t *testing.T) {
	rule, _ := Build(EveryN, 3)
	d := time.Date(2026, 1, 1, 8, 30, 0, 0, sh)
	n, _, _ := Next(rule, d, d, sh)
	if !n.Equal(d.AddDate(0, 0, 3)) {
		t.Fatal(n)
	}
	if Describe(rule) != "每 3 天" {
		t.Fatal(Describe(rule))
	}
	w, _ := Build(Weekly, 3)
	if Describe(w) != "每周三" {
		t.Fatal(Describe(w))
	}
	if Validate("FREQ=BOGUS") == nil {
		t.Fatal("should fail")
	}
}
