package api

import (
	"strconv"
	"time"

	"github.com/jakekeeys/givforecast/internal/assist"
	"github.com/jakekeeys/givforecast/internal/givenergy"

	"log/slog"

	"github.com/jakekeeys/givforecast/internal/forecaster"
	"github.com/jakekeeys/givforecast/internal/givtcp"
	"github.com/jakekeeys/givforecast/internal/solcast"
)

const (
	dateFormat = "2006-01-02"
	timeFormat = "2006-01-02T15:04"

	HA_CHARGE_TARGET_ENTITY_ID = "number.givtcp_ems2503039_ems_charge_target_soc_1"
)

type Server struct {
	f      *forecaster.Forecaster
	sc     *solcast.Client
	gtcpc  *givtcp.Client
	gec    *givenergy.Client
	hac    *assist.Client
	logger *slog.Logger
}

func NewServer(f *forecaster.Forecaster, sc *solcast.Client, gtcpc *givtcp.Client, gec *givenergy.Client, hac *assist.Client) *Server {
	logger := slog.Default().With("component", "api.Server")
	return &Server{
		f:      f,
		sc:     sc,
		gtcpc:  gtcpc,
		gec:    gec,
		hac:    hac,
		logger: logger,
	}
}

func (s *Server) UpdateChargeTarget() error {
	s.logger.Info("updating solar forecasts")
	err := s.sc.UpdateForecast()
	if err != nil {
		return err
	}

	//s.logger.Info("updating consumption averages")
	//err = s.gec.UpdateConsumptionAverages()
	//if err != nil {
	//	return err
	//}

	now := time.Now().UTC()
	d := time.Date(now.Local().Year(), now.Local().Month(), now.Local().Day(), 0, 0, 0, 0, time.Local)
	s.logger.Info("forecasting date", "date", d.String())
	forecast, err := s.f.Forecast(d)
	if err != nil {
		return err
	}

	t := int(forecast.RecommendedChargeTarget)
	s.logger.Info("setting charge target", "target", t)
	// todo make this an interface supported by either givtcp or gecloud

	if s.f.GetConfig().AutomaticTargetsEnabled {
		state, err := s.hac.GetState(HA_CHARGE_TARGET_ENTITY_ID)
		if err != nil {
			return err
		}

		if state.State == strconv.Itoa(t) {
			s.logger.Info("charge target is already set to the desired value")
			return nil
		}

		s.logger.Info("setting charge target", "target", t)
		err = s.hac.SetNumberValue(HA_CHARGE_TARGET_ENTITY_ID, strconv.Itoa(t))
		if err != nil {
			return err
		}

		// maxRetries := 10
		// for i := 1; i < maxRetries+1; i++ {
		// 	err := s.gec.SetChargeUpperLimit(t)
		// 	if err != nil {
		// 		println(fmt.Errorf("setting charge target failed, attempt %d/%d waiting and retrying, err: %w", i, maxRetries, err).Error())
		// 		time.Sleep(time.Second * time.Duration(i*3))
		// 	} else {
		// 		break
		// 	}

		// 	if i == maxRetries {
		// 		return err
		// 	}
		// }
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
