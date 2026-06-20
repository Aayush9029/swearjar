package detector

import "testing"

func TestDetectCountsGroupsAndRepeats(t *testing.T) {
	d := New()
	got := d.Detect("this fukc thing is bullshit")
	if got.Count != 2 {
		t.Fatalf("count=%d want 2 (%+v)", got.Count, got.Matches)
	}
	if got.Matches[0].Group != "fuck" || got.Matches[1].Group != "shit" {
		t.Fatalf("matches=%+v", got.Matches)
	}
}

func TestDetectUsesLibraryFalsePositives(t *testing.T) {
	d := New()
	got := d.Detect("analysis assistant")
	if got.Count != 0 {
		t.Fatalf("count=%d want 0 (%+v)", got.Count, got.Matches)
	}
}

func TestDetectSkipsProgrammingFalseHits(t *testing.T) {
	d := New()
	got := d.Detect("assistant typescript package class pass string and constructor")
	if got.Count != 0 {
		t.Fatalf("count=%d want 0 (%+v)", got.Count, got.Matches)
	}
}

func TestDetectSkipsNormalThis(t *testing.T) {
	d := New()
	got := d.Detect("what the fuck is this shit")
	if got.Count != 2 {
		t.Fatalf("count=%d want 2 (%+v)", got.Count, got.Matches)
	}
}

func TestDetectSkipsModerationOnlyTerms(t *testing.T) {
	d := New()
	got := d.Detect("sex analytics should not create fragments")
	if got.Count != 0 {
		t.Fatalf("count=%d want 0 (%+v)", got.Count, got.Matches)
	}
}

func TestDetectSkipsCommonWordFuzzyNoise(t *testing.T) {
	d := New()
	got := d.Detect("that where batch pick icon dock count parse want func docker prickly grape scrap")
	if got.Count != 0 {
		t.Fatalf("count=%d want 0 (%+v)", got.Count, got.Matches)
	}
}

func TestDetectKeepsSwearTyposAndCompounds(t *testing.T) {
	d := New()
	got := d.Detect("fukc bullshit fucking")
	if got.Count != 3 {
		t.Fatalf("count=%d want 3 (%+v)", got.Count, got.Matches)
	}
}
