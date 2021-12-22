package api

import (
	"ge-charge-optimiser/internal/forecaster"
	"ge-charge-optimiser/internal/givtcp"
	"ge-charge-optimiser/internal/solcast"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

const dateFormat = "2006-01-02"

type Server struct {
	f     *forecaster.Forecaster
	sc    *solcast.Client
	gtcpc *givtcp.Client
}

func NewServer(f *forecaster.Forecaster, sc *solcast.Client, gtcpc *givtcp.Client) *Server {
	return &Server{
		f:     f,
		sc:    sc,
		gtcpc: gtcpc,
	}
}

func (s *Server) UpdateChargeTarget(c *gin.Context) {
	config := s.f.GetConfig()

	now := time.Now().Local()
	var forecastDate time.Time
	if now.Hour() < config.GridPeakStartH && now.Minute() < config.GridPeakStartH {
		forecastDate = now.Truncate(time.Hour * 24)
	} else {
		forecastDate = now.Truncate(time.Hour*24).AddDate(0, 0, 1)
	}

	forecast, err := s.f.Forecast(forecastDate)
	if err != nil {
		c.String(http.StatusInternalServerError, "error getting forecast")
		return
	}

	err = s.gtcpc.SetChargeTarget(int(forecast.RecommendedChargeTarget))
	if err != nil {
		_ = c.Error(err)
		return
	}
}

func (s *Server) ForecastNow(c *gin.Context) {
	fn, err := s.f.ForecastNow()
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, fn)
}

func (s *Server) Forecast(c *gin.Context) {
	ds := c.Query("date")
	if ds == "" {
		ds = time.Now().Local().Format(dateFormat)
	}

	d, err := time.Parse(dateFormat, ds)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	today := time.Now().Local().Truncate(time.Hour * 24)
	if d.Before(today) {
		c.String(http.StatusBadRequest, "date must be today or < 7 days in the future")
		return
	}

	if d.After(today.AddDate(0, 0, 6)) {
		c.String(http.StatusBadRequest, "date must be today or < 7 days in the future")
		return
	}

	fc, err := s.f.Forecast(d)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, fc)
}

func (s *Server) Config(c *gin.Context) {
	config := s.f.GetConfig()

	c.JSON(http.StatusOK, config)
}
