package detector

import "testing"

func TestDetectCountsGroupsAndRepeats(t *testing.T) {
	d := New()
	got := d.Detect("this F   u   C  k thing is bullshit")
	if got.Count != 2 {
		t.Fatalf("count=%d want 2 (%+v)", got.Count, got.Matches)
	}
	for _, match := range got.Matches {
		if match.Source != "go-away" {
			t.Fatalf("source=%q want go-away", match.Source)
		}
	}
}

func TestDetectUsesLibraryFalsePositives(t *testing.T) {
	d := New()
	got := d.Detect("analysis assistant")
	if got.Count != 0 {
		t.Fatalf("count=%d want 0 (%+v)", got.Count, got.Matches)
	}
}
