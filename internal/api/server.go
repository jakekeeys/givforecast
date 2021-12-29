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

	println("updating consumption averages")
	err = s.gec.UpdateConsumptionAverages()
	if err != nil {
		return err
	}

	d := time.Now().Local().Truncate(time.Hour * 24)
	println(fmt.Sprintf("forecasting date %s", d.String()))
	forecast, err := s.f.Forecast(time.Now().Truncate(time.Hour * 24))
	if err != nil {
		return err
	}

	t := int(forecast.RecommendedChargeTarget)
	println(fmt.Sprintf("setting charge target to %d", t))
	// todo make this an interface supported by either givtcp or gecloud
	err = s.gtcpc.SetChargeTarget(t)
	if err != nil {
		return err
	}

	return nil
}
