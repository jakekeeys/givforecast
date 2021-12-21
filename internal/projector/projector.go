package projector

import (
	"fmt"
	"ge-charge-optimiser/internal/solcast"
	"math"
	"time"
)

type Config struct {
	BaseConsumptionKwh float64
	StorageCapacityKwh float64
	GridPeakStartH     int
	GridPeakStartM     int
	InverterEfficiency float64
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
			BaseConsumptionKwh: 1.1,
			StorageCapacityKwh: 16.38,
			GridPeakStartH:     7,
			GridPeakStartM:     30,
			InverterEfficiency: 0.9,
		},
	}

	for _, opt := range opts {
		opt(projector)
	}

	return projector
}

type Estimate struct {
	Date                    time.Time
	ProductionKwh           float64
	ConsumptionKwh          float64
	ChargeKwh               float64
	DischargeKwh            float64
	RecommendedChargeTarget float64
	Projections             []*Projection
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
	var dayProductionKwh, dayConsumptionKwh, dayMaxSOC, dayDischargeKwh, dayChargeKwh float64
	var projections []*Projection
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
		dayConsumptionKwh = dayConsumptionKwh + consumptionKwh

		productionKwh := forecast.PvEstimate * 0.5
		dayProductionKwh = dayProductionKwh + productionKwh

		netKwh := consumptionKwh - productionKwh
		if netKwh > 0 {
			dayDischargeKwh = dayDischargeKwh + netKwh*((1-p.config.InverterEfficiency)+1)
		} else {
			dayChargeKwh = dayChargeKwh + (math.Abs(netKwh) * p.config.InverterEfficiency)
		}

		storageSOC := (((p.config.StorageCapacityKwh - dayDischargeKwh) + dayChargeKwh) / p.config.StorageCapacityKwh) * 100
		if storageSOC > dayMaxSOC {
			dayMaxSOC = storageSOC
		}

		projections = append(projections, &Projection{
			PeriodEnd:     forecast.PeriodEnd,
			KwProduction:  forecast.PvEstimate,
			KwConsumption: p.config.BaseConsumptionKwh,
			KwNet:         p.config.BaseConsumptionKwh - forecast.PvEstimate,
			SOC:           storageSOC,
		})

		println(fmt.Sprintf("time: %s, consumption: %.2f, production %.2f, soc: %.2f, net: %.2f, discharged: %.2f, charged: %.2f", forecast.PeriodEnd, dayConsumptionKwh, dayProductionKwh, storageSOC, netKwh, dayDischargeKwh, dayChargeKwh))
	}

	recommendedChargeTarget := 100.0
	if dayMaxSOC > 100 {
		recommendedChargeTarget = math.Abs((dayMaxSOC - 100) - 100)
	}

	for _, projection := range projections {
		projection.SOC = projection.SOC - 100 + recommendedChargeTarget
	}

	return &Estimate{
		Date:                    t,
		ProductionKwh:           dayProductionKwh,
		ConsumptionKwh:          dayConsumptionKwh,
		ChargeKwh:               dayChargeKwh,
		DischargeKwh:            dayDischargeKwh,
		RecommendedChargeTarget: recommendedChargeTarget,
		Projections:             projections,
	}, nil
}
