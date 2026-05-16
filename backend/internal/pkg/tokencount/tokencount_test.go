package tokencount

import "testing"

func TestEstimate_Empty(t *testing.T) {
	if got := Estimate(""); got != 0 {
		t.Errorf("Estimate(\"\") = %d, want 0", got)
	}
}

func TestEstimate_AsciiAroundFourPerToken(t *testing.T) {
	if got := Estimate("hello world"); got != 2 {
		t.Errorf("Estimate(hello world) = %d, want 2", got)
	}
}

func TestEstimate_CJKEachIsOneToken(t *testing.T) {
	if got := Estimate("你好世界"); got != 4 {
		t.Errorf("Estimate(你好世界) = %d, want 4", got)
	}
}

func TestEstimate_Mixed(t *testing.T) {
	if got := Estimate("Hi 你好"); got != 2 {
		t.Errorf("Estimate(Hi 你好) = %d, want 2", got)
	}
}

func TestEstimate_NonZeroForNonEmpty(t *testing.T) {
	if got := Estimate("x"); got != 1 {
		t.Errorf("Estimate(x) = %d, want >=1", got)
	}
}

func TestCalibrate_HappyPath(t *testing.T) {
	if got := Calibrate(120, 100); got != 1.2 {
		t.Errorf("Calibrate(120, 100) = %f, want 1.2", got)
	}
}

func TestCalibrate_ClampLow(t *testing.T) {
	if got := Calibrate(10, 100); got != 0.5 {
		t.Errorf("Calibrate(10, 100) = %f, want 0.5", got)
	}
}

func TestCalibrate_ClampHigh(t *testing.T) {
	if got := Calibrate(500, 100); got != 3.0 {
		t.Errorf("Calibrate(500, 100) = %f, want 3.0", got)
	}
}

func TestCalibrate_InvalidReturnsOne(t *testing.T) {
	if got := Calibrate(0, 100); got != 1.0 {
		t.Errorf("Calibrate(0, 100) = %f, want 1.0", got)
	}
	if got := Calibrate(100, 0); got != 1.0 {
		t.Errorf("Calibrate(100, 0) = %f, want 1.0", got)
	}
}

func TestMergeCalibration_FirstObservationWins(t *testing.T) {
	if got := MergeCalibration(0, 1.5); got != 1.5 {
		t.Errorf("MergeCalibration(0, 1.5) = %f, want 1.5", got)
	}
}

func TestMergeCalibration_Smooths(t *testing.T) {
	got := MergeCalibration(1.0, 2.0)
	if got < 1.29 || got > 1.31 {
		t.Errorf("MergeCalibration(1.0, 2.0) = %f, want ~1.3", got)
	}
}
