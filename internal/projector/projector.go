package projector

import (
	"ge-charge-optimiser/internal/solcast"
	"math"
	"time"
)

const (
	BaseConsumptionKw     = 0.85
	StorageCapacityKwh    = 16.4
	PeakConsumptionStartH = 7
	peakConsumptionStartM = 30
)

type Projector struct {
	sc *solcast.Client
}

func New(sc *solcast.Client) *Projector {
	return &Projector{sc: sc}
}

type Estimate struct {
	KwhProduction  float64
	KwhConsumption float64
	KwhExcess      float64
	ChargeTarget   float64
	Projections    []*Projection
}

type Projection struct {
	PeriodEnd     time.Time
	KwProduction  float64
	KwConsumption float64
	KwNet         float64
	SOC           float64
}

func (p *Projector) EstimateChargeTargetForDate(t time.Time) (*Estimate, error) {
	forecast, err := p.sc.GetForecast()
	if err != nil {
		return nil, err
	}

	t = t.Truncate(time.Hour * 24)
	var todayKwhProduction, todayKwhConsumption, todayKwhExcessProduction, maxSOC float64
	var projections []*Projection
	storageKwh := StorageCapacityKwh
	for _, forecast := range forecast.Forecasts {
		if forecast.PeriodEnd.After(t.AddDate(0, 0, 1)) || forecast.PeriodEnd.Before(t) {
			continue
		}

		if forecast.PeriodEnd.Hour() <= PeakConsumptionStartH {
			continue
		}

		if forecast.PeriodEnd.Hour() == PeakConsumptionStartH && forecast.PeriodEnd.Minute() == peakConsumptionStartM {
			continue
		}

		consumptionKwh := BaseConsumptionKw * 0.5
		todayKwhConsumption = todayKwhConsumption + consumptionKwh

		productionKwh := forecast.PvEstimate * 0.5
		todayKwhProduction = todayKwhProduction + productionKwh

		netConsumption := consumptionKwh - productionKwh
		if netConsumption < 0 {
			todayKwhExcessProduction = todayKwhExcessProduction + math.Abs(netConsumption)
		}

		storageKwh = storageKwh - netConsumption
		storageSOC := (storageKwh / StorageCapacityKwh) * 100
		if storageSOC > maxSOC {
			maxSOC = storageSOC
		}

		projections = append(projections, &Projection{
			PeriodEnd:     forecast.PeriodEnd,
			KwProduction:  forecast.PvEstimate,
			KwConsumption: BaseConsumptionKw,
			KwNet:         BaseConsumptionKw - forecast.PvEstimate,
			SOC:           storageSOC,
		})
		//println(fmt.Sprintf("estimated soc %.2f @ %s", (storageKwh/StorageCapacityKwh)*100, forecast.PeriodEnd))
	}

	chargeTarget := 100.0
	if maxSOC > 100 {
		chargeTarget = math.Abs((maxSOC - 100) - 100)
	}

	for _, projection := range projections {
		projection.SOC = projection.SOC - 100 + chargeTarget
	}

	return &Estimate{
		KwhProduction:  todayKwhProduction,
		KwhConsumption: todayKwhConsumption,
		KwhExcess:      todayKwhExcessProduction,
		ChargeTarget:   chargeTarget,
		Projections:    projections,
	}, nil
}
