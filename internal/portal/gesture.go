package portal

import (
	"fmt"
	"math"
	"time"
)

const (
	PhaseReady        = "READY"
	PhaseDrawing      = "DRAWING"
	PhasePaused       = "PAUSED"
	PhaseComplete     = "COMPLETE"
	PhaseIdle         = "IDLE"
	PhaseHovering     = "HOVERING"
	PhasePinching     = "PINCHING"
	PhaseScrolling    = "SCROLLING"
	PhaseGrabbed      = "GRABBED"
	PhaseDragging     = "DRAGGING"
	PhaseDestination  = "DESTINATION_SELECTED"
	PhaseThrowing     = "THROWING"
	PhaseTransferring = "TRANSFERRING"
)

const (
	pinchClose     = 0.09
	minConfidence  = 0.5
	pinchHold      = 3
	lostHandFrames = 8
	dragDistance   = 0.02
	throwSpeed     = 0.04
	pointGap       = 0.008
	maxJump        = 0.18
	// resumeJoin continues a segment when the fingertip reappears close to
	// where it left. Anything farther starts a new segment and is not joined.
	resumeJoin = 0.07
	// startMove is how far the fingertip must travel from its first sample
	// before the stroke begins. It is not a radius around the guide.
	startMove = 0.02
)

// Limits decide when a fingertip path counts as a circle.
type Limits struct {
	MinPoints   int
	MinRadius   float64
	MaxRadius   float64
	RadiusCV    float64
	MinSweep    float64
	CloseFactor float64
	StallFrames int
	MaxPoints   int
	Smooth      float64
	Band        float64
	Stall       time.Duration
	Cooldown    time.Duration
	PauseAfter  time.Duration
	LostFor     time.Duration
	ScrollEdge  float64
	ScrollDead  float64
	ScrollStep  float64
	ScrollHold  int
}

// DefaultLimits is the circle the portal accepts.
func DefaultLimits() Limits {
	return Limits{
		MinPoints:   24,
		MinRadius:   0.11,
		MaxRadius:   0.46,
		RadiusCV:    0.42,
		MinSweep:    300 * math.Pi / 180,
		CloseFactor: 0.58,
		StallFrames: 24,
		MaxPoints:   480,
		Smooth:      0.55,
		Band:        0.45,
		Stall:       6 * time.Second,
		Cooldown:    1500 * time.Millisecond,
		PauseAfter:  400 * time.Millisecond,
		LostFor:     2500 * time.Millisecond,
		ScrollEdge:  0.22,
		ScrollDead:  0.06,
		ScrollStep:  18,
		ScrollHold:  2,
	}
}

// Point is one fingertip sample in portal coordinates.
type Point struct {
	X   float64 `json:"x"`
	Y   float64 `json:"y"`
	Cut bool    `json:"cut,omitempty"`
}

// Frame is one hand-tracking sample. Coordinates are 0..1 in the portal.
// Pinch is the thumb-to-index distance. Hit targets are chosen by the UI.
type Frame struct {
	Present    bool    `json:"present"`
	Confidence float64 `json:"confidence"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Pinch      float64 `json:"pinch"`
	OverFile   int     `json:"overFile"`
	OverDevice int     `json:"overDevice"`
	OverPortal bool    `json:"overPortal"`
	OverTray   bool    `json:"overTray"`
	TrayY      float64 `json:"trayY"`
	ToPortalX  float64 `json:"toPortalX"`
	ToPortalY  float64 `json:"toPortalY"`
}

// View is the gesture state the portal draws.
type View struct {
	Phase       string  `json:"phase"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Held        int     `json:"held"`
	Destination int     `json:"destination"`
	HoverFile   int     `json:"hoverFile"`
	HoverDevice int     `json:"hoverDevice"`
	Send        bool    `json:"send"`
	Open        bool    `json:"open"`
	Recognized  bool    `json:"recognized"`
	Scroll      float64 `json:"scroll"`
	Progress    float64 `json:"progress"`
	OffCircle   bool    `json:"offCircle"`
	RingX       float64 `json:"ringX"`
	RingY       float64 `json:"ringY"`
	RingR       float64 `json:"ringR"`
	Sweep       float64 `json:"sweep"`
	Arc         float64 `json:"arc"`
	Degrees     float64 `json:"degrees"`
	Tracked     bool    `json:"tracked"`
	Confidence  float64 `json:"confidence"`
	Reason      string  `json:"reason,omitempty"`
	LostMs      int     `json:"lostMs,omitempty"`
	Trail       []Point `json:"trail,omitempty"`
	Message     string  `json:"message,omitempty"`
	DrawState   string  `json:"drawState,omitempty"`
}

// Machine tracks pinch, drag, and throw gestures. It does not transfer files.
type Machine struct {
	phase      string
	held       int
	dest       int
	pinchOn    int
	pinchOff   int
	lost       int
	ready      bool
	tracked    bool
	conf       float64
	x          float64
	y          float64
	lastX      float64
	lastY      float64
	grabX      float64
	grabY      float64
	throwing   int
	sent       bool
	open       bool
	path       []Point
	trail      []Point
	anchorX    float64
	anchorY    float64
	anchored   bool
	started    bool
	strokeDead bool
	reason     string
	pendingCut bool
	begun      time.Time
	stall      int
	scrollOn   int
	limits     Limits
	drawX      float64
	drawY      float64
	drawReady  bool
	rawX       float64
	rawY       float64
	haveRaw    bool
	breakNext  bool
	lostAt     time.Time
	offBand    int
	fx         oneEuro
	fy         oneEuro
	guideOn    bool
	guideCX    float64
	guideCY    float64
	guideR     float64
	guideSweep float64
	guideErr   float64
	arc0       float64
	lastAng    float64
	haveAng    bool
	dir        int
	back       int
	candX      float64
	candY      float64
	candR      float64
	seenCenter bool
	stable     int
	moved      time.Time
	coolUntil  time.Time
	now        func() time.Time
}

func NewMachine() *Machine {
	return &Machine{
		phase: PhaseReady, held: -1, dest: -1, limits: DefaultLimits(),
		fx: newOneEuro(), fy: newOneEuro(),
	}
}

// Reset returns to the selected destination after a transfer finishes.
func (m *Machine) Reset() {
	m.sent = false
	m.held = -1
	m.throwing = 0
	if m.phase == PhaseTransferring {
		if m.dest >= 0 {
			m.phase = PhaseDestination
		} else {
			m.phase = PhaseIdle
		}
	}
}

func (m *Machine) Step(f Frame) View {
	view := View{Phase: m.phase, X: m.x, Y: m.y, Held: m.held, Destination: m.dest, HoverFile: -1, HoverDevice: -1}
	if m.sent {
		view.Phase = PhaseTransferring
		m.phase = PhaseTransferring
		return view
	}
	tracked := f.Present && f.Confidence >= minConfidence
	m.tracked = tracked
	m.conf = 0
	if tracked {
		m.conf = f.Confidence
	}
	if !tracked {
		m.lost++
		m.pinchOn = 0
		m.pinchOff = 0
		if !m.open && (m.started || len(m.path) > 0) {
			return m.missedStroke(view)
		}
		m.fx.reset()
		m.fy.reset()
		if m.lost >= m.lostLimit() {
			if m.holding() {
				m.drop(PhaseIdle)
				view.Message = "Hand lost"
			} else if len(m.path) > 0 || m.phase == PhaseDrawing {
				m.clearPath()
				m.phase = PhaseReady
				view.Message = "Start drawing"
			} else if m.phase == PhaseScrolling || m.phase == PhasePinching {
				m.scrollOn = 0
				if m.open {
					m.phase = PhaseIdle
				} else {
					m.phase = PhaseReady
				}
			}
		}
		return m.fill(view)
	}
	m.lost = 0
	m.lostAt = time.Time{}
	m.ready = true
	dx, dy := f.X-m.lastX, f.Y-m.lastY
	speed := math.Hypot(dx, dy)
	toward := dx*f.ToPortalX + dy*f.ToPortalY
	m.lastX, m.lastY = f.X, f.Y

	closed := f.Pinch >= 0 && f.Pinch < pinchClose
	if closed {
		m.pinchOn++
		m.pinchOff = 0
	} else {
		m.pinchOff++
		m.pinchOn = 0
	}
	pinchDown := m.pinchOn == pinchHold
	pinchUp := m.pinchOff == pinchHold

	scrolled := m.scrollTray(f, &view)
	if m.phase == PhaseScrolling && !scrolled {
		if m.open {
			m.phase = PhaseIdle
		} else {
			m.phase = PhaseReady
		}
	}
	if scrolled {
		return m.fill(view)
	}

	if !m.open {
		return m.draw(f, view)
	}
	now := m.clock()
	m.x = m.fx.filter(f.X, now)
	m.y = m.fy.filter(f.Y, now)
	m.drawX, m.drawY = m.x, m.y

	if f.OverFile >= 0 {
		view.HoverFile = f.OverFile
	}
	if f.OverDevice >= 0 {
		view.HoverDevice = f.OverDevice
	}

	switch {
	case m.holding():
		if math.Hypot(m.x-m.grabX, m.y-m.grabY) >= dragDistance && m.phase == PhaseGrabbed {
			m.phase = PhaseDragging
		}
		if m.phase == PhaseDragging && speed >= throwSpeed && toward > 0.01 {
			m.phase = PhaseThrowing
			m.throwing = 0
		}
		if m.phase == PhaseThrowing {
			m.throwing++
		}
		if pinchUp || (m.phase == PhaseThrowing && f.OverPortal && m.throwing >= 2) {
			m.release(f, &view)
		}
	default:
		if pinchDown && f.OverFile >= 0 {
			m.held = f.OverFile
			m.phase = PhaseGrabbed
			m.grabX, m.grabY = m.x, m.y
		} else if pinchDown && f.OverDevice >= 0 {
			m.dest = f.OverDevice
			m.phase = PhaseDestination
			view.Message = "Device selected"
		} else if m.pinchOn > 0 && m.pinchOn < pinchHold && (f.OverFile >= 0 || f.OverDevice >= 0) {
			m.phase = PhasePinching
			view.Message = "Pinch"
		} else if m.phase != PhaseDestination && (f.OverFile >= 0 || f.OverDevice >= 0) {
			m.phase = PhaseHovering
		} else if m.phase == PhaseHovering || m.phase == PhasePinching {
			m.phase = PhaseIdle
		}
	}
	return m.fill(view)
}

func (m *Machine) scrollTray(f Frame, view *View) bool {
	pinching := f.Pinch >= 0 && f.Pinch < pinchClose
	if m.holding() || !f.OverTray || pinching {
		m.scrollOn = 0
		return false
	}
	edge := m.limits.ScrollEdge
	if edge <= 0 {
		edge = 0.22
	}
	if m.phase == PhaseScrolling {
		edge += m.limits.ScrollDead
	}
	var dir float64
	switch {
	case f.TrayY <= edge:
		dir = -1
	case f.TrayY >= 1-edge:
		dir = 1
	default:
		m.scrollOn = 0
		return false
	}
	m.scrollOn++
	hold := m.limits.ScrollHold
	if hold < 1 {
		hold = 1
	}
	if m.scrollOn < hold {
		return false
	}
	m.phase = PhaseScrolling
	step := m.limits.ScrollStep
	if step == 0 {
		step = 18
	}
	view.Scroll = dir * step
	view.Message = "Scrolling"
	m.clearPath()
	return true
}

func (m *Machine) lostLimit() int {
	if m.phase == PhaseDrawing || len(m.path) > 0 {
		return lostHandFrames * 3
	}
	return lostHandFrames
}

func (m *Machine) pauseAfter() time.Duration {
	if m.limits.PauseAfter > 0 {
		return m.limits.PauseAfter
	}
	return 400 * time.Millisecond
}

func (m *Machine) lostFor() time.Duration {
	if m.limits.LostFor > 0 {
		return m.limits.LostFor
	}
	return 2500 * time.Millisecond
}

// missedStroke keeps a circle alive across a short tracking gap. The stroke
// pauses only after PauseAfter, and it is cancelled only after LostFor.
func (m *Machine) missedStroke(view View) View {
	now := m.clock()
	if m.lostAt.IsZero() {
		m.lostAt = now
	}
	gap := now.Sub(m.lostAt)
	m.breakNext = true
	if gap >= m.lostFor() {
		m.clearPath()
		m.reason = "reset: lost"
		m.phase = PhaseReady
		view.Message = "Start drawing"
		return m.fill(view)
	}
	if gap >= m.pauseAfter() {
		m.phase = PhasePaused
		m.reason = "tracking paused"
		m.fx.reset()
		m.fy.reset()
	} else if m.started {
		m.phase = PhaseDrawing
		m.reason = "tracking"
	}
	return m.fill(view)
}

func (m *Machine) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

func (m *Machine) draw(f Frame, view View) View {
	if !m.coolUntil.IsZero() && m.clock().Before(m.coolUntil) {
		view.Message = "Portal opening…"
		return m.fill(view)
	}
	// A pinch selects files only after the portal is open. While it is
	// closed, thumb-to-index distance must not block the stroke.
	added := m.note(f.X, f.Y)
	if added {
		m.moved = m.clock()
	}
	if m.started {
		m.phase = PhaseDrawing
	} else {
		m.phase = PhaseReady
	}
	stall := m.limits.Stall
	if stall <= 0 {
		stall = 6 * time.Second
	}
	quiet := !m.moved.IsZero() && !added && m.clock().Sub(m.moved) > stall
	aged := m.started && !m.begun.IsZero() && m.clock().Sub(m.begun) > 20*time.Second
	if quiet || aged || len(m.path) > m.limits.MaxPoints {
		m.clearPath()
		m.reason = "reset: stall"
		m.phase = PhaseReady
		view.Message = "Start drawing"
		return m.fill(view)
	}
	if m.circle() {
		m.open = true
		m.clearPath()
		m.reason = "reset: opened"
		m.phase = PhaseComplete
		cool := m.limits.Cooldown
		if cool <= 0 {
			cool = 1500 * time.Millisecond
		}
		m.coolUntil = m.clock().Add(cool)
		view.Recognized = true
		view.Message = "Portal opening…"
	}
	return m.fill(view)
}

func (m *Machine) note(x, y float64) bool {
	now := m.clock()
	if !m.anchored {
		m.anchored = true
		m.anchorX, m.anchorY = x, y
		m.accept(x, y, now)
		m.reason = ""
		return false
	}
	if m.breakNext {
		m.breakNext = false
		far := m.haveRaw && math.Hypot(x-m.rawX, y-m.rawY) > maxJump
		if far {
			m.fx.reset()
			m.fy.reset()
		}
		if far && m.guideOn && !m.tolerant(x, y) {
			m.rawX, m.rawY = x, y
			m.haveRaw = true
			m.pendingCut = true
			m.reason = "jump"
			return false
		}
		if far && !m.guideOn && len(m.path) < 8 && !m.started {
			m.clearPath()
			return m.note(x, y)
		}
		sx, sy := m.accept(x, y, now)
		if !m.started {
			return false
		}
		cut := true
		m.pendingCut = false
		if len(m.trail) > 0 {
			last := m.trail[len(m.trail)-1]
			if math.Hypot(sx-last.X, sy-last.Y) <= resumeJoin {
				cut = false
			}
		}
		added := m.advance(sx, sy, cut)
		m.reason = "resuming"
		return added
	}
	if m.haveRaw && math.Hypot(x-m.rawX, y-m.rawY) > maxJump {
		if !m.guideOn && len(m.path) < 8 {
			m.clearPath()
			return m.note(x, y)
		}
		m.fx.reset()
		m.fy.reset()
		m.rawX, m.rawY = x, y
		m.haveRaw = true
		m.pendingCut = true
		if !m.guideOn || !m.tolerant(x, y) {
			m.reason = "jump"
			return false
		}
		sx, sy := m.accept(x, y, now)
		return m.advance(sx, sy, true)
	}
	if m.guideOn && m.abrupt(x, y) {
		m.rawX, m.rawY = x, y
		m.haveRaw = true
		m.pendingCut = true
		return false
	}
	sx, sy := m.accept(x, y, now)
	if !m.started {
		if math.Hypot(x-m.anchorX, y-m.anchorY) < startMove {
			m.reason = "waiting"
			return false
		}
		m.started = true
		m.begun = now
	}
	return m.advance(sx, sy, false)
}

func (m *Machine) accept(x, y float64, now time.Time) (float64, float64) {
	m.rawX, m.rawY = x, y
	m.haveRaw = true
	m.drawX = m.fx.filter(x, now)
	m.drawY = m.fy.filter(y, now)
	m.drawReady = true
	m.x, m.y = m.drawX, m.drawY
	return m.drawX, m.drawY
}

func (m *Machine) appendStroke(x, y float64, cut bool) bool {
	if !m.addPoint(x, y) {
		return false
	}
	if cut {
		m.path[len(m.path)-1].Cut = true
	}
	// The stroke is the fingertip path. Circle fitting only measures it.
	m.trail = append(m.trail, m.path[len(m.path)-1])
	return true
}

func (m *Machine) advance(x, y float64, forceCut bool) bool {
	if !m.guideOn {
		m.reason = ""
		cut := forceCut || m.pendingCut
		m.pendingCut = false
		return m.appendStroke(x, y, cut)
	}
	m.adapt(x, y)
	ang := math.Atan2(y-m.guideCY, x-m.guideCX)
	var d float64
	if m.haveAng {
		d = unwrap(ang - m.lastAng)
	} else {
		m.arc0 = ang
	}
	m.haveAng = true
	m.lastAng = ang
	if math.Abs(d) < 0.012 && len(m.trail) > 0 {
		m.reason = ""
		return false
	}
	m.reason = ""
	cut := forceCut || m.pendingCut
	m.pendingCut = false
	added := m.appendStroke(x, y, cut)
	if added || math.Abs(d) >= 0.012 {
		m.addSweep(d, cut)
	}
	return added
}

// addSweep keeps the angle already earned. A brief reversal or a tracking
// gap does not subtract progress or demand another revolution.
func (m *Machine) addSweep(d float64, cut bool) {
	if cut || math.Abs(d) > 1.05 {
		if m.reason == "" {
			m.reason = "gap"
		}
		return
	}
	if m.dir == 0 {
		m.guideSweep += d
		switch {
		case m.guideSweep > 0.45:
			m.dir = 1
		case m.guideSweep < -0.45:
			m.dir = -1
		}
		return
	}
	if d*float64(m.dir) > 0 {
		m.guideSweep += d
		return
	}
	if math.Abs(d) > 0.08 && m.reason == "" {
		m.reason = "reverse"
	}
}

func (m *Machine) tolerant(x, y float64) bool {
	if !m.guideOn || m.guideR < 1e-4 {
		return false
	}
	band := m.limits.Band
	if band <= 0 {
		band = 0.45
	}
	d := math.Hypot(x-m.guideCX, y-m.guideCY)
	return math.Abs(d-m.guideR) <= m.guideR*band
}

// abrupt is a sample that leaves the circle in one step. A smooth change in
// radius is not abrupt, so the gesture keeps moving and the ring adapts.
func (m *Machine) abrupt(x, y float64) bool {
	if !m.guideOn || m.tolerant(x, y) || !m.haveRaw {
		return false
	}
	limit := m.guideR * 0.5
	if limit < 0.09 {
		limit = 0.09
	}
	return math.Hypot(x-m.rawX, y-m.rawY) > limit
}

func (m *Machine) adapt(x, y float64) {
	if !m.guideOn {
		return
	}
	d := math.Hypot(x-m.guideCX, y-m.guideCY)
	if d < 1e-4 {
		return
	}
	// Follow a changing radius slowly. The center stays locked, and the
	// radius cannot jump to a much larger circle in one drawing.
	m.guideR += 0.08 * (d - m.guideR)
	lim := m.limits
	floor := lim.MinRadius * 0.55
	if floor < 0.05 {
		floor = 0.05
	}
	lo, hi := floor, lim.MaxRadius
	if m.candR > 0 {
		if m.candR*0.65 > lo {
			lo = m.candR * 0.65
		}
		hi = m.candR * 1.5
		if lim.MaxRadius > 0 && hi > lim.MaxRadius {
			hi = lim.MaxRadius
		}
	}
	if m.guideR < lo {
		m.guideR = lo
	}
	if hi > 0 && m.guideR > hi {
		m.guideR = hi
	}
}

func (m *Machine) inBand(x, y float64) bool {
	return m.tolerant(x, y)
}

func unwrap(d float64) float64 {
	for d > math.Pi {
		d -= 2 * math.Pi
	}
	for d < -math.Pi {
		d += 2 * math.Pi
	}
	return d
}

// oneEuro is an adaptive low-pass filter. It smooths a still fingertip and
// follows a moving one with little lag.
type oneEuro struct {
	minCutoff float64
	beta      float64
	dCutoff   float64
	x         float64
	dx        float64
	ready     bool
	last      time.Time
}

func newOneEuro() oneEuro {
	return oneEuro{minCutoff: 1.4, beta: 10, dCutoff: 1}
}

func (e *oneEuro) reset() {
	e.ready = false
	e.dx = 0
}

func (e *oneEuro) filter(v float64, now time.Time) float64 {
	if !e.ready {
		e.ready = true
		e.x = v
		e.dx = 0
		e.last = now
		return v
	}
	dt := now.Sub(e.last).Seconds()
	if dt < 1.0/120 {
		dt = 1.0 / 30
	} else if dt > 0.25 {
		dt = 0.25
	}
	e.last = now
	freq := 1 / dt
	deriv := (v - e.x) * freq
	e.dx += euroAlpha(freq, e.dCutoff) * (deriv - e.dx)
	cutoff := e.minCutoff + e.beta*math.Abs(e.dx)
	e.x += euroAlpha(freq, cutoff) * (v - e.x)
	return e.x
}

func euroAlpha(freq, cutoff float64) float64 {
	if cutoff < 1e-3 {
		cutoff = 1e-3
	}
	tau := 1 / (2 * math.Pi * cutoff)
	return 1 / (1 + tau*freq)
}

func (m *Machine) addPoint(x, y float64) bool {
	if len(m.path) > 0 && math.Hypot(x-m.path[len(m.path)-1].X, y-m.path[len(m.path)-1].Y) < pointGap {
		return false
	}
	m.path = append(m.path, Point{X: x, Y: y})
	return true
}

func (m *Machine) circle() bool {
	m.trackGuide()
	if !m.guideOn {
		return false
	}
	if len(m.trail) < 8 {
		return false
	}
	return math.Abs(m.guideSweep) >= m.limits.MinSweep
}

func (m *Machine) trackGuide() {
	pts := m.path
	if len(pts) < 8 {
		return
	}
	cx, cy, r, err, ok := fitCenter(pts)
	lim := m.limits
	if !m.guideOn {
		floor := lim.MinRadius * 0.7
		if floor <= 0 {
			floor = 0.08
		}
		if !ok || r < floor || r > lim.MaxRadius || err > lim.RadiusCV || !circleFits(pts, cx, cy, r) {
			m.seenCenter = false
			m.stable = 0
			if m.reason == "" {
				if r > lim.MaxRadius || (ok && !circleFits(pts, cx, cy, r)) {
					m.reason = "radius"
				} else {
					m.reason = "center"
				}
			}
			return
		}
		sweep := sweepAround(pts, cx, cy)
		if math.Abs(sweep) < 0.75 {
			if m.reason == "" {
				m.reason = "center"
			}
			return
		}
		m.candX, m.candY, m.candR = cx, cy, r
		m.seenCenter = true
		m.guideOn = true
		m.guideCX, m.guideCY, m.guideR = cx, cy, r
		m.guideErr = err
		m.armAngle()
		return
	}
}

func (m *Machine) armAngle() {
	m.guideSweep = 0
	m.haveAng = false
	m.dir = 0
	if len(m.trail) == 0 {
		return
	}
	m.guideSweep = sweepAround(m.trail, m.guideCX, m.guideCY)
	first := m.trail[0]
	last := m.trail[len(m.trail)-1]
	m.arc0 = math.Atan2(first.Y-m.guideCY, first.X-m.guideCX)
	m.lastAng = math.Atan2(last.Y-m.guideCY, last.X-m.guideCX)
	m.haveAng = true
	switch {
	case m.guideSweep > 0.2:
		m.dir = 1
	case m.guideSweep < -0.2:
		m.dir = -1
	}
}

func fitCenter(pts []Point) (cx, cy, r, err float64, ok bool) {
	n := len(pts)
	triples := [][3]int{{0, n / 3, n - 1}, {0, n / 2, n - 1}, {n / 5, n / 2, (4 * n) / 5}}
	var xs, ys, rs []float64
	for _, t := range triples {
		if t[1] <= t[0] || t[2] <= t[1] || t[2] >= n {
			continue
		}
		x, y, rad, fit := circumcenter(pts[t[0]], pts[t[1]], pts[t[2]])
		if !fit || rad < 0.05 || rad > 0.7 {
			continue
		}
		xs = append(xs, x)
		ys = append(ys, y)
		rs = append(rs, rad)
	}
	if len(xs) > 0 {
		cx, cy, r = median(xs), median(ys), median(rs)
		return cx, cy, r, radiusError(pts, cx, cy, r), true
	}
	var sx, sy, rad float64
	for _, p := range pts {
		sx += p.X
		sy += p.Y
	}
	cx, cy = sx/float64(n), sy/float64(n)
	for _, p := range pts {
		rad += math.Hypot(p.X-cx, p.Y-cy)
	}
	rad /= float64(n)
	if rad <= 1e-4 {
		return 0, 0, 0, 1, false
	}
	return cx, cy, rad, radiusError(pts, cx, cy, rad), true
}

// circleFits rejects a center that sits on the stroke or a radius so large
// that the arc would leave the drawing area.
func circleFits(pts []Point, cx, cy, r float64) bool {
	if cx < 0 || cy < 0 || cx > 1 || cy > 1 || r <= 0 {
		return false
	}
	if cx-r < -0.08 || cx+r > 1.08 || cy-r < -0.08 || cy+r > 1.08 {
		return false
	}
	if !centerClear(pts, cx, cy, r) {
		return false
	}
	var span float64
	for i := range pts {
		for j := i + 1; j < len(pts); j++ {
			d := math.Hypot(pts[i].X-pts[j].X, pts[i].Y-pts[j].Y)
			if d > span {
				span = d
			}
		}
	}
	return span <= 0 || r <= span*1.8
}

func centerClear(pts []Point, cx, cy, r float64) bool {
	if r < 1e-3 {
		return false
	}
	limit := r * 0.5
	for _, p := range pts {
		if math.Hypot(p.X-cx, p.Y-cy) < limit {
			return false
		}
	}
	return true
}

func median(vs []float64) float64 {
	s := append([]float64(nil), vs...)
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && s[j] < s[j-1] {
			s[j], s[j-1] = s[j-1], s[j]
			j--
		}
	}
	return s[len(s)/2]
}

func circumcenter(a, b, c Point) (float64, float64, float64, bool) {
	d := 2 * (a.X*(b.Y-c.Y) + b.X*(c.Y-a.Y) + c.X*(a.Y-b.Y))
	if math.Abs(d) < 1e-6 {
		return 0, 0, 0, false
	}
	ux := ((a.X*a.X+a.Y*a.Y)*(b.Y-c.Y) + (b.X*b.X+b.Y*b.Y)*(c.Y-a.Y) + (c.X*c.X+c.Y*c.Y)*(a.Y-b.Y)) / d
	uy := ((a.X*a.X+a.Y*a.Y)*(c.X-b.X) + (b.X*b.X+b.Y*b.Y)*(a.X-c.X) + (c.X*c.X+c.Y*c.Y)*(b.X-a.X)) / d
	return ux, uy, math.Hypot(a.X-ux, a.Y-uy), true
}

func radiusError(pts []Point, cx, cy, r float64) float64 {
	if r < 1e-4 || len(pts) == 0 {
		return 1
	}
	var sum float64
	for _, p := range pts {
		d := math.Hypot(p.X-cx, p.Y-cy) - r
		sum += d * d
	}
	return math.Sqrt(sum/float64(len(pts))) / r
}

func sweepAround(pts []Point, cx, cy float64) float64 {
	if len(pts) < 2 {
		return 0
	}
	prev := math.Atan2(pts[0].Y-cy, pts[0].X-cx)
	var sweep float64
	for i := 1; i < len(pts); i++ {
		ang := math.Atan2(pts[i].Y-cy, pts[i].X-cx)
		d := ang - prev
		for d > math.Pi {
			d -= 2 * math.Pi
		}
		for d < -math.Pi {
			d += 2 * math.Pi
		}
		if pts[i].Cut || math.Abs(d) > 1.2 {
			prev = ang
			continue
		}
		sweep += d
		prev = ang
	}
	return sweep
}

func (m *Machine) clearPath() {
	m.path = nil
	m.trail = nil
	m.anchored = false
	m.started = false
	m.strokeDead = false
	m.stall = 0
	m.moved = time.Time{}
	m.drawReady = false
	m.haveRaw = false
	m.breakNext = false
	m.lostAt = time.Time{}
	m.offBand = 0
	m.pendingCut = false
	m.reason = ""
	m.begun = time.Time{}
	m.arc0 = 0
	m.lastAng = 0
	m.haveAng = false
	m.dir = 0
	m.back = 0
	m.fx.reset()
	m.fy.reset()
	m.guideOn = false
	m.guideCX, m.guideCY, m.guideR, m.guideSweep, m.guideErr = 0, 0, 0, 0, 0
	m.candX, m.candY, m.candR = 0, 0, 0
	m.seenCenter = false
	m.stable = 0
}

func (m *Machine) holding() bool {
	return m.held >= 0 && (m.phase == PhaseGrabbed || m.phase == PhaseDragging || m.phase == PhaseThrowing)
}

func (m *Machine) release(f Frame, view *View) {
	launched := f.OverPortal || m.phase == PhaseThrowing
	if m.dest >= 0 && launched {
		m.phase = PhaseTransferring
		m.sent = true
		view.Send = true
		view.Message = "Sending"
		return
	}
	if m.dest < 0 {
		view.Message = "Select a device first"
	} else {
		view.Message = "Release into the portal"
	}
	next := PhaseIdle
	if m.dest >= 0 {
		next = PhaseDestination
	}
	m.drop(next)
}

func (m *Machine) drop(phase string) {
	m.held = -1
	m.throwing = 0
	m.phase = phase
}

func (m *Machine) fill(view View) View {
	view.Phase = m.phase
	view.X, view.Y = m.x, m.y
	view.Held = m.held
	view.Destination = m.dest
	view.Open = m.open
	view.Tracked = m.tracked
	view.Confidence = m.conf
	view.Reason = m.reason
	if !m.lostAt.IsZero() {
		view.LostMs = int(m.clock().Sub(m.lostAt).Milliseconds())
	}
	if len(m.trail) > 0 {
		view.Trail = append([]Point(nil), m.trail...)
	}
	if m.guideOn && m.limits.MinSweep > 0 {
		view.RingX, view.RingY, view.RingR = m.guideCX, m.guideCY, m.guideR
		view.Sweep = m.guideSweep
		view.Arc = m.arc0
		view.Degrees = math.Abs(m.guideSweep) * 180 / math.Pi
		view.Progress = math.Min(1, math.Abs(m.guideSweep)/m.limits.MinSweep)
	}
	if view.Message == "" || view.Message == "Start drawing" {
		view.Message = m.drawMessage()
	}
	switch {
	case view.Recognized || m.phase == PhaseComplete:
		view.DrawState = PhaseComplete
	case m.phase == PhasePaused:
		view.DrawState = PhasePaused
	case m.started || m.phase == PhaseDrawing:
		view.DrawState = PhaseDrawing
	default:
		view.DrawState = "IDLE"
	}
	return view
}

func (m *Machine) drawMessage() string {
	if m.open {
		return ""
	}
	switch m.reason {
	case "tracking":
		return "Tracking hand…"
	case "resuming":
		return "Resuming…"
	}
	if m.phase == PhasePaused {
		return "Continue drawing"
	}
	if m.phase == PhaseDrawing && m.guideOn && m.limits.MinSweep > 0 {
		pct := int(math.Round(math.Min(1, math.Abs(m.guideSweep)/m.limits.MinSweep) * 100))
		if pct >= 8 {
			return fmt.Sprintf("Circle %d%% complete", pct)
		}
	}
	if m.phase == PhaseDrawing {
		return "Drawing"
	}
	return "Start drawing"
}
