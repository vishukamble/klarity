package cmd

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vishukamble/klarity/pkg/config"
)

// detectedWithUnmatched builds a DetectedEnvs with one matched env and one
// unmatched cluster — used by tests that need phase 1 (Assign) to be active.
func detectedWithUnmatched() config.DetectedEnvs {
	return config.DetectedEnvs{
		Envs:      map[string][]string{"prod": {"ctx-prod"}},
		Order:     []string{"prod"},
		Unmatched: []string{"orphan-ctx"},
	}
}

// TestTUIPhaseAdvance verifies that pressing Enter on phase 0 with unmatched
// clusters advances to phase 1 (Assign).
func TestTUIPhaseAdvance(t *testing.T) {
	m := newInitTUIModel(detectedWithUnmatched(), config.DefaultConfig(), false)

	raw, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result := raw.(initTUIModel)

	if result.phase != 1 {
		t.Errorf("expected phase 1 after enter on phase 0 with unmatched clusters, got %d", result.phase)
	}
}

// TestTUIPhaseAdvanceSkipsUnmatched verifies that Enter on phase 0 skips
// phase 1 when there are no unmatched clusters (goes straight to phase 2).
func TestTUIPhaseAdvanceSkipsUnmatched(t *testing.T) {
	detected := config.DetectedEnvs{
		Envs:  map[string][]string{"prod": {"ctx-prod"}},
		Order: []string{"prod"},
		// no Unmatched
	}
	m := newInitTUIModel(detected, config.DefaultConfig(), false)

	raw, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result := raw.(initTUIModel)

	if result.phase != 2 {
		t.Errorf("expected phase 2 (skipping unmatched) after enter, got %d", result.phase)
	}
}

// TestTUIPhaseBack verifies that Esc on phase 1 goes back to phase 0.
func TestTUIPhaseBack(t *testing.T) {
	m := newInitTUIModel(detectedWithUnmatched(), config.DefaultConfig(), false)
	m.phase = 1

	raw, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	result := raw.(initTUIModel)

	if result.phase != 0 {
		t.Errorf("expected phase 0 after esc on phase 1, got %d", result.phase)
	}
}

// TestTUIPhaseBackSkipsUnmatched verifies that Esc on phase 2 skips phase 1
// and goes back to phase 0 when there were no unmatched clusters.
func TestTUIPhaseBackSkipsUnmatched(t *testing.T) {
	detected := config.DetectedEnvs{
		Envs:  map[string][]string{"prod": {"ctx-prod"}},
		Order: []string{"prod"},
	}
	m := newInitTUIModel(detected, config.DefaultConfig(), false)
	m.phase = 2

	raw, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	result := raw.(initTUIModel)

	if result.phase != 0 {
		t.Errorf("expected phase 0 (skip unmatched on back) after esc on phase 2, got %d", result.phase)
	}
}

// TestTUITierToggle verifies that Space on phase 2 toggles the tier of the
// env under the cursor.
func TestTUITierToggle(t *testing.T) {
	detected := config.DetectedEnvs{
		Envs:  map[string][]string{"staging": {"ctx-stg"}},
		Order: []string{"staging"},
	}
	m := newInitTUIModel(detected, config.DefaultConfig(), false)
	m.phase = 2
	m.tierCursor = 0

	originalTier := m.tiers["staging"] // "standard" (staging has no "prod" keyword)

	raw, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	result := raw.(initTUIModel)

	if result.tiers["staging"] == originalTier {
		t.Errorf("tier should have been toggled from %q but remained the same", originalTier)
	}
}

// TestTUITierToggleTwice verifies that two Space presses return the tier to
// its original value (toggle is symmetric).
func TestTUITierToggleTwice(t *testing.T) {
	detected := config.DetectedEnvs{
		Envs:  map[string][]string{"staging": {"ctx-stg"}},
		Order: []string{"staging"},
	}
	m := newInitTUIModel(detected, config.DefaultConfig(), false)
	m.phase = 2
	m.tierCursor = 0
	original := m.tiers["staging"]

	raw, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m2 := raw.(initTUIModel)
	raw2, _ := m2.Update(tea.KeyMsg{Type: tea.KeySpace})
	result := raw2.(initTUIModel)

	if result.tiers["staging"] != original {
		t.Errorf("tier after two toggles: want %q, got %q", original, result.tiers["staging"])
	}
}

// TestTUIQuit verifies that Ctrl+C sets done and quitting flags.
func TestTUIQuit(t *testing.T) {
	m := newInitTUIModel(detectedWithUnmatched(), config.DefaultConfig(), false)

	raw, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	result := raw.(initTUIModel)

	if !result.done {
		t.Error("expected done == true after ctrl+c")
	}
	if !result.quitting {
		t.Error("expected quitting == true after ctrl+c")
	}
}

// TestTUIQuitWithQ verifies that pressing 'q' outside input mode quits.
func TestTUIQuitWithQ(t *testing.T) {
	m := newInitTUIModel(detectedWithUnmatched(), config.DefaultConfig(), false)

	raw, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	result := raw.(initTUIModel)

	if !result.done {
		t.Error("expected done == true after pressing q")
	}
}

// TestTUIInputActiveBlocksQ verifies that 'q' does NOT quit when inputActive.
func TestTUIInputActiveBlocksQ(t *testing.T) {
	m := newInitTUIModel(detectedWithUnmatched(), config.DefaultConfig(), false)
	m.phase = 1
	m.inputActive = true

	raw, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	result := raw.(initTUIModel)

	if result.done {
		t.Error("q should not quit when inputActive is true")
	}
	// The character should have been appended to inputVal instead.
	if result.inputVal != "q" {
		t.Errorf("expected inputVal = %q, got %q", "q", result.inputVal)
	}
}

// TestTUIEscPhase0Quits verifies that Esc on phase 0 quits without saving.
func TestTUIEscPhase0Quits(t *testing.T) {
	m := newInitTUIModel(detectedWithUnmatched(), config.DefaultConfig(), false)
	m.phase = 0

	raw, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	result := raw.(initTUIModel)

	if !result.quitting {
		t.Error("expected quitting == true after esc on phase 0")
	}
	if result.resultCfg != nil {
		t.Error("resultCfg should be nil when quitting")
	}
}

// TestTUIWindowSize verifies that WindowSizeMsg stores terminal dimensions.
func TestTUIWindowSize(t *testing.T) {
	m := newInitTUIModel(detectedWithUnmatched(), config.DefaultConfig(), false)

	raw, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	result := raw.(initTUIModel)

	if result.width != 120 || result.height != 40 {
		t.Errorf("expected 120×40, got %d×%d", result.width, result.height)
	}
}

// TestTUINextPhaseSkipsUnmatched unit-tests the nextPhase helper directly.
func TestTUINextPhaseSkipsUnmatched(t *testing.T) {
	detected := config.DetectedEnvs{
		Envs:  map[string][]string{"prod": {"ctx-prod"}},
		Order: []string{"prod"},
	}
	m := newInitTUIModel(detected, config.DefaultConfig(), false)
	m.phase = 0

	if got := m.nextPhase(); got != 2 {
		t.Errorf("nextPhase() with no unmatched: want 2, got %d", got)
	}

	m2 := newInitTUIModel(detectedWithUnmatched(), config.DefaultConfig(), false)
	m2.phase = 0
	if got := m2.nextPhase(); got != 1 {
		t.Errorf("nextPhase() with unmatched: want 1, got %d", got)
	}
}

// TestTUIPrevPhaseSkipsUnmatched unit-tests the prevPhase helper directly.
func TestTUIPrevPhaseSkipsUnmatched(t *testing.T) {
	detected := config.DetectedEnvs{
		Envs:  map[string][]string{"prod": {"ctx-prod"}},
		Order: []string{"prod"},
	}
	m := newInitTUIModel(detected, config.DefaultConfig(), false)
	m.phase = 2

	if got := m.prevPhase(); got != 0 {
		t.Errorf("prevPhase() from 2 with no unmatched: want 0, got %d", got)
	}

	m2 := newInitTUIModel(detectedWithUnmatched(), config.DefaultConfig(), false)
	m2.phase = 2
	if got := m2.prevPhase(); got != 1 {
		t.Errorf("prevPhase() from 2 with unmatched: want 1, got %d", got)
	}
}
