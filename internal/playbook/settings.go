package playbook

import "0945-playbook/internal/config"

type Settings struct {
	SessionOpen  string
	SessionClose string
	ChartBaseURL string
	ChartDate    string
	ChartTime    string

	EntryMinutesAfterOpen int
	AvgCloseBars          int
	MinEntryPrice         float64
	MinFirst15Vol         float64
	MinFirst15DollarVol   float64
	UpperSignalRatio      float64
	LowerSignalRatio      float64

	RiskDollars     float64
	ShareLotSize    int
	MaxShares       int
	MinRiskPerShare float64
	UseTimeExit     bool
	TimeExitWindow  string

	LookbackSessions          int
	OrbMinutes                int
	TargetStopMultiplier      float64
	RequireFullLookback       bool
	UseFirst15FallbackIfNoORB bool

	ModUpperMinSignal        float64
	ModUpperMaxSignal        float64
	ModUpperMinVWAPRewardPct float64
	ModUpperMinHODRiskPct    float64
	ModUpperMaxHODRiskPct    float64

	ExtremeUpperMinSignal float64
	ExtremeUpperMinPrice  float64
	ExtremeUpperMaxPrice  float64

	MidLongMinPrice       float64
	MidLongMaxPrice       float64
	MidLongMinDistancePct float64
	MidLongMaxDistancePct float64
	MidLongMaxSignal      float64

	LowerShortMinTargetPct   float64
	LowerShortMaxTargetPct   float64
	LowerShortMinHODRiskPct  float64
	LowerShortMaxHODRiskPct  float64
	LowerPrioritySignalRatio float64
	LowerORBMinDistancePct   float64
	LowerORBMaxDistancePct   float64
}

func SettingsFromConfig(cfg config.Config) Settings {
	return Settings{
		SessionOpen:  cfg.Session.Open,
		SessionClose: cfg.Session.Close,
		ChartBaseURL: cfg.Scan.ChartBaseURL,

		EntryMinutesAfterOpen: cfg.Signal.EntryMinutesAfterOpen,
		AvgCloseBars:          cfg.Signal.AvgCloseBars,
		MinEntryPrice:         cfg.Signal.MinEntryPrice,
		MinFirst15Vol:         cfg.Signal.MinFirst15Vol,
		MinFirst15DollarVol:   cfg.Signal.MinFirst15DollarVol,
		UpperSignalRatio:      cfg.Signal.UpperSignalRatio,
		LowerSignalRatio:      cfg.Signal.LowerSignalRatio,

		RiskDollars:     cfg.Risk.RiskDollars,
		ShareLotSize:    cfg.Risk.ShareLotSize,
		MaxShares:       cfg.Risk.MaxShares,
		MinRiskPerShare: cfg.Risk.MinRiskPerShare,
		UseTimeExit:     cfg.Risk.UseTimeExit,
		TimeExitWindow:  cfg.Risk.TimeExitWindow,

		LookbackSessions:          cfg.ORB.LookbackSessions,
		OrbMinutes:                cfg.ORB.OrbMinutes,
		TargetStopMultiplier:      cfg.ORB.TargetStopMultiplier,
		RequireFullLookback:       cfg.ORB.RequireFullLookback,
		UseFirst15FallbackIfNoORB: cfg.ORB.UseFirst15FallbackIfNoORB,

		ModUpperMinSignal:        cfg.Branch.ModerateUpperVWAPFade.MinSignal,
		ModUpperMaxSignal:        cfg.Branch.ModerateUpperVWAPFade.MaxSignal,
		ModUpperMinVWAPRewardPct: cfg.Branch.ModerateUpperVWAPFade.MinVWAPRewardPct,
		ModUpperMinHODRiskPct:    cfg.Branch.ModerateUpperVWAPFade.MinHODRiskPct,
		ModUpperMaxHODRiskPct:    cfg.Branch.ModerateUpperVWAPFade.MaxHODRiskPct,

		ExtremeUpperMinSignal: cfg.Branch.ExtremeUpperHardStopFade.MinSignal,
		ExtremeUpperMinPrice:  cfg.Branch.ExtremeUpperHardStopFade.MinPrice,
		ExtremeUpperMaxPrice:  cfg.Branch.ExtremeUpperHardStopFade.MaxPrice,

		MidLongMinPrice:       cfg.Branch.MidPricedUpperLong.MinPrice,
		MidLongMaxPrice:       cfg.Branch.MidPricedUpperLong.MaxPrice,
		MidLongMinDistancePct: cfg.Branch.MidPricedUpperLong.MinDistancePct,
		MidLongMaxDistancePct: cfg.Branch.MidPricedUpperLong.MaxDistancePct,
		MidLongMaxSignal:      cfg.Branch.MidPricedUpperLong.MaxSignal,

		LowerShortMinTargetPct:   cfg.Branch.LowerHODStopShort.MinTargetPct,
		LowerShortMaxTargetPct:   cfg.Branch.LowerHODStopShort.MaxTargetPct,
		LowerShortMinHODRiskPct:  cfg.Branch.LowerHODStopShort.MinHODRiskPct,
		LowerShortMaxHODRiskPct:  cfg.Branch.LowerHODStopShort.MaxHODRiskPct,
		LowerPrioritySignalRatio: cfg.Branch.LowerHODStopShort.PrioritySignalRatio,
		LowerORBMinDistancePct:   cfg.Branch.LowerOneToOneORBFallback.MinDistancePct,
		LowerORBMaxDistancePct:   cfg.Branch.LowerOneToOneORBFallback.MaxDistancePct,
	}
}
