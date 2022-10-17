package api

import (
	"bytes"
	"fmt"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/jakekeeys/givforecast/internal/forecaster"
	"time"
)

func ForcastToCharts(f *forecaster.ForecastDay) ([]byte, error) {
	productionChart := charts.NewLine()
	productionChart.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: "Solar",
		}))
	var xAxis []string
	var yAxis []opts.LineData
	for _, proj := range f.Forecasts {
		xAxis = append(xAxis, proj.PeriodEnd.Format(time.Kitchen))
		yAxis = append(yAxis, opts.LineData{
			Value:  proj.ProductionW / 1000,
			Symbol: "Kw",
		})
	}
	productionChart.SetXAxis(xAxis).
		AddSeries("W", yAxis)

	socChart := charts.NewLine()
	socChart.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: fmt.Sprintf("SOC (Overnight Target %.0f%%)", f.RecommendedChargeTarget),
		}))
	xAxis = []string{}
	yAxis = []opts.LineData{}
	for _, proj := range f.Forecasts {
		xAxis = append(xAxis, proj.PeriodEnd.Format(time.Kitchen))
		yAxis = append(yAxis, opts.LineData{
			Value: proj.SOC,
		})
	}
	socChart.SetXAxis(xAxis).
		AddSeries("%", yAxis)

	chargeDischargeChart := charts.NewLine()
	chargeDischargeChart.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: "Charge/Discharge",
		}))
	xAxis = []string{}
	yAxis = []opts.LineData{}
	var yAxis2 []opts.LineData
	for _, proj := range f.Forecasts {
		xAxis = append(xAxis, proj.PeriodEnd.Format(time.Kitchen))
		yAxis = append(yAxis, opts.LineData{
			Name:   "Discharge",
			Symbol: "Kw",
			Value:  proj.DischargeW / 1000,
		})
		yAxis2 = append(yAxis2, opts.LineData{
			Name:   "Charge",
			Symbol: "Kw",
			Value:  proj.ChargeW / 1000,
		})
	}
	chargeDischargeChart.SetXAxis(xAxis).
		AddSeries("Discharge", yAxis).
		AddSeries("Charge", yAxis2)

	consumptionChart := charts.NewLine()
	consumptionChart.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: "Consumption",
		}))
	xAxis = []string{}
	yAxis = []opts.LineData{}
	for _, proj := range f.Forecasts {
		xAxis = append(xAxis, proj.PeriodEnd.Format(time.Kitchen))
		yAxis = append(yAxis, opts.LineData{
			Value: proj.ConsumptionW,
		})
	}
	consumptionChart.SetXAxis(xAxis).
		AddSeries("W", yAxis)

	page := components.NewPage()
	page.SetLayout(components.PageFlexLayout)

	page.AddCharts(socChart)
	page.AddCharts(chargeDischargeChart)
	page.AddCharts(productionChart)
	page.AddCharts(consumptionChart)

	bodyBuf := bytes.NewBuffer([]byte{})

	err := page.Render(bodyBuf)
	if err != nil {
		return nil, err
	}

	return bodyBuf.Bytes(), nil
}
