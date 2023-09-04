package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/jakekeeys/givforecast/internal/forecaster"

	"github.com/gin-gonic/gin"
	"github.com/jakekeeys/givforecast/internal/solcast"
)

func (s *Server) RootHandler(c *gin.Context) {
	d := time.Now()

	ds := c.Query("date")
	if ds != "" {
		switch ds {
		case "tomorrow":
			d = d.Add(24 * time.Hour)
		default:
			tp, err := time.Parse(dateFormat, ds)
			if err != nil {
				c.String(http.StatusBadRequest, err.Error())
				return
			}
			d = tp
		}
	}

	forecast, err := s.f.Forecast(d)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	charts, err := ForcastToCharts(forecast)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	_, err = c.Writer.Write(charts)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
}

func (s *Server) UpdateChargeTargetHandler(c *gin.Context) {
	err := s.UpdateChargeTarget()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
}

func (s *Server) SetChargeTargetHandler(c *gin.Context) {
	type SetChargeTargetRequest struct {
		ChargeToPercent int `json:"chargeToPercent"`
	}

	var ctr SetChargeTargetRequest
	err := c.ShouldBindJSON(&ctr)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	maxRetries := 10
	for i := 1; i < maxRetries+1; i++ {
		err := s.gec.SetChargeUpperLimit(ctr.ChargeToPercent)
		if err != nil {
			println(fmt.Errorf("setting charge target failed, attempt %d/%d waiting and retrying, err: %w", i, maxRetries, err).Error())
			time.Sleep(time.Second * time.Duration(i*3))
		} else {
			break
		}

		if i == maxRetries {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
	}
}

func (s *Server) ForecastNowHandler(c *gin.Context) {
	t := time.Now().Local()

	ts := c.Query("time")
	if ts != "" {
		pt, err := time.ParseInLocation(timeFormat, ts, time.Local)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}
		t = pt
	}

	fn, err := s.f.ForecastNow(t)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, fn)
}

func (s *Server) ForecastHandler(c *gin.Context) {
	d := time.Now()

	ds := c.Query("date")
	if ds != "" {
		switch ds {
		case "tomorrow":
			d = d.Add(24 * time.Hour)
		default:
			tp, err := time.Parse(dateFormat, ds)
			if err != nil {
				c.String(http.StatusBadRequest, err.Error())
				return
			}
			d = tp
		}
	}

	now := time.Now().UTC()
	today := time.Date(now.Local().Year(), now.Local().Month(), now.Local().Day(), 0, 0, 0, 0, time.Local)
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
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, fc)
}

func (s *Server) ConfigHandler(c *gin.Context) {
	config := s.f.GetConfig()

	c.JSON(http.StatusOK, config)
}

func (s *Server) SetConfigHandler(c *gin.Context) {
	var config forecaster.Config
	err := c.ShouldBindJSON(&config)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	s.f.SetConfig(config)
	return
}

func (s *Server) SetConsumptionAverage(c *gin.Context) {
	var value struct {
		Value float64 `json:"value"`
	}

	err := c.ShouldBindJSON(&value)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	config := s.f.GetConfig()
	config.AvgConsumptionKw = value.Value

	s.f.SetConfig(*config)
	return
}

func (s *Server) SetBatteryUpper(c *gin.Context) {
	var value struct {
		Value float64 `json:"value"`
	}

	err := c.ShouldBindJSON(&value)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	config := s.f.GetConfig()
	config.BatteryUpperReserve = value.Value

	s.f.SetConfig(*config)
	return
}

func (s *Server) SetBatteryLower(c *gin.Context) {
	var value struct {
		Value float64 `json:"value"`
	}

	err := c.ShouldBindJSON(&value)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	config := s.f.GetConfig()
	config.BatteryLowerReserve = value.Value

	s.f.SetConfig(*config)
	return
}

func (s *Server) SetAutomaticTargets(c *gin.Context) {
	var value struct {
		Value bool `json:"value"`
	}

	err := c.ShouldBindJSON(&value)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	config := s.f.GetConfig()
	config.AutomaticTargetsEnabled = value.Value

	s.f.SetConfig(*config)
	return
}

//func (s *Server) SetConsumptionAveragesHandler(c *gin.Context) {
//	var data map[time.Time]float64
//	err := c.ShouldBindJSON(&data)
//	if err != nil {
//		c.String(http.StatusInternalServerError, err.Error())
//		return
//	}
//
//	s.gec.SetConsumptionAverages(data)
//	return
//}

//func (s *Server) GetBatteryDataHandler(c *gin.Context) {
//	data, err := s.gec.GetBatteryData()
//	if err != nil {
//		c.String(http.StatusInternalServerError, err.Error())
//		return
//	}
//
//	c.JSON(http.StatusOK, data)
//}

//func (s *Server) GetConsumptionAveragesHandler(c *gin.Context) {
//	averages, err := s.gec.GetConsumptionAverages()
//	if err != nil {
//		c.String(http.StatusInternalServerError, err.Error())
//		return
//	}
//
//	c.JSON(http.StatusOK, averages)
//}

//func (s *Server) UpdateConsumptionAveragesHandler(c *gin.Context) {
//	err := s.gec.UpdateConsumptionAverages()
//	if err != nil {
//		c.String(http.StatusInternalServerError, err.Error())
//		return
//	}
//}

func (s *Server) SetForecastDataHandler(c *gin.Context) {
	var data solcast.ForecastData
	err := c.ShouldBindJSON(&data)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	err = s.sc.SetForecast(data)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	return
}

func (s *Server) UpdateForecastDataHandler(c *gin.Context) {
	err := s.sc.UpdateForecast()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	return
}

func (s *Server) GetForecastDataHandler(c *gin.Context) {
	forecast, err := s.sc.GetForecast()
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, forecast)
	return
}

//func (s *Server) SubmitSolarActualsHandler(c *gin.Context) {
//	err := s.SubmitSolarActuals()
//	if err != nil {
//		c.String(http.StatusInternalServerError, err.Error())
//		return
//	}
//}
