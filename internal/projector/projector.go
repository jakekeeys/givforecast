package projector

import (
	"ge-charge-optimiser/internal/solcast"
	"math"
	"time"
)

type Config struct {
	BaseConsumptionKwh float64
	StorageCapacityKwh float64
	GridPeakStartH     int
	GridPeakStartM     int
}

func WithConfig(c *Config) Option {
	return func(p *Projector) {
		p.config = c
	}
}

type Option func(p *Projector)

type Projector struct {
	sc     *solcast.Client
	config *Config
}

func New(sc *solcast.Client, opts ...Option) *Projector {
	projector := &Projector{
		sc: sc,
		config: &Config{
			BaseConsumptionKwh: 1.00,
			StorageCapacityKwh: 16.38,
			GridPeakStartH:     7,
			GridPeakStartM:     30,
		},
	}

	for _, opt := range opts {
		opt(projector)
	}

	return projector
}

type Estimate struct {
	Date           time.Time
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

func (p *Projector) GetConfig() *Config {
	return p.config
}

func (p *Projector) Project(t time.Time) (*Estimate, error) {
	forecast, err := p.sc.GetForecast()
	if err != nil {
		return nil, err
	}

	t = t.Truncate(time.Hour * 24)
	var todayKwhProduction, todayKwhConsumption, todayKwhExcessProduction, maxSOC float64
	var projections []*Projection
	storageKwh := p.config.StorageCapacityKwh
	for _, forecast := range forecast.Forecasts {
		if forecast.PeriodEnd.After(t.AddDate(0, 0, 1)) || forecast.PeriodEnd.Before(t) {
			continue
		}

		if forecast.PeriodEnd.Hour() <= p.config.GridPeakStartH {
			continue
		}

		if forecast.PeriodEnd.Hour() == p.config.GridPeakStartH && forecast.PeriodEnd.Minute() == p.config.GridPeakStartM {
			continue
		}

		consumptionKwh := p.config.BaseConsumptionKwh * 0.5
		todayKwhConsumption = todayKwhConsumption + consumptionKwh

		productionKwh := forecast.PvEstimate * 0.5
		todayKwhProduction = todayKwhProduction + productionKwh

		netConsumption := consumptionKwh - productionKwh
		if netConsumption < 0 {
			todayKwhExcessProduction = todayKwhExcessProduction + math.Abs(netConsumption)
		}

		storageKwh = storageKwh - netConsumption
		storageSOC := (storageKwh / p.config.StorageCapacityKwh) * 100
		if storageSOC > maxSOC {
			maxSOC = storageSOC
		}

		projections = append(projections, &Projection{
			PeriodEnd:     forecast.PeriodEnd,
			KwProduction:  forecast.PvEstimate,
			KwConsumption: p.config.BaseConsumptionKwh,
			KwNet:         p.config.BaseConsumptionKwh - forecast.PvEstimate,
			SOC:           storageSOC,
		})
	}

	chargeTarget := 100.0
	if maxSOC > 100 {
		chargeTarget = math.Abs((maxSOC - 100) - 100)
	}

	for _, projection := range projections {
		projection.SOC = projection.SOC - 100 + chargeTarget
	}

	return &Estimate{
		Date:           t,
		KwhProduction:  todayKwhProduction,
		KwhConsumption: todayKwhConsumption,
		KwhExcess:      todayKwhExcessProduction,
		ChargeTarget:   chargeTarget,
		Projections:    projections,
	}, nil
}
