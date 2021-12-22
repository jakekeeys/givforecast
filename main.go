package main

import (
	"ge-charge-optimiser/internal/api"
	"ge-charge-optimiser/internal/forecaster"
	"ge-charge-optimiser/internal/givtcp"
	"ge-charge-optimiser/internal/solcast"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	sc := solcast.NewClient()
	p := forecaster.New(sc)
	gtcp := givtcp.NewClient()
	s := api.NewServer(p, gtcp)

	r.GET("/forecast", s.Forecast)
	r.GET("/forecast/now", s.ForecastNow)
	r.GET("/forecast/config", s.Config)
	r.GET("/updatechargetarget", s.UpdateChargeTarget)

	err := r.Run(":8080")
	if err != nil {
		panic(err)
	}
}
