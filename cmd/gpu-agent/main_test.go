package main

import "testing"

// Parsing is the only logic in this program worth a test — everything else is
// one exec and one JSON encode. These cases are the ones that would silently
// report a WRONG number rather than fail, which is the failure mode that
// matters for a panel someone glances at.

func TestParseSMI(t *testing.T) {
	t.Run("one card", func(t *testing.T) {
		gpus, err := parseSMI("NVIDIA GeForce RTX 5070 Ti, 61, 38, 7400, 16384\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(gpus) != 1 {
			t.Fatalf("got %d GPUs, want 1", len(gpus))
		}
		g := gpus[0]
		if g.Name != "NVIDIA GeForce RTX 5070 Ti" {
			t.Errorf("name = %q", g.Name)
		}
		if g.TempC != 61 || g.FanPct != 38 || g.VRAMUsedMB != 7400 || g.VRAMTotalMB != 16384 {
			t.Errorf("numbers = %+v", g)
		}
	})

	// Plenty of cards never report fan speed. That is not an error and must
	// not take the lane down — the other four fields are still true.
	t.Run("N/A fan speed becomes zero, not an error", func(t *testing.T) {
		gpus, err := parseSMI("NVIDIA A100-SXM4-40GB, 44, [N/A], 0, 40960")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gpus[0].FanPct != 0 {
			t.Errorf("fan = %v, want 0", gpus[0].FanPct)
		}
		if gpus[0].TempC != 44 {
			t.Errorf("temp = %v, want 44 — an N/A column must not shift the others", gpus[0].TempC)
		}
	})

	// The reason parseSMI splits from the right. Getting this wrong reports a
	// plausible temperature that is actually a fan speed.
	t.Run("comma in the card name does not shift the columns", func(t *testing.T) {
		gpus, err := parseSMI("Weird Card, Inc. GPU, 55, 20, 100, 8192")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gpus[0].Name != "Weird Card, Inc. GPU" {
			t.Errorf("name = %q", gpus[0].Name)
		}
		if gpus[0].TempC != 55 || gpus[0].VRAMTotalMB != 8192 {
			t.Errorf("numbers = %+v", gpus[0])
		}
	})

	t.Run("two cards", func(t *testing.T) {
		gpus, err := parseSMI("Card A, 50, 30, 100, 8192\nCard B, 70, 90, 200, 12288\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(gpus) != 2 || gpus[1].Name != "Card B" || gpus[1].TempC != 70 {
			t.Errorf("got %+v", gpus)
		}
	})

	// nvidia-smi prints nothing at all when the driver is up but no card is
	// visible. Silence must not read as "a healthy machine with no GPU".
	t.Run("empty output is an error", func(t *testing.T) {
		if _, err := parseSMI("   \n"); err == nil {
			t.Fatal("expected an error for empty output")
		}
	})

	t.Run("short line is an error", func(t *testing.T) {
		if _, err := parseSMI("NVIDIA GeForce RTX 5070 Ti, 61"); err == nil {
			t.Fatal("expected an error for a truncated line")
		}
	})
}
