package api

import (
	"fmt"
	"time"

	"github.com/jakekeeys/givforecast/internal/givenergy"

	"github.com/jakekeeys/givforecast/internal/forecaster"
	"github.com/jakekeeys/givforecast/internal/givtcp"
	"github.com/jakekeeys/givforecast/internal/solcast"
)

const (
	dateFormat = "2006-01-02"
	timeFormat = "2006-01-02T15:04"
)

type Server struct {
	f     *forecaster.Forecaster
	sc    *solcast.Client
	gtcpc *givtcp.Client
	gec   *givenergy.Client
}

func NewServer(f *forecaster.Forecaster, sc *solcast.Client, gtcpc *givtcp.Client, gec *givenergy.Client) *Server {
	return &Server{
		f:     f,
		sc:    sc,
		gtcpc: gtcpc,
		gec:   gec,
	}
}

func (s *Server) UpdateChargeTarget() error {
	println("updating solar forecasts")
	err := s.sc.UpdateForecast()
	if err != nil {
		return err
	}

	//println("updating consumption averages")
	//err = s.gec.UpdateConsumptionAverages()
	//if err != nil {
	//	return err
	//}

	now := time.Now().UTC()
	d := time.Date(now.Local().Year(), now.Local().Month(), now.Local().Day(), 0, 0, 0, 0, time.Local)
	println(fmt.Sprintf("forecasting date %s", d.String()))
	forecast, err := s.f.Forecast(d)
	if err != nil {
		return err
	}

	t := int(forecast.RecommendedChargeTarget)
	println(fmt.Sprintf("setting charge target to %d", t))
	// todo make this an interface supported by either givtcp or gecloud

	if s.f.GetConfig().AutomaticTargetsEnabled {
		maxRetries := 10
		for i := 1; i < maxRetries+1; i++ {
			err := s.gec.SetChargeUpperLimit(t)
			if err != nil {
				println(fmt.Errorf("setting charge target failed, attempt %d/%d waiting and retrying, err: %w", i, maxRetries, err).Error())
				time.Sleep(time.Second * time.Duration(i*3))
			} else {
				break
			}

			if i == maxRetries {
				return err
			}
		}
	}

	return nil
}

//func (s *Server) SubmitSolarActuals() error {
//	println("submitting solar readings to solcast")
//	now := time.Now().UTC()
//	yesterday := time.Date(now.Local().Year(), now.Local().Month(), now.Local().Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -1)
//	day, err := s.gec.PlantChartDay(yesterday)
//	if err != nil {
//		return err
//	}
//
//	solarActuals := map[time.Time]float64{}
//	for _, measurement := range day.Data {
//		t, err := time.Parse("2006-01-02 15:04:05", measurement.Time)
//		if err != nil {
//			return err
//		}
//
//		t = roundUpTime(t, time.Minute*10)
//		if v, ok := solarActuals[t]; ok {
//			solarActuals[t] = (v + measurement.Ppv) / 2
//		} else {
//			solarActuals[t] = measurement.Ppv
//		}
//	}
//
//	var measurements []solcast.Measurement
//	for k, v := range solarActuals {
//		if v < 50 {
//			continue
//		}
//
//		measurements = append(measurements, solcast.Measurement{
//			PeriodEnd:  k,
//			Period:     "PT10M",
//			TotalPower: v / 1000,
//		})
//	}
//
//	sort.Slice(measurements, func(i, j int) bool {
//		return measurements[i].PeriodEnd.Before(measurements[j].PeriodEnd)
//	})
//
//	err = s.sc.SubmitMeasurements(&solcast.SubmitMeasurementsRequest{Measurements: measurements})
//	if err != nil {
//		return err
//	}
//
//	return nil
//}

func roundUpTime(t time.Time, roundOn time.Duration) time.Time {
	t = t.Round(roundOn)

	if time.Since(t) >= 0 {
		t = t.Add(roundOn)
	}

	return t
}
