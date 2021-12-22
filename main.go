package main

import (
	"ge-charge-optimiser/internal/api"
	"ge-charge-optimiser/internal/forecaster"
	"ge-charge-optimiser/internal/solcast"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	sc := solcast.NewClient()
	p := forecaster.New(sc)
	s := api.NewServer(p)

	r.GET("/forecastday", s.ForecastDay)
	r.GET("/forecastnow", s.ForecastNow)
	r.GET("/config", s.Config)

	err := r.Run(":8080")
	if err != nil {
		panic(err)
	}
}
