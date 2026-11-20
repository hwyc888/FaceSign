package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRecognitionThresholdsPersistAndResetOnlyOverrides(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "thresholds.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	defaults := RecognitionThresholds{
		MatchThreshold: .68, DetectionThreshold: .50,
		LivenessPassThreshold: .60, LivenessFrameThreshold: .55, LivenessSpoofThreshold: .40,
	}
	current, err := st.LoadRecognitionThresholds(ctx, defaults)
	if err != nil || current != defaults {
		t.Fatalf("defaults: %#v err=%v", current, err)
	}

	app, err := st.AppSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	app.AutoStartCheckin = true
	if err := st.UpdateAppSettings(ctx, app); err != nil {
		t.Fatal(err)
	}

	custom := RecognitionThresholds{
		MatchThreshold: .74, DetectionThreshold: .43,
		LivenessPassThreshold: .68, LivenessFrameThreshold: .57, LivenessSpoofThreshold: .37,
	}
	if err := st.SaveRecognitionThresholds(ctx, custom); err != nil {
		t.Fatal(err)
	}
	current, err = st.LoadRecognitionThresholds(ctx, defaults)
	if err != nil || current != custom {
		t.Fatalf("saved settings did not load: %#v err=%v", current, err)
	}
	if err := st.ResetRecognitionThresholds(ctx); err != nil {
		t.Fatal(err)
	}
	// Resetting removes overrides: the next process uses its startup flags.
	otherDefaults := defaults
	otherDefaults.MatchThreshold = .72
	current, err = st.LoadRecognitionThresholds(ctx, otherDefaults)
	if err != nil || current != otherDefaults {
		t.Fatalf("reset did not restore the startup baseline: %#v err=%v", current, err)
	}
	app, err = st.AppSettings(ctx)
	if err != nil || !app.AutoStartCheckin {
		t.Fatalf("reset modified unrelated settings: %#v err=%v", app, err)
	}
}
