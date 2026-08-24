package main

import "testing"

func TestGarminSyncSummary(t *testing.T) {
	tests := []struct {
		uploaded int
		skipped  int
		want     string
	}{
		{uploaded: 1, want: "Garmin sync successful: 1 activity"},
		{uploaded: 2, skipped: 1, want: "Garmin sync successful: 2 activities; 1 skipped"},
		{skipped: 1, want: "Garmin sync skipped: 1 activity"},
		{skipped: 2, want: "Garmin sync skipped: 2 activities"},
	}

	for _, test := range tests {
		if got := garminSyncSummary(test.uploaded, test.skipped); got != test.want {
			t.Errorf("garminSyncSummary(%d, %d) = %q, want %q", test.uploaded, test.skipped, got, test.want)
		}
	}
}
