package api

import (
	"ge-charge-optimiser/internal/forecaster"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

const dateFormat = "2006-01-02"

type Server struct {
	f *forecaster.Forecaster
}

func NewServer(f *forecaster.Forecaster) *Server {
	return &Server{f: f}
}

func (s *Server) ForecastNow(c *gin.Context) {
	fn, err := s.f.ForecastNow()
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, fn)
}

func (s *Server) ForecastDay(c *gin.Context) {
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

	fc, err := s.f.ForecastDay(d)
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
