package domain

import (
	"testing"
)

func TestMeetingAlert_ResolvedMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		alert    MeetingAlert
		wantMode string
	}{
		{
			name:     "no mode and not final",
			alert:    MeetingAlert{Title: "A"},
			wantMode: "",
		},
		{
			name:     "explicit mode",
			alert:    MeetingAlert{Title: "A", Mode: "reduced"},
			wantMode: "reduced",
		},
		{
			name:     "is_final_notification without explicit mode",
			alert:    MeetingAlert{Title: "A", IsFinalNotification: true},
			wantMode: "final",
		},
		{
			name:     "explicit mode takes precedence over is_final_notification",
			alert:    MeetingAlert{Title: "A", Mode: "reduced", IsFinalNotification: true},
			wantMode: "reduced",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.alert.ResolvedMode(); got != tt.wantMode {
				t.Errorf("ResolvedMode() = %q, want %q", got, tt.wantMode)
			}
		})
	}
}
