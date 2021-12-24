package forecaster

import (
	"errors"
	"math"
	"time"

	"github.com/jakekeeys/givforecast/internal/givenergy"
	"github.com/jakekeeys/givforecast/internal/solcast"
)

type Config struct {
	StorageCapacityKwh float64
	InverterEfficiency float64
	ACChargeStart      time.Time
	ACChargeEnd        time.Time
}

func WithConfig(c *Config) Option {
	return func(p *Forecaster) {
		p.config = c
	}
}

type Option func(p *Forecaster)

type Forecaster struct {
	sc     *solcast.Client
	gec    *givenergy.Client
	config *Config
}

func New(sc *solcast.Client, gec *givenergy.Client, opts ...Option) *Forecaster {
	acChargeStart := time.Date(1, 1, 1, 0, 30, 0, 0, time.Local)
	acChargeEnd := time.Date(1, 1, 1, 7, 30, 0, 0, time.Local)

	projector := &Forecaster{
		sc:  sc,
		gec: gec,
		config: &Config{
			StorageCapacityKwh: 16.38, // todo consume from ge cloud
			InverterEfficiency: 0.965,
			ACChargeStart:      acChargeStart,
			ACChargeEnd:        acChargeEnd,
		},
	}

	for _, opt := range opts {
		opt(projector)
	}

	return projector
}

type ForecastDay struct {
	Date                    time.Time
	ProductionKwh           float64
	ConsumptionKwh          float64
	ChargeKwh               float64
	DischargeKwh            float64
	RecommendedChargeTarget float64
	Forecasts               []*Forecast
}

type Forecast struct {
	PeriodEnd      time.Time
	ProductionKwh  float64
	ConsumptionKwh float64
	ChargeKwh      float64
	DischargeKwh   float64
	ProductionW    float64
	ConsumptionW   float64
	ChargeW        float64
	DischargeW     float64
	SOC            float64
}

func (f *Forecaster) GetConfig() *Config {
	return f.config
}

func (f *Forecaster) ForecastNow() (*Forecast, error) {
	fc, err := f.Forecast(time.Now().Local())
	if err != nil {
		return nil, err
	}

	now := time.Now().Local()
	peakStartToday := time.Date(now.Year(), now.Month(), now.Day(), f.config.ACChargeEnd.Hour(), f.config.ACChargeEnd.Minute(), 0, 0, time.Local)
	if now.Before(peakStartToday) {
		// todo after 0000 next day this is returned
		return &Forecast{
			PeriodEnd:      peakStartToday, // todo plus half an hour maybe or just return the first period for the day
			ProductionKwh:  0,
			ConsumptionKwh: 0,
			ChargeKwh:      0,
			DischargeKwh:   0,
			SOC:            fc.RecommendedChargeTarget,
		}, nil
	}

	for _, forecast := range fc.Forecasts {
		if forecast.PeriodEnd.Before(now) {
			continue
		}

		return &Forecast{
			PeriodEnd:      forecast.PeriodEnd.Local(),
			ProductionKwh:  forecast.ProductionKwh,
			ConsumptionKwh: forecast.ConsumptionKwh,
			ChargeKwh:      forecast.ChargeKwh,
			DischargeKwh:   forecast.DischargeKwh,
			ProductionW:    forecast.ProductionW,
			ConsumptionW:   forecast.ConsumptionW,
			ChargeW:        forecast.ChargeW,
			DischargeW:     forecast.DischargeW,
			SOC:            forecast.SOC,
		}, nil
	}

	return nil, errors.New("unable find matching forecast")
}

func (f *Forecaster) Forecast(t time.Time) (*ForecastDay, error) {
	forecast, err := f.sc.GetForecast()
	if err != nil {
		return nil, err
	}

	consumptionAverages, err := f.gec.GetConsumptionAverages()
	if err != nil {
		return nil, err
	}

	t = t.Truncate(time.Hour * 24)
	var dayProductionKwh, dayConsumptionKwh, dayMaxSOC, dayDischargeKwh, dayChargeKwh float64
	var forecasts []*Forecast
	for _, forecast := range forecast.Forecasts {
		if forecast.PeriodEnd.After(t.AddDate(0, 0, 1)) || forecast.PeriodEnd.Before(t) {
			continue
		}

		// todo not current forecasting from 2330 > 0000 because of this
		if forecast.PeriodEnd.Hour() <= f.config.ACChargeEnd.Hour() {
			continue
		}
		// todo probably want to return all data outside the charging window

		if forecast.PeriodEnd.Hour() == f.config.ACChargeEnd.Hour() && forecast.PeriodEnd.Minute() == f.config.ACChargeEnd.Minute() {
			continue
		}

		consumptionKwh := (consumptionAverages[time.Date(1, 1, 1, forecast.PeriodEnd.Hour(), 0, 0, 0, time.Local)] / 1000) * 0.5
		dayConsumptionKwh = dayConsumptionKwh + consumptionKwh

		productionKwh := forecast.PvEstimate * 0.5
		dayProductionKwh = dayProductionKwh + productionKwh

		netKwh := consumptionKwh - productionKwh
		var chargeKw, dischargeKw float64
		if netKwh > 0 {
			dischargeKw = netKwh * ((1 - f.config.InverterEfficiency) + 1)
			dayDischargeKwh = dayDischargeKwh + dischargeKw
		} else {
			chargeKw = math.Abs(netKwh) * f.config.InverterEfficiency
			dayChargeKwh = dayChargeKwh + chargeKw
		}

		storageSOC := (((f.config.StorageCapacityKwh - dayDischargeKwh) + dayChargeKwh) / f.config.StorageCapacityKwh) * 100
		if storageSOC > dayMaxSOC {
			dayMaxSOC = storageSOC
		}
		if storageSOC < 0 {
			storageSOC = 0
		}

		forecasts = append(forecasts, &Forecast{
			PeriodEnd:      forecast.PeriodEnd.Local(),
			ProductionKwh:  dayProductionKwh,
			ConsumptionKwh: dayConsumptionKwh,
			ChargeKwh:      dayChargeKwh,
			DischargeKwh:   dayDischargeKwh,
			ProductionW:    productionKwh * 2 * 1000,
			ConsumptionW:   consumptionKwh * 2 * 1000,
			ChargeW:        chargeKw * 2 * 1000,
			DischargeW:     dischargeKw * 2 * 1000,
			SOC:            storageSOC,
		})
	}

	recommendedChargeTarget := 100.0
	if dayMaxSOC > 100 {
		recommendedChargeTarget = math.Abs((dayMaxSOC - 100) - 100)
	}

	for _, projection := range forecasts {
		projection.SOC = projection.SOC - 100 + recommendedChargeTarget
	}

	return &ForecastDay{
		Date:                    t,
		ProductionKwh:           dayProductionKwh,
		ConsumptionKwh:          dayConsumptionKwh,
		ChargeKwh:               dayChargeKwh,
		DischargeKwh:            dayDischargeKwh,
		RecommendedChargeTarget: recommendedChargeTarget,
		Forecasts:               forecasts,
	}, nil
}
