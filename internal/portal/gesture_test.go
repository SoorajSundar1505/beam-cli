package portal

import (
	"math"
	"testing"
	"time"
)

func tracked(x, y, pinch float64, file, device int, portal bool) Frame {
	return Frame{
		Present: true, Confidence: 0.9, X: x, Y: y, Pinch: pinch,
		OverFile: file, OverDevice: device, OverPortal: portal,
		ToPortalX: 0.5 - x, ToPortalY: 0.4 - y,
	}
}

func pinch(m *Machine, f Frame) View {
	var last View
	f.Pinch = 0.02
	for i := 0; i < pinchHold; i++ {
		last = m.Step(f)
	}
	return last
}

func open(m *Machine, f Frame) View {
	var last View
	f.Pinch = 0.2
	for i := 0; i < pinchHold; i++ {
		last = m.Step(f)
		if last.Send {
			return last
		}
	}
	return last
}

func trace(m *Machine, cx, cy, r float64, n int, sweep float64) View {
	var last View
	for i := 0; i <= n; i++ {
		ang := -math.Pi/2 + sweep*float64(i)/float64(n)
		last = m.Step(Frame{
			Present: true, Confidence: 0.95,
			X: cx + r*math.Cos(ang), Y: cy + r*math.Sin(ang),
			Pinch: 0.2, OverFile: -1, OverDevice: -1,
		})
		if last.Recognized {
			return last
		}
	}
	return last
}

func openPortal(t *testing.T, m *Machine) {
	t.Helper()
	view := trace(m, 0.5, 0.45, 0.14, 40, 2*math.Pi)
	if !view.Open || !view.Recognized || view.Phase != PhaseComplete || len(view.Trail) != 0 {
		t.Fatalf("open: %+v", view)
	}
}

func TestHugeOffscreenArcIsRejected(t *testing.T) {
	m := NewMachine()
	var view View
	for i := 0; i <= 24; i++ {
		ang := 1.15 + float64(i)*0.03
		view = m.Step(tracked(0.5+0.62*math.Cos(ang), -0.22+0.62*math.Sin(ang), 0.2, -1, -1, false))
	}
	if view.Open || view.RingR > 0.4 {
		t.Fatalf("oversized arc was accepted: r %.3f open %v", view.RingR, view.Open)
	}
}

func TestOneTurnOpensAndAReverseDoesNotEraseIt(t *testing.T) {
	m := NewMachine()
	view := trace(m, 0.52, 0.48, 0.16, 42, 2*math.Pi)
	if !view.Open || !view.Recognized {
		t.Fatalf("one turn did not open: phase %s degrees %.1f", view.Phase, view.Degrees)
	}
	partial := NewMachine()
	var mid View
	const n = 36
	for i := 0; i <= 24; i++ {
		ang := -math.Pi/2 + 2*math.Pi*float64(i)/float64(n)
		mid = partial.Step(Frame{
			Present: true, Confidence: 0.95, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			X: 0.52 + 0.16*math.Cos(ang), Y: 0.48 + 0.16*math.Sin(ang),
		})
	}
	if mid.Open || mid.Degrees < 180 {
		t.Fatalf("partial: %+v", mid)
	}
	held := mid.Degrees
	for i := 23; i >= 21; i-- {
		ang := -math.Pi/2 + 2*math.Pi*float64(i)/float64(n)
		mid = partial.Step(Frame{
			Present: true, Confidence: 0.95, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			X: 0.52 + 0.16*math.Cos(ang), Y: 0.48 + 0.16*math.Sin(ang),
		})
	}
	if mid.Degrees+1 < held {
		t.Fatalf("reverse erased progress: %.1f -> %.1f", held, mid.Degrees)
	}
	if mid.Open {
		t.Fatal("reverse completed the circle")
	}
}

func TestCircleOpensPortalAndShowsTrail(t *testing.T) {
	m := NewMachine()
	if m.Step(tracked(0.5, 0.45, 0.2, -1, -1, false)).Phase != PhaseReady {
		t.Fatal("portal was active before a circle")
	}
	mid := trace(m, 0.5, 0.45, 0.14, 12, math.Pi)
	if mid.Open || mid.Phase != PhaseDrawing || len(mid.Trail) < 8 {
		t.Fatalf("drawing: %+v trail %d", mid, len(mid.Trail))
	}
	m = NewMachine()
	openPortal(t, m)
}

func TestIncompleteCircleDoesNotOpen(t *testing.T) {
	m := NewMachine()
	view := trace(m, 0.5, 0.45, 0.14, 28, math.Pi)
	if view.Open || view.Recognized || view.Phase == PhaseIdle {
		t.Fatalf("incomplete opened: %+v", view)
	}
	line := NewMachine()
	var last View
	for i := 0; i < 30; i++ {
		last = line.Step(tracked(0.15+float64(i)*0.02, 0.5, 0.2, -1, -1, false))
	}
	if last.Open || last.Recognized {
		t.Fatalf("line opened: %+v", last)
	}
}

func TestCircleThresholdsRejectTightSweep(t *testing.T) {
	m := NewMachine()
	m.limits.MinSweep = 2 * math.Pi
	view := trace(m, 0.5, 0.45, 0.14, 40, 2*math.Pi*0.92)
	if view.Open {
		t.Fatal("threshold did not reject the circle")
	}
}

func TestLostHandCancelsDrawing(t *testing.T) {
	m := NewMachine()
	trace(m, 0.5, 0.45, 0.14, 10, math.Pi/2)
	start := time.Unix(0, 0)
	now := start
	m.now = func() time.Time { return now }
	miss := Frame{Present: false, OverFile: -1, OverDevice: -1}
	brief := m.Step(miss)
	if brief.Open || len(brief.Trail) == 0 || brief.DrawState != PhaseDrawing || brief.Message != "Tracking hand…" {
		t.Fatalf("grace: %+v", brief)
	}
	now = start.Add(450 * time.Millisecond)
	paused := m.Step(miss)
	if paused.DrawState != PhasePaused || len(paused.Trail) == 0 || paused.Message != "Continue drawing" {
		t.Fatalf("pause: %+v", paused)
	}
	now = start.Add(3 * time.Second)
	view := m.Step(miss)
	if view.Open || view.Phase != PhaseReady || len(view.Trail) != 0 {
		t.Fatalf("cancelled: %+v", view)
	}
}

func TestSmallAndIncompleteCirclesStayClosed(t *testing.T) {
	small := trace(NewMachine(), 0.5, 0.5, 0.04, 48, 2*math.Pi)
	if small.Open || small.Recognized {
		t.Fatalf("tiny circle opened: %+v", small)
	}
	partial := trace(NewMachine(), 0.5, 0.45, 0.16, 36, 250*math.Pi/180)
	if partial.Open || partial.Progress >= 1 {
		t.Fatalf("incomplete opened: %+v progress %v", partial, partial.Progress)
	}
	if partial.Progress < 0.5 || partial.RingR <= 0 || len(partial.Message) < 8 || partial.Message[:7] != "Circle " {
		t.Fatalf("missing progress: %+v", partial)
	}
}

func TestSlightEllipseAndNoiseStillOpen(t *testing.T) {
	ellipse := NewMachine()
	var view View
	const n = 56
	for i := 0; i <= n; i++ {
		ang := -math.Pi/2 + 2*math.Pi*float64(i)/float64(n)
		view = ellipse.Step(Frame{
			Present: true, Confidence: 0.95, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			X: 0.5 + 0.17*math.Cos(ang), Y: 0.45 + 0.145*math.Sin(ang),
		})
		if view.Recognized {
			break
		}
	}
	if !view.Open || !view.Recognized || view.Message != "Portal opening…" {
		t.Fatalf("ellipse: %+v", view)
	}

	noisy := NewMachine()
	for i := 0; i <= n; i++ {
		ang := -math.Pi/2 + 2*math.Pi*float64(i)/float64(n)
		view = noisy.Step(Frame{
			Present: true, Confidence: 0.95, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			X: 0.5 + 0.16*math.Cos(ang) + 0.008*math.Sin(float64(i)*2.2),
			Y: 0.45 + 0.16*math.Sin(ang) + 0.008*math.Cos(float64(i)*1.7),
		})
		if view.Recognized {
			break
		}
	}
	if !view.Open || !view.Recognized {
		t.Fatalf("noise: %+v", view)
	}
}

func TestGuideCenterStaysPut(t *testing.T) {
	m := NewMachine()
	var early View
	for i := 0; i <= 18; i++ {
		ang := -math.Pi/2 + math.Pi*float64(i)/18
		early = m.Step(Frame{
			Present: true, Confidence: 0.95, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			X: 0.52 + 0.16*math.Cos(ang), Y: 0.48 + 0.16*math.Sin(ang),
		})
	}
	if early.RingR <= 0 || early.Open {
		t.Fatalf("guide: %+v", early)
	}
	late := early
	for i := 19; i <= 28; i++ {
		ang := -math.Pi/2 + math.Pi*float64(i)/18
		late = m.Step(Frame{
			Present: true, Confidence: 0.95, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			X: 0.52 + 0.16*math.Cos(ang), Y: 0.48 + 0.16*math.Sin(ang),
		})
	}
	if math.Hypot(late.RingX-early.RingX, late.RingY-early.RingY) > 0.03 || late.Open {
		t.Fatalf("guide drifted: early (%v,%v) late (%v,%v)", early.RingX, early.RingY, late.RingX, late.RingY)
	}
}

func TestBriefTrackingLossKeepsTheCircle(t *testing.T) {
	m := NewMachine()
	for i := 0; i <= 12; i++ {
		ang := -math.Pi/2 + math.Pi*float64(i)/24
		m.Step(Frame{
			Present: true, Confidence: 0.95, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			X: 0.5 + 0.16*math.Cos(ang), Y: 0.45 + 0.16*math.Sin(ang),
		})
	}
	var lost View
	for i := 0; i < 4; i++ {
		lost = m.Step(Frame{Present: false, OverFile: -1, OverDevice: -1})
	}
	if lost.Open || len(lost.Trail) == 0 || lost.DrawState != PhaseDrawing || lost.Message != "Tracking hand…" {
		t.Fatalf("brief loss cleared the stroke: %+v", lost)
	}
}

func TestGapBreaksTheStrokeAndTheCircleStillOpens(t *testing.T) {
	m := NewMachine()
	const n = 48
	step := func(i int) View {
		ang := -math.Pi/2 + 2*math.Pi*float64(i)/float64(n)
		return m.Step(Frame{
			Present: true, Confidence: 0.95, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			X: 0.5 + 0.16*math.Cos(ang), Y: 0.45 + 0.16*math.Sin(ang),
		})
	}
	var view View
	for i := 0; i <= 20; i++ {
		view = step(i)
	}
	if len(view.Trail) < 8 || view.Open {
		t.Fatalf("arc: %+v", view)
	}
	for i := 0; i < 4; i++ {
		view = m.Step(Frame{Present: false, OverFile: -1, OverDevice: -1})
	}
	if len(view.Trail) == 0 {
		t.Fatal("gap cleared the trail")
	}
	view = step(26)
	cut := false
	for _, p := range view.Trail {
		if p.Cut {
			cut = true
		}
	}
	if !cut {
		t.Fatal("gap was drawn as a connected stroke")
	}
	for i := 27; i <= n+4; i++ {
		view = step(i)
		if view.Recognized {
			break
		}
	}
	if !view.Open || !view.Recognized {
		t.Fatalf("circle after a gap: %+v", view)
	}
}

func TestEitherDirectionAndAChangingRadiusOpen(t *testing.T) {
	ccw := NewMachine()
	var ccwView View
	const steps = 42
	for i := 0; i <= steps; i++ {
		ang := -math.Pi/2 - 2*math.Pi*float64(i)/float64(steps)
		ccwView = ccw.Step(Frame{
			Present: true, Confidence: 0.95, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			X: 0.5 + 0.16*math.Cos(ang), Y: 0.45 + 0.16*math.Sin(ang),
		})
		if i == 28 && ccwView.Degrees < 180 {
			t.Fatalf("counterclockwise progress stuck: %+v", ccwView)
		}
		if ccwView.Recognized {
			break
		}
	}
	if !ccwView.Open || !ccwView.Recognized {
		t.Fatalf("counterclockwise: %+v", ccwView)
	}
	growing := NewMachine()
	var view View
	const n = 52
	for i := 0; i <= n; i++ {
		ang := -math.Pi/2 + 2*math.Pi*float64(i)/float64(n)
		radius := 0.13 + 0.05*float64(i)/float64(n)
		view = growing.Step(Frame{
			Present: true, Confidence: 0.95, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			X: 0.5 + radius*math.Cos(ang), Y: 0.45 + radius*math.Sin(ang),
		})
		if i == 24 && (view.Progress < 0.25 || len(view.Trail) < 8) {
			t.Fatalf("progress stuck mid-circle: %+v", view)
		}
		if view.Recognized {
			break
		}
	}
	if !view.Open || !view.Recognized {
		t.Fatalf("changing radius: %+v", view)
	}
}

func TestWideRadiusChangeDoesNotPause(t *testing.T) {
	m := NewMachine()
	var view View
	const n = 56
	var sawPast202 bool
	for i := 0; i <= n; i++ {
		ang := -math.Pi/2 + 2*math.Pi*float64(i)/float64(n)
		radius := 0.15 + 0.06*float64(i)/float64(n)
		view = m.Step(Frame{
			Present: true, Confidence: 1, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			X: 0.5 + radius*math.Cos(ang), Y: 0.45 + radius*math.Sin(ang),
		})
		if view.Reason == "radius" {
			t.Fatalf("radius pause at %d°: %+v", int(view.Degrees), view)
		}
		if view.Degrees > 202 || view.Open {
			sawPast202 = true
		}
		if view.Recognized {
			break
		}
	}
	if !sawPast202 || !view.Open || !view.Recognized {
		t.Fatalf("variable radius stuck: %+v", view)
	}
}

func TestFastAndSlowCirclesBothOpen(t *testing.T) {
	fast := trace(NewMachine(), 0.5, 0.45, 0.16, 36, 2*math.Pi)
	if !fast.Open || !fast.Recognized {
		t.Fatalf("fast: %+v", fast)
	}
	m := NewMachine()
	start := time.Unix(0, 0)
	step := 90 * time.Millisecond
	var tick int
	m.now = func() time.Time { return start.Add(time.Duration(tick) * step) }
	var view View
	const n = 48
	for i := 0; i <= n; i++ {
		tick = i
		ang := -math.Pi/2 + 2*math.Pi*float64(i)/float64(n)
		view = m.Step(Frame{
			Present: true, Confidence: 0.95, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			X: 0.5 + 0.16*math.Cos(ang), Y: 0.45 + 0.16*math.Sin(ang),
		})
		if view.Recognized {
			break
		}
	}
	if !view.Open || !view.Recognized {
		t.Fatalf("slow: %+v", view)
	}
	if !m.coolUntil.After(m.clock()) {
		t.Fatal("cooldown was not set")
	}
}

func TestPauseDoesNotEraseASlowCircle(t *testing.T) {
	m := NewMachine()
	start := time.Unix(0, 0)
	now := start
	m.now = func() time.Time { return now }
	traceAt := func(i int) {
		ang := -math.Pi/2 + math.Pi*float64(i)/20
		now = start.Add(time.Duration(i) * 100 * time.Millisecond)
		m.Step(Frame{
			Present: true, Confidence: 0.95, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			X: 0.5 + 0.16*math.Cos(ang), Y: 0.45 + 0.16*math.Sin(ang),
		})
	}
	for i := 0; i <= 10; i++ {
		traceAt(i)
	}
	now = start.Add(4 * time.Second)
	held := m.Step(Frame{
		Present: true, Confidence: 0.95, Pinch: 0.2, OverFile: -1, OverDevice: -1,
		X: m.drawX, Y: m.drawY,
	})
	if held.Open || len(held.Trail) == 0 {
		t.Fatalf("pause cleared the circle: %+v", held)
	}
	now = start.Add(12 * time.Second)
	dropped := m.Step(Frame{
		Present: true, Confidence: 0.95, Pinch: 0.2, OverFile: -1, OverDevice: -1,
		X: m.drawX, Y: m.drawY,
	})
	if dropped.Open || len(dropped.Trail) != 0 {
		t.Fatalf("stall: %+v", dropped)
	}
}

func TestTrackingJumpDoesNotExtendTheTrail(t *testing.T) {
	m := NewMachine()
	first := m.Step(tracked(0.4, 0.4, 0.2, -1, -1, false))
	jumped := m.Step(tracked(0.9, 0.85, 0.2, -1, -1, false))
	if len(jumped.Trail) > len(first.Trail) {
		t.Fatalf("jump was drawn: %+v", jumped.Trail)
	}
}

func TestOffCircleSpikeIsNotConnected(t *testing.T) {
	m := NewMachine()
	var view View
	for i := 0; i <= 22; i++ {
		ang := -math.Pi/2 + 1.4*math.Pi*float64(i)/22
		view = m.Step(Frame{
			Present: true, Confidence: 0.95, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			X: 0.5 + 0.16*math.Cos(ang), Y: 0.45 + 0.16*math.Sin(ang),
		})
	}
	if view.RingR <= 0 {
		t.Fatalf("guide missing: %+v", view)
	}
	beforeSweep := math.Abs(view.Sweep)
	before := len(view.Trail)
	view = m.Step(tracked(view.RingX, view.RingY, 0.2, -1, -1, false))
	view = m.Step(tracked(0.05, 0.05, 0.2, -1, -1, false))
	if len(view.Trail) != before {
		t.Fatalf("spike joined the trail: before %d after %d", before, len(view.Trail))
	}
	for i := 23; i <= 40; i++ {
		ang := -math.Pi/2 + (1.4*math.Pi/22)*float64(i)
		view = m.Step(Frame{
			Present: true, Confidence: 0.95, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			X: 0.5 + 0.16*math.Cos(ang), Y: 0.45 + 0.16*math.Sin(ang),
		})
		for n := 1; n < len(view.Trail); n++ {
			if view.Trail[n].Cut {
				continue
			}
			if math.Hypot(view.Trail[n].X-view.Trail[n-1].X, view.Trail[n].Y-view.Trail[n-1].Y) > 0.12 {
				t.Fatalf("chord in the trail: %+v", view.Trail[n-1:n+1])
			}
		}
		if view.Open {
			return
		}
	}
	if math.Abs(view.Sweep) <= beforeSweep {
		t.Fatalf("progress stayed stuck after the hand returned: %+v", view)
	}
}

func TestMovementDrawsBeforeTheCircleIsRecognized(t *testing.T) {
	m := NewMachine()
	var view View
	for i := 0; i < 6; i++ {
		view = m.Step(tracked(0.30+float64(i)*0.03, 0.40, 0.02, -1, -1, false))
	}
	if view.DrawState != PhaseDrawing || len(view.Trail) < 2 || view.Open || view.Message != "Drawing" {
		t.Fatalf("stroke did not start: %+v", view)
	}
}

func TestStillHandStaysIdleAndMovementDraws(t *testing.T) {
	still := NewMachine()
	var view View
	for i := 0; i < 20; i++ {
		view = still.Step(tracked(0.4, 0.4, 0.2, -1, -1, false))
	}
	if len(view.Trail) != 0 || view.Open || view.DrawState != "IDLE" {
		t.Fatalf("still hand drew: %+v", view)
	}
	wander := NewMachine()
	for i := 0; i < 30; i++ {
		view = wander.Step(tracked(0.15+float64(i)*0.02, 0.2+float64(i)*0.01, 0.2, -1, -1, false))
	}
	if view.Open || len(view.Trail) < 2 || view.DrawState != PhaseDrawing {
		t.Fatalf("movement did not draw: %+v", view)
	}
}

func TestJitterDoesNotOpenPortal(t *testing.T) {
	m := NewMachine()
	var view View
	for i := 0; i < 40; i++ {
		view = m.Step(Frame{
			Present: true, Confidence: 0.95, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			X: 0.5 + 0.01*math.Sin(float64(i)), Y: 0.45 + 0.01*math.Cos(float64(i)*1.3),
		})
	}
	if view.Open || view.Recognized {
		t.Fatalf("jitter opened: %+v", view)
	}
}

func TestPinchIgnoredUntilCircle(t *testing.T) {
	m := NewMachine()
	view := pinch(m, tracked(0.2, 0.3, 0.02, 0, 1, false))
	if view.Open || view.Held >= 0 || view.Destination >= 0 || view.Phase == PhaseGrabbed {
		t.Fatalf("acted before portal: %+v", view)
	}
}

func TestHoverThenGrabAndDrag(t *testing.T) {
	m := NewMachine()
	openPortal(t, m)
	view := m.Step(tracked(0.1, 0.2, 0.2, 1, -1, false))
	if view.Phase != PhaseHovering || view.HoverFile != 1 || !view.Open {
		t.Fatalf("hover: %+v", view)
	}
	view = pinch(m, tracked(0.1, 0.2, 0.02, 1, -1, false))
	if view.Phase != PhaseGrabbed || view.Held != 1 {
		t.Fatalf("grab: %+v", view)
	}
	for i := 0; i < 6; i++ {
		view = m.Step(tracked(0.1-float64(i)*0.01, 0.2+float64(i)*0.01, 0.02, -1, -1, false))
	}
	if view.Phase != PhaseDragging || view.Held != 1 {
		t.Fatalf("drag: %+v", view)
	}
}

func TestPinchSelectsDeviceBeforeSend(t *testing.T) {
	m := NewMachine()
	openPortal(t, m)
	view := pinch(m, tracked(0.8, 0.3, 0.02, -1, 2, false))
	if view.Phase != PhaseDestination || view.Destination != 2 || view.Send {
		t.Fatalf("select: %+v", view)
	}
	open(m, tracked(0.8, 0.3, 0.2, -1, 2, false))
	view = pinch(m, tracked(0.1, 0.2, 0.02, 0, -1, false))
	if view.Held != 0 || view.Destination != 2 {
		t.Fatalf("grab kept destination: %+v", view)
	}
	for i := 0; i < 8; i++ {
		view = m.Step(tracked(0.1+float64(i)*0.04, 0.2+float64(i)*0.02, 0.02, -1, -1, false))
	}
	view = open(m, tracked(0.5, 0.4, 0.2, -1, -1, true))
	if !view.Send || view.Phase != PhaseTransferring || view.Held != 0 || view.Destination != 2 {
		t.Fatalf("release: %+v", view)
	}
	again := m.Step(tracked(0.5, 0.4, 0.2, -1, -1, true))
	if again.Send {
		t.Fatal("transfer fired twice")
	}
	m.Reset()
	if m.phase != PhaseDestination || m.held != -1 || !m.open {
		t.Fatalf("after transfer: phase %s held %d open %v", m.phase, m.held, m.open)
	}
}

func TestReleaseWithoutDeviceDoesNotSend(t *testing.T) {
	m := NewMachine()
	openPortal(t, m)
	pinch(m, tracked(0.1, 0.2, 0.02, 0, -1, false))
	m.Step(tracked(0.5, 0.4, 0.02, -1, -1, true))
	view := open(m, tracked(0.5, 0.4, 0.2, -1, -1, true))
	if view.Send || view.Message != "Select a device first" {
		t.Fatalf("view: %+v", view)
	}
}

func TestThrowIntoPortalSends(t *testing.T) {
	m := NewMachine()
	openPortal(t, m)
	pinch(m, tracked(0.2, 0.2, 0.02, -1, 0, false))
	open(m, tracked(0.2, 0.2, 0.2, -1, -1, false))
	pinch(m, tracked(0.1, 0.8, 0.02, 1, -1, false))
	m.Step(tracked(0.1, 0.8, 0.02, -1, -1, false))
	var view View
	for i := 0; i < 3; i++ {
		view = m.Step(tracked(0.15+float64(i)*0.08, 0.7-float64(i)*0.08, 0.02, -1, -1, i == 2))
	}
	if view.Phase != PhaseTransferring || !view.Send || view.Held != 1 {
		t.Fatalf("throw: %+v", view)
	}
}

func TestLostHandDropsFileWithoutSending(t *testing.T) {
	m := NewMachine()
	openPortal(t, m)
	pinch(m, tracked(0.2, 0.2, 0.02, -1, 0, false))
	open(m, tracked(0.2, 0.2, 0.2, -1, -1, false))
	pinch(m, tracked(0.1, 0.2, 0.02, 0, -1, false))
	var view View
	for i := 0; i < lostHandFrames; i++ {
		view = m.Step(Frame{Present: false, OverFile: -1, OverDevice: -1})
	}
	if view.Send || view.Held != -1 || view.Message != "Hand lost" || !view.Open {
		t.Fatalf("lost: %+v", view)
	}
}

func TestHandScrollsTrayFromEitherEdge(t *testing.T) {
	m := NewMachine()
	up := Frame{Present: true, Confidence: 1, X: 0.1, Y: 0.1, Pinch: 0.2, OverFile: -1, OverDevice: -1, OverTray: true, TrayY: 0.05}
	var view View
	for i := 0; i < m.limits.ScrollHold; i++ {
		view = m.Step(up)
	}
	if view.Phase != PhaseScrolling || view.Scroll >= 0 {
		t.Fatalf("scroll up: %+v", view)
	}
	mid := m.Step(Frame{Present: true, Confidence: 1, X: 0.1, Y: 0.5, Pinch: 0.2, OverFile: -1, OverDevice: -1, OverTray: true, TrayY: 0.5})
	if mid.Phase == PhaseScrolling || mid.Scroll != 0 {
		t.Fatalf("dead zone scrolled: %+v", mid)
	}
	down := Frame{Present: true, Confidence: 1, X: 0.1, Y: 0.9, Pinch: 0.2, OverFile: -1, OverDevice: -1, OverTray: true, TrayY: 0.95}
	for i := 0; i < m.limits.ScrollHold; i++ {
		view = m.Step(down)
	}
	if view.Phase != PhaseScrolling || view.Scroll <= 0 {
		t.Fatalf("scroll down: %+v", view)
	}
	left := m.Step(tracked(0.5, 0.5, 0.2, -1, -1, false))
	if left.Phase == PhaseScrolling || left.Scroll != 0 {
		t.Fatalf("left the tray: %+v", left)
	}
}

func TestScrollDoesNotRunWhileDragging(t *testing.T) {
	m := NewMachine()
	openPortal(t, m)
	pinch(m, tracked(0.2, 0.2, 0.02, 0, -1, false))
	f := tracked(0.12, 0.9, 0.02, -1, -1, false)
	f.OverTray = true
	f.TrayY = 0.95
	var view View
	for i := 0; i < 4; i++ {
		view = m.Step(f)
	}
	if view.Scroll != 0 || view.Phase == PhaseScrolling || view.Held != 0 {
		t.Fatalf("drag scrolled: %+v", view)
	}
}

func TestTrayMotionDoesNotOpenPortal(t *testing.T) {
	m := NewMachine()
	var view View
	for i := 0; i < 40; i++ {
		view = m.Step(Frame{
			Present: true, Confidence: 1, Pinch: 0.2, OverFile: -1, OverDevice: -1,
			OverTray: true, TrayY: 0.02, X: 0.08, Y: 0.1 + float64(i)*0.001,
		})
	}
	if view.Open || view.Recognized {
		t.Fatalf("tray opened the portal: %+v", view)
	}
}

func TestLowConfidenceDoesNotGrab(t *testing.T) {
	m := NewMachine()
	openPortal(t, m)
	f := tracked(0.1, 0.2, 0.02, 0, -1, false)
	f.Confidence = 0.2
	view := pinch(m, f)
	if view.Phase == PhaseGrabbed || view.Send {
		t.Fatalf("low confidence grabbed: %+v", view)
	}
}
