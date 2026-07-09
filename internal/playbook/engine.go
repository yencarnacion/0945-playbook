package playbook

import (
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"0945-playbook/internal/data"
	"0945-playbook/internal/watchlist"
)

type SparkPoint struct {
	Time  string  `json:"time"`
	Close float64 `json:"close"`
}

type Evaluation struct {
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Industry string `json:"industry"`
	Order    int    `json:"order"`

	Status string `json:"status"`
	Action string `json:"action"`
	Branch string `json:"branch"`
	Reason string `json:"reason"`
	Phase  string `json:"phase"`
	Side   int    `json:"side"`

	Clock              string  `json:"clock"`
	Price              float64 `json:"price"`
	Avg15              float64 `json:"avg15"`
	Ratio              float64 `json:"ratio"`
	DeltaPct           float64 `json:"delta_pct"`
	First15Bars        int     `json:"first15_bars"`
	First15Complete    bool    `json:"first15_complete"`
	First15Vol         float64 `json:"first15_vol"`
	First15DollarVol   float64 `json:"first15_dollar_vol"`
	VWAP               float64 `json:"vwap"`
	VWAPRewardPct      float64 `json:"vwap_reward_pct"`
	DayHigh            float64 `json:"day_high"`
	DayLow             float64 `json:"day_low"`
	HODRiskPct         float64 `json:"hod_risk_pct"`
	TargetStopDistance float64 `json:"target_stop_distance"`
	DistancePct        float64 `json:"distance_pct"`
	ORBSource          string  `json:"orb_source"`
	PriorORBCount      int     `json:"prior_orb_count"`
	LookbackOK         bool    `json:"lookback_ok"`

	Entry          float64 `json:"entry"`
	Target         float64 `json:"target"`
	Stop           float64 `json:"stop"`
	RiskPerShare   float64 `json:"risk_per_share"`
	RewardPerShare float64 `json:"reward_per_share"`
	RMultiple      float64 `json:"r_multiple"`
	Shares         int     `json:"shares"`
	EVScore        float64 `json:"ev_score"`

	Candidate bool `json:"candidate"`
	Signal    bool `json:"signal"`
	Active    bool `json:"active"`
	Exited    bool `json:"exited"`

	ChartURL    string       `json:"chart_url"`
	LastUpdated string       `json:"last_updated"`
	Spark       []SparkPoint `json:"spark"`
	Error       string       `json:"error"`
}

type tradeState struct {
	active         bool
	taken          bool
	side           int
	entryBarIndex  int
	branch         string
	entry          float64
	target         float64
	stop           float64
	riskPerShare   float64
	rewardPerShare float64
	rMultiple      float64
	shares         int
	signalRatio    float64
	lastStatus     string
	lastAction     string
	lastReason     string
	exited         bool
	exitWasProfit  bool
}

func Evaluate(item watchlist.Item, bars []data.Bar, now time.Time, loc *time.Location, s Settings, priorORBRanges []float64) Evaluation {
	sort.Slice(bars, func(i, j int) bool { return bars[i].Time.Before(bars[j].Time) })

	if loc == nil {
		loc = time.Local
	}
	if now.IsZero() {
		if len(bars) > 0 {
			now = bars[len(bars)-1].Time
		} else {
			now = time.Now()
		}
	}
	now = now.In(loc)
	s.ChartTime = chartClock(now)

	ev := Evaluation{
		Symbol:   item.Symbol,
		Name:     item.Name,
		Industry: item.Industry,
		Order:    item.Order,
		Status:   "WAIT 09:45",
		Action:   "WAIT",
		Branch:   "-",
		Phase:    "wait",
		ChartURL: chartURL(s.ChartBaseURL, item.Symbol, s.ChartDate, s.ChartTime, "buy"),
	}

	if len(bars) == 0 {
		ev.Status = "NO DATA"
		ev.Action = "NONE"
		ev.Phase = "error"
		ev.Reason = "No bars loaded."
		return finalize(ev, s)
	}
	ev.LastUpdated = now.Format(time.RFC3339)

	open := sessionTime(now, loc, s.SessionOpen)
	closeTime := sessionTime(now, loc, s.SessionClose)

	var dayHigh float64
	var dayLow float64
	var first15CloseSum float64
	var first15Vol float64
	var first15DollarVol float64
	var first15High float64
	var first15Low float64
	var first15BarCount int
	var rollingCloseSum float64
	var rollingCloses []float64
	var cumPV float64
	var cumVol float64
	var orbHigh float64
	var orbLow float64
	var lastBar data.Bar
	var lastMinutesAfterOpen int
	var lastInRTH bool
	state := tradeState{lastStatus: "WAIT 09:45", lastAction: "WAIT"}

	for idx, bar := range bars {
		bt := bar.Time.In(loc)
		if bt.After(now) {
			break
		}
		if bt.Before(open) || !bt.Before(closeTime) {
			continue
		}
		minutesAfterOpen := int(bt.Sub(open) / time.Minute)
		if minutesAfterOpen < 0 {
			continue
		}

		lastBar = bar
		lastMinutesAfterOpen = minutesAfterOpen
		lastInRTH = true

		rollingCloses = append(rollingCloses, bar.Close)
		rollingCloseSum += bar.Close
		if len(rollingCloses) > s.AvgCloseBars {
			rollingCloseSum -= rollingCloses[0]
			rollingCloses = rollingCloses[1:]
		}

		if dayHigh == 0 || bar.High > dayHigh {
			dayHigh = bar.High
		}
		if dayLow == 0 || bar.Low < dayLow {
			dayLow = bar.Low
		}

		barVWAP := bar.VWAP
		if barVWAP <= 0 {
			barVWAP = (bar.High + bar.Low + bar.Close) / 3
		}
		cumPV += barVWAP * bar.Volume
		cumVol += bar.Volume

		if minutesAfterOpen < s.AvgCloseBars {
			first15CloseSum += bar.Close
			first15BarCount++
			first15Vol += bar.Volume
			first15DollarVol += bar.Close * bar.Volume
			if first15High == 0 || bar.High > first15High {
				first15High = bar.High
			}
			if first15Low == 0 || bar.Low < first15Low {
				first15Low = bar.Low
			}
		}

		if minutesAfterOpen < s.OrbMinutes {
			if orbHigh == 0 || bar.High > orbHigh {
				orbHigh = bar.High
			}
			if orbLow == 0 || bar.Low < orbLow {
				orbLow = bar.Low
			}
		}

		if minutesAfterOpen >= s.EntryMinutesAfterOpen && !state.taken && !state.active {
			rollingAvgClose := safeDiv(rollingCloseSum, float64(len(rollingCloses)))
			evaluateEntry(&state, bar, idx, rollingAvgClose, first15BarCount, first15Vol, first15DollarVol, dayHigh, cumPV, cumVol, first15High, first15Low, s, priorORBRanges)
		}

		if state.active && idx > state.entryBarIndex {
			stopHit := (state.side == 1 && bar.Low <= state.stop) || (state.side == -1 && bar.High >= state.stop)
			targetHit := (state.side == 1 && bar.High >= state.target) || (state.side == -1 && bar.Low <= state.target)
			if stopHit {
				state.active = false
				state.exited = true
				state.exitWasProfit = false
				state.lastStatus = "STOPPED"
				state.lastAction = "STOP"
				state.lastReason = "Stop touched. Same-minute conflict = stop."
			} else if targetHit {
				state.active = false
				state.exited = true
				state.exitWasProfit = true
				state.lastStatus = "TAKE PROFIT"
				state.lastAction = "TP"
				state.lastReason = "Target touched."
			} else if s.UseTimeExit && inWindow(bt, s.TimeExitWindow) {
				state.active = false
				state.exited = true
				state.exitWasProfit = false
				state.lastStatus = "TIME EXIT"
				state.lastAction = "FLAT"
				state.lastReason = "No TP/stop before time exit."
			}
		}
	}

	if !lastInRTH {
		ev.Status = "OUTSIDE RTH"
		ev.Action = "WAIT"
		ev.Phase = "wait"
		ev.Reason = "No regular-session bars at this clock."
		return finalize(ev, s)
	}

	first15Complete := first15BarCount >= s.AvgCloseBars
	avg15 := safeDiv(rollingCloseSum, float64(len(rollingCloses)))

	targetStopDistance, orbSource, lookbackOK := targetStopDistance(first15Complete, first15High, first15Low, s, priorORBRanges)
	price := lastBar.Close
	ratio := safeDiv(price, avg15)
	deltaPct := ratio - 1
	vwap := safeDiv(cumPV, cumVol)
	vwapRewardPct := safeDiv(price-vwap, price)
	hodRiskPct := safeDiv(dayHigh-price, price)
	distancePct := safeDiv(targetStopDistance, price)

	ev.Price = price
	ev.Avg15 = avg15
	ev.Ratio = ratio
	ev.DeltaPct = deltaPct
	ev.First15Bars = first15BarCount
	ev.First15Complete = first15Complete
	ev.First15Vol = first15Vol
	ev.First15DollarVol = first15DollarVol
	ev.VWAP = vwap
	ev.VWAPRewardPct = vwapRewardPct
	ev.DayHigh = dayHigh
	ev.DayLow = dayLow
	ev.HODRiskPct = hodRiskPct
	ev.TargetStopDistance = targetStopDistance
	ev.DistancePct = distancePct
	ev.ORBSource = orbSource
	ev.PriorORBCount = len(priorORBRanges)
	ev.LookbackOK = lookbackOK
	ev.Clock = fmt.Sprintf("+%dm", lastMinutesAfterOpen)
	ev.Spark = sparkPoints(bars, now, loc, 48)

	if state.taken {
		ev.Signal = true
		ev.Side = state.side
		ev.Branch = state.branch
		ev.Entry = state.entry
		ev.Target = state.target
		ev.Stop = state.stop
		ev.RiskPerShare = state.riskPerShare
		ev.RewardPerShare = state.rewardPerShare
		ev.RMultiple = state.rMultiple
		ev.Shares = state.shares
		ev.Active = state.active
		ev.Exited = state.exited
		ev.Reason = state.lastReason
		ev.Action = state.lastAction
		ev.Status = state.lastStatus
		if state.active {
			ev.Status = "ACTIVE " + sideName(state.side)
			ev.Action = holdAction(state.side)
			ev.Phase = "active"
		} else if state.exited {
			ev.Phase = "done"
		} else {
			ev.Phase = "signal"
		}
		ev.EVScore = signalScore(ev)
		return finalize(ev, s)
	}

	if lastMinutesAfterOpen < s.EntryMinutesAfterOpen {
		ev.Candidate = ratio >= s.UpperSignalRatio || ratio <= s.LowerSignalRatio
		if ev.Candidate {
			applyCandidateEstimate(&ev, price, ratio, vwap, dayHigh, first15High, first15Low, first15Complete, priorORBRanges, s)
			ev.Phase = "likely"
			ev.Reason = "Estimated from live pre-09:45 data; final branch, shares, target, and stop recalc at 09:45."
			ev.EVScore = candidateScore(ev, s)
		} else {
			ev.Status = "WAIT 09:45"
			ev.Action = "WAIT"
			ev.Phase = "wait"
			ev.Reason = "Building first-15 stats."
		}
		return finalize(ev, s)
	}

	ev.Status = state.lastStatus
	ev.Action = state.lastAction
	ev.Reason = state.lastReason
	if ev.Status == "" || ev.Status == "WAIT 09:45" {
		ev.Status = "NO TRADE"
		ev.Action = "NONE"
		ev.Phase = "none"
		ev.Reason = noTradeReason(first15Complete, lookbackOK, ratio, price, first15Vol, first15DollarVol, s)
	}
	return finalize(ev, s)
}

func applyCandidateEstimate(ev *Evaluation, price float64, signalRatio float64, rthVWAP float64, dayHigh float64, first15High float64, first15Low float64, first15Complete bool, priorORBRanges []float64, s Settings) {
	distance, source, ok := targetStopDistanceEstimate(first15Complete, first15High, first15Low, s, priorORBRanges)
	if distance <= 0 {
		distance = math.Max(price*0.02, s.MinRiskPerShare)
		source = "2% live estimate"
		ok = true
	}
	if distance > 0 {
		ev.TargetStopDistance = distance
		ev.DistancePct = safeDiv(distance, price)
		ev.ORBSource = source
		ev.LookbackOK = ok
	}

	branch, side, target, stop := estimateBranch(price, signalRatio, rthVWAP, dayHigh, distance, ev.DistancePct, ev.VWAPRewardPct, ev.HODRiskPct, s)
	ev.Side = side
	ev.Branch = branch
	ev.Entry = price
	ev.Target = target
	ev.Stop = stop
	ev.Status = "LIKELY BUY"
	ev.Action = "EST BUY"
	if side < 0 {
		ev.Status = "LIKELY SELL"
		ev.Action = "EST SELL SHORT"
	}

	if target <= 0 || stop <= 0 || side == 0 {
		return
	}

	riskPerShare := price - stop
	rewardPerShare := target - price
	if side == -1 {
		riskPerShare = stop - price
		rewardPerShare = price - target
	}
	ev.RiskPerShare = riskPerShare
	ev.RewardPerShare = rewardPerShare
	ev.RMultiple = safeDiv(rewardPerShare, riskPerShare)
	ev.Shares = estimateShares(riskPerShare, s)
}

func estimateBranch(price float64, signalRatio float64, rthVWAP float64, dayHigh float64, distance float64, distancePct float64, vwapRewardPct float64, hodRiskPct float64, s Settings) (string, int, float64, float64) {
	upperSignal := signalRatio >= s.UpperSignalRatio
	lowerSignal := signalRatio <= s.LowerSignalRatio
	entry := price

	branch1 := signalRatio >= s.ModUpperMinSignal &&
		signalRatio <= s.ModUpperMaxSignal &&
		vwapRewardPct >= s.ModUpperMinVWAPRewardPct &&
		hodRiskPct >= s.ModUpperMinHODRiskPct &&
		hodRiskPct <= s.ModUpperMaxHODRiskPct
	branch2 := signalRatio >= s.ExtremeUpperMinSignal &&
		price >= s.ExtremeUpperMinPrice &&
		price <= s.ExtremeUpperMaxPrice
	branch3 := upperSignal &&
		price >= s.MidLongMinPrice &&
		price <= s.MidLongMaxPrice &&
		distancePct >= s.MidLongMinDistancePct &&
		distancePct <= s.MidLongMaxDistancePct &&
		signalRatio < s.MidLongMaxSignal
	branch4 := lowerSignal &&
		distancePct >= s.LowerShortMinTargetPct &&
		distancePct <= s.LowerShortMaxTargetPct &&
		hodRiskPct >= s.LowerShortMinHODRiskPct &&
		hodRiskPct <= s.LowerShortMaxHODRiskPct
	branch5 := lowerSignal &&
		distancePct >= s.LowerORBMinDistancePct &&
		distancePct <= s.LowerORBMaxDistancePct

	switch {
	case branch1:
		return "EST B1 VWAP FADE", -1, rthVWAP, maxPositive(dayHigh, entry+distance)
	case branch2:
		return "EST B2 EXT FADE", -1, entry - distance, entry + distance
	case branch3:
		return "EST B3 LONG", 1, entry + distance, entry - distance
	case branch4:
		branch := "EST B4 SHORT"
		if signalRatio <= s.LowerPrioritySignalRatio {
			branch = "EST B4 SHORT *"
		}
		return branch, -1, entry - distance, maxPositive(dayHigh, entry+distance)
	case branch5:
		return "EST B5 1:1 SHORT", -1, entry - distance, entry + distance
	}

	if upperSignal {
		if signalRatio >= s.ExtremeUpperMinSignal || rthVWAP > 0 && entry > rthVWAP {
			target := entry - distance
			if rthVWAP > 0 && rthVWAP < entry {
				target = rthVWAP
			}
			return "EST UPPER FADE", -1, target, maxPositive(dayHigh, entry+distance)
		}
		return "EST UPPER LONG", 1, entry + distance, entry - distance
	}
	if lowerSignal {
		return "EST LOWER SHORT", -1, entry - distance, maxPositive(dayHigh, entry+distance)
	}
	return "EST WATCH", 0, 0, 0
}

func evaluateEntry(state *tradeState, bar data.Bar, idx int, avg15 float64, first15BarCount int, first15Vol float64, first15DollarVol float64, dayHigh float64, cumPV float64, cumVol float64, first15High float64, first15Low float64, s Settings, priorORBRanges []float64) {
	first15Complete := first15BarCount >= s.AvgCloseBars
	signalRatio := safeDiv(bar.Close, avg15)
	targetStopDistance, _, lookbackOK := targetStopDistance(first15Complete, first15High, first15Low, s, priorORBRanges)
	rthVWAP := safeDiv(cumPV, cumVol)
	distancePct := safeDiv(targetStopDistance, bar.Close)
	vwapRewardPct := safeDiv(bar.Close-rthVWAP, bar.Close)
	hodRiskPct := safeDiv(dayHigh-bar.Close, bar.Close)
	upperSignal := signalRatio >= s.UpperSignalRatio
	lowerSignal := signalRatio <= s.LowerSignalRatio

	coreReady := first15Complete &&
		lookbackOK &&
		signalRatio > 0 &&
		rthVWAP > 0 &&
		bar.Close >= s.MinEntryPrice &&
		first15Vol >= s.MinFirst15Vol &&
		first15DollarVol >= s.MinFirst15DollarVol

	state.lastStatus = "NO TRADE"
	state.lastAction = "NONE"
	state.lastReason = "Signal exists, but no branch matched."

	if !coreReady {
		state.lastReason = noTradeReason(first15Complete, lookbackOK, signalRatio, bar.Close, first15Vol, first15DollarVol, s)
		return
	}

	branch := ""
	side := 0
	entry := bar.Close
	target := 0.0
	stop := 0.0

	branch1 := signalRatio >= s.ModUpperMinSignal &&
		signalRatio <= s.ModUpperMaxSignal &&
		vwapRewardPct >= s.ModUpperMinVWAPRewardPct &&
		hodRiskPct >= s.ModUpperMinHODRiskPct &&
		hodRiskPct <= s.ModUpperMaxHODRiskPct

	branch2 := signalRatio >= s.ExtremeUpperMinSignal &&
		bar.Close >= s.ExtremeUpperMinPrice &&
		bar.Close <= s.ExtremeUpperMaxPrice

	branch3 := upperSignal &&
		bar.Close >= s.MidLongMinPrice &&
		bar.Close <= s.MidLongMaxPrice &&
		distancePct >= s.MidLongMinDistancePct &&
		distancePct <= s.MidLongMaxDistancePct &&
		signalRatio < s.MidLongMaxSignal

	branch4 := lowerSignal &&
		distancePct >= s.LowerShortMinTargetPct &&
		distancePct <= s.LowerShortMaxTargetPct &&
		hodRiskPct >= s.LowerShortMinHODRiskPct &&
		hodRiskPct <= s.LowerShortMaxHODRiskPct

	branch5 := lowerSignal &&
		distancePct >= s.LowerORBMinDistancePct &&
		distancePct <= s.LowerORBMaxDistancePct

	switch {
	case branch1:
		branch = "B1 VWAP FADE"
		side = -1
		target = rthVWAP
		stop = dayHigh
	case branch2:
		branch = "B2 EXT FADE"
		side = -1
		target = entry - targetStopDistance
		stop = entry + targetStopDistance
	case branch3:
		branch = "B3 LONG"
		side = 1
		target = entry + targetStopDistance
		stop = entry - targetStopDistance
	case branch4:
		branch = "B4 SHORT"
		if signalRatio <= s.LowerPrioritySignalRatio {
			branch = "B4 SHORT *"
		}
		side = -1
		target = entry - targetStopDistance
		stop = dayHigh
	case branch5:
		branch = "B5 1:1 SHORT"
		side = -1
		target = entry - targetStopDistance
		stop = entry + targetStopDistance
	}

	if side == 0 {
		return
	}

	riskPerShare := entry - stop
	rewardPerShare := target - entry
	if side == -1 {
		riskPerShare = stop - entry
		rewardPerShare = entry - target
	}
	rMultiple := safeDiv(rewardPerShare, riskPerShare)
	rawShares := 0
	if riskPerShare >= s.MinRiskPerShare {
		rawShares = int(math.Floor(s.RiskDollars / riskPerShare))
	}
	roundedShares := rawShares
	if s.ShareLotSize > 1 {
		roundedShares = int(math.Floor(float64(rawShares)/float64(s.ShareLotSize))) * s.ShareLotSize
	}
	if roundedShares > s.MaxShares {
		roundedShares = s.MaxShares
	}
	if riskPerShare < s.MinRiskPerShare || roundedShares <= 0 {
		state.lastReason = "Risk/share too small or shares rounded to 0."
		return
	}

	state.active = true
	state.taken = true
	state.side = side
	state.entryBarIndex = idx
	state.branch = branch
	state.entry = entry
	state.target = target
	state.stop = stop
	state.riskPerShare = riskPerShare
	state.rewardPerShare = rewardPerShare
	state.rMultiple = rMultiple
	state.shares = roundedShares
	state.signalRatio = signalRatio
	state.lastStatus = "ACTIVE " + sideName(side)
	if side == 1 {
		state.lastAction = "BUY"
	} else {
		state.lastAction = "SELL SHORT"
	}
	state.lastReason = ""
}

func targetStopDistance(first15Complete bool, first15High float64, first15Low float64, s Settings, priorORBRanges []float64) (float64, string, bool) {
	canUsePrior := len(priorORBRanges) > 0 && (!s.RequireFullLookback || len(priorORBRanges) >= s.LookbackSessions)
	if canUsePrior {
		sum := 0.0
		for _, v := range priorORBRanges {
			sum += v
		}
		return (sum / float64(len(priorORBRanges))) * s.TargetStopMultiplier, "Prior ORB", true
	}
	first15Range := 0.0
	if first15Complete && first15High > first15Low {
		first15Range = first15High - first15Low
	}
	if s.UseFirst15FallbackIfNoORB && first15Range > 0 {
		return first15Range * s.TargetStopMultiplier, "15m fallback", true
	}
	return 0, "No ORB", false
}

func targetStopDistanceEstimate(first15Complete bool, first15High float64, first15Low float64, s Settings, priorORBRanges []float64) (float64, string, bool) {
	distance, source, ok := targetStopDistance(first15Complete, first15High, first15Low, s, priorORBRanges)
	if distance > 0 {
		return distance, source, ok
	}
	if s.UseFirst15FallbackIfNoORB && first15High > first15Low {
		return (first15High - first15Low) * s.TargetStopMultiplier, "live 15m estimate", true
	}
	return 0, "No ORB", false
}

func noTradeReason(first15Complete bool, lookbackOK bool, ratio float64, price float64, first15Vol float64, first15DollarVol float64, s Settings) string {
	switch {
	case !first15Complete:
		return "Missing first15 bars."
	case !lookbackOK:
		return "Need ORB distance."
	case ratio == 0:
		return "Signal ratio unavailable."
	case price < s.MinEntryPrice:
		return "Price below min."
	case first15Vol < s.MinFirst15Vol:
		return "Vol15 below min."
	case first15DollarVol < s.MinFirst15DollarVol:
		return "$Vol15 below min."
	case ratio < s.UpperSignalRatio && ratio > s.LowerSignalRatio:
		return "No upper/lower signal."
	default:
		return "Core filter failed."
	}
}

func candidateScore(ev Evaluation, s Settings) float64 {
	if ev.Ratio == 0 {
		return 0
	}
	bandDistance := 0.0
	if ev.Ratio >= s.UpperSignalRatio {
		bandDistance = ev.Ratio - s.UpperSignalRatio
	} else if ev.Ratio <= s.LowerSignalRatio {
		bandDistance = s.LowerSignalRatio - ev.Ratio
	}
	volProgress := safeDiv(ev.First15Vol, s.MinFirst15Vol)
	if volProgress > 1 {
		volProgress = 1
	}
	score := 35 + bandDistance*1200 + volProgress*20
	if ev.LookbackOK {
		score += 10
	}
	return clamp(score, 0, 79)
}

func signalScore(ev Evaluation) float64 {
	score := 80.0
	if ev.RMultiple > 0 {
		score += math.Min(ev.RMultiple*8, 16)
	}
	score += math.Min(math.Abs(ev.DeltaPct)*150, 8)
	if ev.Branch == "B4 SHORT *" {
		score += 5
	}
	if ev.Phase == "done" && ev.Status == "STOPPED" {
		score -= 20
	}
	return clamp(score, 0, 100)
}

func estimateShares(riskPerShare float64, s Settings) int {
	if riskPerShare < s.MinRiskPerShare {
		return 0
	}
	rawShares := int(math.Floor(s.RiskDollars / riskPerShare))
	if s.ShareLotSize > 1 {
		rawShares = int(math.Floor(float64(rawShares)/float64(s.ShareLotSize))) * s.ShareLotSize
	}
	if rawShares > s.MaxShares {
		return s.MaxShares
	}
	return rawShares
}

func maxPositive(v float64, fallback float64) float64 {
	if v > fallback {
		return v
	}
	return fallback
}

func sparkPoints(bars []data.Bar, now time.Time, loc *time.Location, limit int) []SparkPoint {
	points := make([]SparkPoint, 0, limit)
	for _, b := range bars {
		if b.Time.In(loc).After(now.In(loc)) {
			break
		}
		points = append(points, SparkPoint{
			Time:  b.Time.In(loc).Format("15:04"),
			Close: b.Close,
		})
	}
	if len(points) > limit {
		points = points[len(points)-limit:]
	}
	return points
}

func sessionTime(now time.Time, loc *time.Location, hhmm string) time.Time {
	parts := strings.Split(hhmm, ":")
	hour := 9
	minute := 30
	if len(parts) == 2 {
		if v, err := parseInt(parts[0]); err == nil {
			hour = v
		}
		if v, err := parseInt(parts[1]); err == nil {
			minute = v
		}
	}
	n := now.In(loc)
	return time.Date(n.Year(), n.Month(), n.Day(), hour, minute, 0, 0, loc)
}

func inWindow(t time.Time, window string) bool {
	start, end, ok := strings.Cut(window, "-")
	if !ok {
		return false
	}
	st := sessionTime(t, t.Location(), start)
	et := sessionTime(t, t.Location(), end)
	return !t.Before(st) && t.Before(et)
}

func parseInt(s string) (int, error) {
	var out int
	_, err := fmt.Sscanf(s, "%d", &out)
	return out, err
}

func safeDiv(a, b float64) float64 {
	if b == 0 || math.IsNaN(a) || math.IsNaN(b) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		return 0
	}
	return a / b
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func sideName(side int) string {
	if side == 1 {
		return "LONG"
	}
	return "SHORT"
}

func holdAction(side int) string {
	if side == 1 {
		return "BUY/HOLD"
	}
	return "SHORT/HOLD"
}

func chartClock(t time.Time) string {
	if t.IsZero() {
		return "0945"
	}
	return t.Format("1504")
}

func chartURL(base, symbol, date, clock, signal string) string {
	base = strings.TrimRight(base, "/")
	if base == "" {
		return ""
	}
	if date != "" {
		if clock == "" {
			clock = "0945"
		}
		if signal == "" {
			signal = "buy"
		}
		return base + "/api/open-chart/" + url.PathEscape(symbol) + "/" + url.PathEscape(date) + "/" + url.PathEscape(clock) + "?resolution=1m&signal=" + url.QueryEscape(signal)
	}
	return base + "/?ticker=" + url.QueryEscape(symbol)
}

func finalize(ev Evaluation, s Settings) Evaluation {
	ev = clean(ev)
	signal := "buy"
	if ev.Side < 0 {
		signal = "sell"
	}
	ev.ChartURL = chartURL(s.ChartBaseURL, ev.Symbol, s.ChartDate, s.ChartTime, signal)
	return ev
}

func clean(ev Evaluation) Evaluation {
	if ev.Phase == "" {
		ev.Phase = "none"
	}
	if ev.Branch == "" {
		ev.Branch = "-"
	}
	if ev.Status == "" {
		ev.Status = "NO TRADE"
	}
	if ev.Action == "" {
		ev.Action = "NONE"
	}
	return ev
}
